// Copyright 2025 The Kaia Authors
//
// Erigon is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// Erigon is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with Erigon. If not, see <http://www.gnu.org/licenses/>.

package kaiatrie

import (
	"context"
	"errors"
	"fmt"

	"github.com/erigontech/erigon-lib/commitment"
	"github.com/erigontech/erigon-lib/common/length"
	"github.com/erigontech/erigon-lib/crypto"
	"github.com/erigontech/erigon-lib/kv"
	"github.com/erigontech/erigon-lib/state"
	"github.com/erigontech/erigon-lib/types/accounts"
)

var (
	_ commitment.PatriciaContext = (*DeferredContext)(nil)

	keyHphState = []byte("hphstate") // "hphstate" => Latest {hphState, blockNum, txNum}

	errNoDomains = errors.New("cannot operate without domains")
	errNotLatest = errors.New("cannot operate on non-latest commitment state")
)

type AccountMode uint8

const (
	ModeErigonV3 AccountMode = 0
	ModeRawBytes AccountMode = 1
)

// DeferredContext is an implmentation of PatriciaContext that allows a deferred involvement of the SharedDomains
// to minimize the usage of the SharedDomains, and database transactions within. This allows concurrent access
// to the state database, which is required in Kaia statedb.
type DeferredContext struct {
	// Account decoding mode
	accountMode AccountMode

	// Unhashed and uncommitted changes.
	pendingUpdates  *commitment.Updates
	trie            *commitment.HexPatriciaHashed
	trieStateLoaded bool

	// Uncommitted changes.
	pendingAccounts map[string][]byte        // addr[20] => SerialiseV3 (ModeErigonV3) or RawBytes (ModeRawBytes)
	pendingStorages map[string][]byte        // addr[20] || slot[32] => data[32]
	pendingBranches map[string]pendingBranch // prefix[] => data[], prevData[], prevStep

	// Committed at current block.
	committedAccounts map[string][]byte
	committedStorages map[string][]byte
	committedBranches map[string]pendingBranch

	// Underlying database.
	sd   *state.SharedDomains
	step uint64

	trace bool
}

type pendingBranch struct {
	data     []byte
	prevData []byte
	prevStep uint64
}

func NewDeferredContext(tmpdir string, accountMode AccountMode) *DeferredContext {
	ctx := &DeferredContext{
		accountMode:       accountMode,
		pendingUpdates:    commitment.NewUpdates(commitment.ModeDirect, tmpdir, commitment.KeyToHexNibbleHash),
		pendingAccounts:   make(map[string][]byte),
		pendingStorages:   make(map[string][]byte),
		pendingBranches:   make(map[string]pendingBranch),
		committedAccounts: make(map[string][]byte),
		committedStorages: make(map[string][]byte),
		committedBranches: make(map[string]pendingBranch),
		step:              0,
	}
	ctx.trie = commitment.NewHexPatriciaHashed(length.Addr, ctx, tmpdir)
	return ctx
}

func (c *DeferredContext) SetDomains(sd *state.SharedDomains) {
	c.sd = sd
	c.step = sd.TxNum() / sd.StepSize()
}

func (c *DeferredContext) SetTrace(trace bool) {
	c.trace = trace
}

func (c *DeferredContext) tracef(format string, args ...any) {
	if c.trace {
		fmt.Printf(format, args...)
	}
}

func (c *DeferredContext) PutAccount(plainKey []byte, encAccount []byte) {
	c.pendingUpdates.TouchPlainKey(string(plainKey), encAccount, c.pendingUpdates.TouchAccount)
	c.pendingAccounts[string(plainKey)] = encAccount
	c.tracef("ctx.PutAccount %x: %x (#%d)\n", plainKey, encAccount, c.pendingUpdates.Size())
}

func (c *DeferredContext) AccountRaw(plainKey []byte) ([]byte, error) {
	if data, ok := c.pendingAccounts[string(plainKey)]; ok {
		c.tracef("ctx.AccountRaw(pending) %x: %x\n", plainKey, data)
		return data, nil
	}
	if data, ok := c.committedAccounts[string(plainKey)]; ok {
		c.tracef("ctx.AccountRaw(committed) %x: %x\n", plainKey, data)
		return data, nil
	}

	if c.sd != nil {
		data, err := c.sd.GetCommitmentContext().AccountRaw(plainKey)
		if err != nil {
			return nil, err
		}
		c.tracef("ctx.AccountRaw(db) %x: %x\n", plainKey, data)
		return data, nil
	}

	// Account data does not exist. Return empty account.
	return nil, nil
}

func (c *DeferredContext) Account(plainKey []byte) (*commitment.Update, error) {
	encAccount, err := c.AccountRaw(plainKey)
	if err != nil {
		return nil, err
	}

	if c.accountMode == ModeRawBytes {
		u := &commitment.Update{CodeHash: commitment.EmptyCodeHashArray} // default to empty code hash
		u.Flags = commitment.RawBytesUpdate
		u.RawBytes = make([]byte, len(encAccount))
		copy(u.RawBytes, encAccount)
		return u, nil
	}

	if c.accountMode == ModeErigonV3 {
		u := &commitment.Update{CodeHash: commitment.EmptyCodeHashArray} // default to empty code hash
		if len(encAccount) == 0 {
			u.Flags = commitment.DeleteUpdate
			return u, nil
		}

		acc := new(accounts.Account)
		if err := accounts.DeserialiseV3(acc, encAccount); err != nil {
			return nil, err
		}

		u.Flags |= commitment.NonceUpdate
		u.Nonce = acc.Nonce

		u.Flags |= commitment.BalanceUpdate
		u.Balance.Set(&acc.Balance)

		if ch := acc.CodeHash.Bytes(); len(ch) > 0 { // if code hash is not empty
			u.Flags |= commitment.CodeUpdate
			copy(u.CodeHash[:], ch)
		}
		return u, nil
	}

	return nil, fmt.Errorf("invalid account mode: %d", c.accountMode)
}

func (c *DeferredContext) PutStorage(plainKey []byte, encStorage []byte) {
	c.pendingUpdates.TouchPlainKey(string(plainKey), encStorage, c.pendingUpdates.TouchStorage)
	c.pendingStorages[string(plainKey)] = encStorage
	c.tracef("ctx.PutStorage %x: %x (#%d)\n", plainKey, encStorage, c.pendingUpdates.Size())
}

func (c *DeferredContext) StorageRaw(plainKey []byte) ([]byte, error) {
	// Read from pending changes and wrap it into Update. See SharedDomainsCommitmentContext.Storage().
	if data, ok := c.pendingStorages[string(plainKey)]; ok {
		c.tracef("ctx.StorageRaw(pending) %x: %x\n", plainKey, data)
		return data, nil
	}
	if data, ok := c.committedStorages[string(plainKey)]; ok {
		c.tracef("ctx.StorageRaw(committed) %x: %x\n", plainKey, data)
		return data, nil
	}

	// Read from database if available.
	if c.sd != nil {
		data, err := c.sd.GetCommitmentContext().StorageRaw(plainKey)
		if err != nil {
			return nil, err
		}
		c.tracef("ctx.StorageRaw(db) %x: %x\n", plainKey, data)
		return data, nil
	}

	// Storage data does not exist. Return empty storage.
	return nil, nil
}

func (c *DeferredContext) Storage(plainKey []byte) (*commitment.Update, error) {
	encData, err := c.StorageRaw(plainKey)
	if err != nil {
		return nil, err
	}

	// Wrap it into Update. See SharedDomainsCommitmentContext.Storage().
	if len(encData) == 0 {
		return &commitment.Update{Flags: commitment.DeleteUpdate}, nil
	}

	u := &commitment.Update{
		Flags:      commitment.StorageUpdate,
		StorageLen: len(encData),
	}
	copy(u.Storage[:u.StorageLen], encData)
	return u, nil
}

func (c *DeferredContext) PutBranch(prefix []byte, data []byte, prevData []byte, prevStep uint64) error {
	c.tracef("ctx.PutBranch %x: %x, %d\n", prefix, data, prevStep)
	c.pendingBranches[string(prefix)] = pendingBranch{
		data:     data,
		prevData: prevData,
		prevStep: prevStep,
	}
	return nil
}

func (c *DeferredContext) Branch(prefix []byte) ([]byte, uint64, error) {
	if pb, ok := c.pendingBranches[string(prefix)]; ok {
		c.tracef("ctx.Branch(pending) %x: %x, %d\n", prefix, pb.data, pb.prevStep)
		return pb.data, pb.prevStep, nil
	}
	if pb, ok := c.committedBranches[string(prefix)]; ok {
		c.tracef("ctx.Branch(committed) %x: %x, %d\n", prefix, pb.data, pb.prevStep)
		return pb.data, pb.prevStep, nil
	}

	if c.sd != nil {
		u, step, err := c.sd.GetCommitmentContext().Branch(prefix)
		c.tracef("ctx.Branch(db) %x: %x, %d\n", prefix, u, step)
		return u, step, err
	}

	// Branch data does not exist. Return nil.
	return nil, 0, nil
}

// Load the internal state of HexPatriciaHashed (hph state) before hashing new updates.
// The database stores the latest block's hph state. We can only work from there, so we will only calculate
// the root hash of the pending block only.
func (c *DeferredContext) loadHphState() error {
	if c.trieStateLoaded {
		return nil
	}

	// Must use the keyCommitmentState because it is special keyword in SharedDomains, bypassing any Branch-specific logic.
	// Because CommitmentDomain does not store history (see erigon-lib/state/aggregator2.go:Schema[kv.CommitmentDomain]),
	// Branch() will always return the latest state.
	cs, err := customGet(c.sd, keyHphState)
	if err != nil {
		return err
	}

	// Special case where the database is empty, i.e. not even genesis block is committed.
	if len(cs) == 0 {
		c.tracef("ctx.Load empty state\n")
		c.trieStateLoaded = true
		return nil
	}

	txNum, blockNum, hphState, err := state.DecodeCommitmentState(cs)
	if err != nil {
		return err
	}

	// Make sure the provided `sd` is at the latest block. Otherwise, we will be wrongly calculate the
	// root hash of an historic block (sd.BlockNum()) from the latest hph state (cs)
	c.tracef("ctx.Load stored blockNum=%d txNum=%d context blockNum=%d txNum=%d\n", blockNum, txNum, c.sd.BlockNum(), c.sd.TxNum())
	if txNum != c.sd.TxNum() || blockNum != c.sd.BlockNum() {
		return fmt.Errorf("%w: stored txNum=%d blockNum=%d context txNum=%d blockNum=%d", errNotLatest, txNum, blockNum, c.sd.TxNum(), c.sd.BlockNum())
	}

	c.tracef("ctx.Load hash(hphstate)=%x\n", crypto.Keccak256(hphState))
	c.trie.SetState(hphState)
	c.trieStateLoaded = true
	return nil
}

func (c *DeferredContext) Hash() ([]byte, error) {
	// Load the last hph state before processing new updates.
	if c.sd == nil {
		return nil, fmt.Errorf("cannot calculate merkle root hash: %w", errNoDomains)
	}
	if err := c.loadHphState(); err != nil {
		return nil, err
	}

	// Note: c.pendingUpdates will be reset inside Process(), no need to Reset here.
	// See HexPatriciaHashed.Process() > updates.HashSort() > clear(t.keys)
	numUpdates := c.pendingUpdates.Size()
	rootHash, err := c.trie.Process(context.Background(), c.pendingUpdates, "")
	c.tracef("ctx.Hash len(updates)=%d, rootHash=%x\n", numUpdates, rootHash)
	return rootHash, err
}

// Commit pending changes to database.
func (c *DeferredContext) Commit() error {
	if c.sd == nil {
		return fmt.Errorf("cannot commit pending changes: %w", errNoDomains)
	}
	c.tracef("ctx.Commit to blockNum=%d txNum=%d len(accounts)=%d len(storages)=%d len(branches)=%d\n", c.sd.BlockNum(), c.sd.TxNum(), len(c.pendingAccounts), len(c.pendingStorages), len(c.pendingBranches))

	// Commit pending accounts.
	for addr, acc := range c.pendingAccounts {
		if err := c.sd.DomainPutOrDelRaw(kv.AccountsDomain, []byte(addr), acc); err != nil {
			fmt.Printf("xx ctx.Commit fail %v\n", err)
			return err
		}
	}
	c.committedAccounts = c.pendingAccounts
	c.pendingAccounts = make(map[string][]byte)

	// Commit pending storages.
	for plainKey, encStorage := range c.pendingStorages {
		if err := c.sd.DomainPutOrDelRaw(kv.StorageDomain, []byte(plainKey), encStorage); err != nil {
			return err
		}
	}
	c.committedStorages = c.pendingStorages
	c.pendingStorages = make(map[string][]byte)

	// Commit pending branches.
	for prefix, pb := range c.pendingBranches {
		if err := c.sd.GetCommitmentContext().PutBranch([]byte(prefix), pb.data, pb.prevData, pb.prevStep); err != nil {
			return err
		}
	}
	c.committedBranches = c.pendingBranches
	c.pendingBranches = make(map[string]pendingBranch)

	// Commit hph state.
	hphState, err := c.trie.EncodeCurrentState(nil)
	if err != nil {
		return err
	}
	c.tracef("ctx.Commit hash(hphstate)=%x\n", crypto.Keccak256(hphState))
	cs, err := state.EncodeCommitmentState(c.sd.TxNum(), c.sd.BlockNum(), hphState)
	if err != nil {
		return err
	}
	if err := customPut(c.sd, keyHphState, cs); err != nil {
		return err
	}

	return nil
}

// Abuse the `ReceiptDomain` to store custom data (i.e. not account/storage/branch). This is safe because
// (1) ReceiptDomain is irrelevant to the state trie processing.
// (2) Kaia will use its own database for receipts, so ReceiptDomain is not used for storing receipts.
func customGet(sd *state.SharedDomains, key []byte) ([]byte, error) {
	data, _, err := sd.GetLatest(kv.ReceiptDomain, key)
	return data, err
}

func customPut(sd *state.SharedDomains, key []byte, data []byte) error {
	return sd.DomainPutOrDelRaw(kv.ReceiptDomain, key, data)
}
