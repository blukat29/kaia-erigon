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

package commitment

import (
	"errors"
	"fmt"

	"github.com/erigontech/erigon-lib/types/accounts"
)

var (
	_ PatriciaContext = (*KaiaPatriciaContext)(nil)
)

type AccountMode uint8

const (
	ModeErigonV3 AccountMode = 0
	ModeRawBytes AccountMode = 1
)

type KaiaPatriciaContext struct {
	trace bool // For debugging.

	// Optional DB-backed PatriciaContext that reads from and write to the database.
	// If nil, KaiaPatriciaContext is an ephemeral in-memory context.
	dbCtx PatriciaContext

	mode AccountMode
	step uint64

	pendingAccounts map[string][]byte // addr[20] => SerialiseV3 (ModeErigonV3) or RawBytes (ModeRawBytes)
	pendingStorages map[string][]byte // addr[20] || slot[32] => data[32]
	pendingBranches map[string][]byte // prefix[] => data[]
}

func NewKaiaPatriciaContext(mode AccountMode, step uint64) *KaiaPatriciaContext {
	return &KaiaPatriciaContext{
		mode:            mode,
		step:            step,
		pendingAccounts: make(map[string][]byte),
		pendingStorages: make(map[string][]byte),
		pendingBranches: make(map[string][]byte),
	}
}

// Must accompany with SetUnderlyingCtx(nil).
func (kc *KaiaPatriciaContext) SetUnderlyingCtx(dbCtx PatriciaContext) {
	kc.dbCtx = dbCtx
}

func (kc *KaiaPatriciaContext) Branch(prefix []byte) ([]byte, uint64, error) {
	if data, ok := kc.pendingBranches[string(prefix)]; ok {
		kc.tracef("kc.Branch(pending) %x: %x, %d\n", prefix, data, kc.step)
		return data, kc.step, nil
	}

	if kc.dbCtx != nil {
		u, step, err := kc.dbCtx.Branch(prefix)
		kc.tracef("kc.Branch(db) %x: %x, %d, %v\n", prefix, u, step, err)
		return u, step, err
	}

	// Branch data does not exist. Return nil.
	return nil, 0, nil
}

func (kc *KaiaPatriciaContext) PutBranch(prefix []byte, data []byte, prevData []byte, prevStep uint64) error {
	kc.tracef("kc.PutBranch %x: %x, %x, %d\n", prefix, data, prevData, prevStep)

	// Write to pending changes.
	kc.pendingBranches[string(prefix)] = data

	// Write to database.
	if kc.dbCtx != nil {
		return kc.dbCtx.PutBranch(prefix, data, prevData, prevStep)
	}

	// If no database, simply won't commit because KaiaPatriciaContext is acting as a
	// temporary in-memory context.
	return nil
}

func (kc *KaiaPatriciaContext) Account(plainKey []byte) (*Update, error) {
	// Read from pending changes and wrap it into Update. See SharedDomainsCommitmentContext.Account().
	if encAccount, ok := kc.pendingAccounts[string(plainKey)]; ok {
		u, err := kc.getAccountUpdate(encAccount)
		if err != nil {
			return nil, err
		}
		kc.tracef("kc.Account(pending) %x: %x => %v\n", plainKey, encAccount, u.String())
		return u, nil
	}

	// Read from database. dbCtx.Account should include the empty-account fallback.
	if kc.dbCtx != nil {
		u, err := kc.dbCtx.Account(plainKey)
		kc.tracef("kc.Account(db) %x: %x => %v\n", plainKey, u.Storage[:u.StorageLen], u.String())
		return u, err
	}

	// Account does not exist. Return empty account.
	return &Update{CodeHash: EmptyCodeHashArray, Flags: DeleteUpdate}, nil
}

func (kc *KaiaPatriciaContext) getAccountUpdate(encAccount []byte) (*Update, error) {
	if kc.mode == ModeRawBytes {
		u := &Update{CodeHash: EmptyCodeHashArray} // default to empty code hash
		u.Flags = RawBytesUpdate
		u.RawBytes = make([]byte, len(encAccount))
		copy(u.RawBytes, encAccount)
		return u, nil
	}

	if kc.mode == ModeErigonV3 {
		u := &Update{CodeHash: EmptyCodeHashArray} // default to empty code hash
		if len(encAccount) == 0 {
			u.Flags = DeleteUpdate
			return u, nil
		}

		acc := new(accounts.Account)
		if err := accounts.DeserialiseV3(acc, encAccount); err != nil {
			return nil, err
		}

		u.Flags |= NonceUpdate
		u.Nonce = acc.Nonce

		u.Flags |= BalanceUpdate
		u.Balance.Set(&acc.Balance)

		if ch := acc.CodeHash.Bytes(); len(ch) > 0 { // if code hash is not empty
			u.Flags |= CodeUpdate
			copy(u.CodeHash[:], ch)
		}
		return u, nil
	}

	return nil, errors.New("unknown account mode") // should not happen
}

func (kc *KaiaPatriciaContext) Storage(plainKey []byte) (*Update, error) {
	// Read from pending changes and wrap it into Update. See SharedDomainsCommitmentContext.Storage().
	if data, ok := kc.pendingStorages[string(plainKey)]; ok {
		u := &Update{}
		if len(data) == 0 {
			u.Flags = DeleteUpdate
			u.StorageLen = 0
		} else {
			u.Flags = StorageUpdate
			u.StorageLen = len(data)
			copy(u.Storage[:u.StorageLen], data)
		}
		kc.tracef("kc.Storage(pending) %x: %x => %v\n", plainKey, u.Storage[:u.StorageLen], u.String())
		return u, nil
	}

	// Read from database.
	if kc.dbCtx != nil {
		u, err := kc.dbCtx.Storage(plainKey)
		kc.tracef("kc.Storage(db) %x: %x => %v\n", plainKey, u.Storage[:u.StorageLen], u.String())
		return u, err
	}

	// Storage data does not exist. Return empty storage.
	return &Update{Flags: DeleteUpdate}, nil
}

// Injects pending account into the context. Even if this is not committed to the database,
// HexPatriciaHashed will read this pending account.
func (kc *KaiaPatriciaContext) PutAccount(plainKey []byte, data []byte) {
	kc.tracef("kc.PutAccount %x: %x\n", plainKey, data)
	kc.pendingAccounts[string(plainKey)] = data
}

func (kc *KaiaPatriciaContext) DeleteAccount(plainKey []byte) {
	kc.tracef("kc.DeleteAccount %x\n", plainKey)
	delete(kc.pendingAccounts, string(plainKey))
}

func (kc *KaiaPatriciaContext) PendingAccounts() map[string][]byte {
	return kc.pendingAccounts
}

func (kc *KaiaPatriciaContext) putAccountUpdate(plainKey []byte, update *Update) error {
	if update.Flags&DeleteUpdate != 0 {
		kc.DeleteAccount(plainKey)
		return nil
	}

	if kc.mode == ModeRawBytes {
		if update.Flags&RawBytesUpdate == 0 {
			return errors.New("expected RawBytesUpdate in ModeRawBytes, got field-level update")
		}
		kc.PutAccount(plainKey, update.RawBytes)
		return nil
	}

	if kc.mode == ModeErigonV3 {
		if update.Flags&RawBytesUpdate != 0 {
			return errors.New("expected field-level update in ModeErigonV3, got RawBytesUpdate")
		}
		kc.PutAccount(plainKey, accounts.SerialiseV3(&accounts.Account{
			Nonce:    update.Nonce,
			Balance:  update.Balance,
			CodeHash: update.CodeHash,
		}))
		return nil
	}

	return errors.New("unknown account mode") // should not happen
}

// Injects pending storage into the context. Even if this is not committed to the database,
// HexPatriciaHashed will read this pending storage.
func (kc *KaiaPatriciaContext) PutStorage(plainKey []byte, data []byte) {
	kc.tracef("kc.PutStorage %x: %x\n", plainKey, data)
	kc.pendingStorages[string(plainKey)] = data
}

func (kc *KaiaPatriciaContext) DeleteStorage(plainKey []byte) {
	kc.tracef("kc.DeleteStorage %x\n", plainKey)
	delete(kc.pendingStorages, string(plainKey))
}

func (kc *KaiaPatriciaContext) putStorageUpdate(plainKey []byte, update *Update) error {
	if update.Flags&DeleteUpdate != 0 {
		kc.DeleteStorage(plainKey)
		return nil
	}

	kc.PutStorage(plainKey, update.Storage[:update.StorageLen])
	return nil
}

func (kc *KaiaPatriciaContext) tracef(format string, args ...any) {
	if kc.trace {
		fmt.Printf(format, args...)
	}
}

func (kc *KaiaPatriciaContext) setTrace(trace bool) {
	kc.trace = trace
}
