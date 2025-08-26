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
	"fmt"

	"github.com/erigontech/erigon-lib/commitment"
	"github.com/erigontech/erigon-lib/kv"
	"github.com/erigontech/erigon-lib/state"
	"github.com/erigontech/erigon-lib/types/accounts"
)

var (
	_ commitment.PatriciaContext = (*DeferredContext)(nil)
)

type DeferredContext struct {
	pendingAccounts map[string][]byte        // addr[20] => SerialiseV3 (ModeErigonV3) or RawBytes (ModeRawBytes)
	pendingStorages map[string][]byte        // addr[20] || slot[32] => data[32]
	pendingBranches map[string]pendingBranch // prefix[] => data[], prevData[], prevStep
	step            uint64

	sd *state.SharedDomains

	trace bool
}

type pendingBranch struct {
	data     []byte
	prevData []byte
	prevStep uint64
}

func NewDeferredContext() *DeferredContext {
	return &DeferredContext{
		pendingAccounts: make(map[string][]byte),
		pendingStorages: make(map[string][]byte),
		pendingBranches: make(map[string]pendingBranch),
		step:            0,
	}
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
	c.pendingAccounts[string(plainKey)] = encAccount
}

func (c *DeferredContext) AccountRaw(plainKey []byte) ([]byte, error) {
	if data, ok := c.pendingAccounts[string(plainKey)]; ok {
		c.tracef("ctx.AccountRaw(pending) %x: %x\n", plainKey, data)
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

func (c *DeferredContext) PutStorage(plainKey []byte, encStorage []byte) {
	c.pendingStorages[string(plainKey)] = encStorage
}

func (c *DeferredContext) StorageRaw(plainKey []byte) ([]byte, error) {
	// Read from pending changes and wrap it into Update. See SharedDomainsCommitmentContext.Storage().
	if data, ok := c.pendingStorages[string(plainKey)]; ok {
		c.tracef("ctx.StorageRaw(pending) %x: %x\n", plainKey, data)
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
	c.tracef("ctx.PutBranch %x: %x, %x, %d\n", prefix, data, prevData, prevStep)
	c.pendingBranches[string(prefix)] = pendingBranch{
		data:     data,
		prevData: prevData,
		prevStep: prevStep,
	}
	return nil
}

func (c *DeferredContext) Branch(prefix []byte) ([]byte, uint64, error) {
	if pb, ok := c.pendingBranches[string(prefix)]; ok {
		c.tracef("ctx.Branch(pending) %x: %x, %x, %d\n", prefix, pb.data, pb.prevData, pb.prevStep)
		return pb.data, pb.prevStep, nil
	}

	if c.sd != nil {
		u, step, err := c.sd.GetCommitmentContext().Branch(prefix)
		c.tracef("ctx.Branch(db) %x: %x, %d, %v\n", prefix, u, step, err)
		return u, step, err
	}

	// Branch data does not exist. Return nil.
	return nil, 0, nil
}

// Commit pending changes to database.
func (c *DeferredContext) Commit() error {
	if c.sd == nil {
		return nil
	}

	// Commit pending accounts.
	for addr, acc := range c.pendingAccounts {
		if err := c.sd.DomainPut(kv.AccountsDomain, []byte(addr), nil, acc, nil, 0); err != nil {
			return err
		}
	}

	// Commit pending storages.
	for plainKey, encStorage := range c.pendingStorages {
		if err := c.sd.DomainPut(kv.StorageDomain, []byte(plainKey), nil, encStorage, nil, 0); err != nil {
			return err
		}
	}

	// Commit pending branches.
	for prefix, pb := range c.pendingBranches {
		if err := c.sd.GetCommitmentContext().PutBranch([]byte(prefix), pb.data, pb.prevData, pb.prevStep); err != nil {
			return err
		}
	}

	return nil
}
