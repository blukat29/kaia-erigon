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

	"github.com/erigontech/erigon-lib/common"
	"github.com/erigontech/erigon-lib/state"
)

var (
	_ Trie = (*DeferredAccountTrie)(nil)
	_ Trie = (*DeferredStorageTrie)(nil)
)

type DeferredAccountTrie struct {
	dm  *DomainsManager
	ctx *DeferredContext

	roNum uint64
	rwNum uint64
}

func NewDeferredAccountTrie(dm *DomainsManager, blockNum uint64, genesis bool, accountMode AccountMode) *DeferredAccountTrie {
	ctx := NewDeferredContext(dm.dirs.Tmp, accountMode)

	var roNum, rwNum uint64
	if genesis {
		roNum = 0
		rwNum = 0
	} else {
		roNum = blockNum
		rwNum = blockNum + 1
	}

	return &DeferredAccountTrie{
		dm:    dm,
		ctx:   ctx,
		roNum: roNum,
		rwNum: rwNum,
	}
}

func (t *DeferredAccountTrie) SetTrace(trace bool) {
	t.ctx.SetTrace(trace)
}

func (t *DeferredAccountTrie) Get(key []byte) ([]byte, error) {
	var result []byte
	err := t.dm.WithDomainsRo(t.roNum, func(sd *state.SharedDomains) error {
		t.ctx.SetDomains(sd)
		acc, err := t.ctx.AccountRaw(key)
		result = acc
		return err
	})
	return result, err
}

func (t *DeferredAccountTrie) Update(key []byte, value []byte) error {
	t.ctx.PutAccount(key, value)
	return nil
}

func (t *DeferredAccountTrie) Delete(key []byte) error {
	t.ctx.PutAccount(key, nil)
	return nil
}

func (t *DeferredAccountTrie) Hash() ([]byte, error) {
	var result []byte
	err := t.dm.WithDomainsRo(t.roNum, func(sd *state.SharedDomains) error {
		t.ctx.SetDomains(sd)
		rootHash, err := t.ctx.Hash()
		result = rootHash
		return err
	})
	return result, err
}

func (t *DeferredAccountTrie) Commit() ([]byte, error) {
	h, err := t.Hash()
	if err != nil {
		return nil, err
	}
	err = t.dm.WithDomainsRw(t.rwNum, func(sd *state.SharedDomains) error {
		t.ctx.SetDomains(sd)
		if err := t.ctx.Commit(); err != nil {
			return err
		}
		return WriteBlockNumByRoot(sd, h, t.rwNum)
	})
	return h, err
}

type DeferredStorageTrie struct {
	dm          *DomainsManager
	ctx         *DeferredContext
	addr        common.Address
	initialRoot common.Hash

	roNum uint64
	rwNum uint64

	updated bool // true if the storage trie has ever been updated since construction.
}

func NewDeferredStorageTrie(dm *DomainsManager, addrB, storageRoot []byte, blockNum uint64, genesis bool, accountMode AccountMode) *DeferredStorageTrie {
	addr := common.BytesToAddress(addrB)
	ctx := NewDeferredContext(dm.dirs.Tmp, accountMode)

	var roNum, rwNum uint64
	if genesis {
		roNum = 0
		rwNum = 0
	} else {
		roNum = blockNum
		rwNum = blockNum + 1
	}

	dm.WithDomainsRo(roNum, func(sd *state.SharedDomains) error {
		ctx.SetDomains(sd)
		acc, _ := ctx.AccountRaw(addr.Bytes())
		if acc == nil {
			// Add a surrogate account so HPH can calculate the storage root hash for this account even if
			// the account does not exist just yet. Usually happens in contract deployment transaction's constructor().
			ctx.PutAccount(addr.Bytes(), surrogateAccount(accountMode))
		}
		return nil
	})

	return &DeferredStorageTrie{
		dm:          dm,
		ctx:         ctx,
		addr:        addr,
		initialRoot: common.BytesToHash(storageRoot),
		roNum:       roNum,
		rwNum:       rwNum,
	}
}

func surrogateAccount(accountMode AccountMode) []byte {
	switch accountMode {
	case ModeErigonV3:
		// A valid ErigonV3 (SerialiseV3) account.
		return emptyEncAccountE3
	case ModeRawBytes:
		// Actually it doesn't matter in ModeRawBytes because this account data is never decoded and instead treated as an opaque value
		// in HexPatriciaHashed, Updates, and DeferredContext. Therefore it's safe to use emptyEncAccountE3 for ModeRawBytes.
		return emptyEncAccountE3
	}
	return nil
}

func (t *DeferredStorageTrie) SetTrace(trace bool) {
	t.ctx.SetTrace(trace)
	t.ctx.trie.SetTrace(trace)
}

func (t *DeferredStorageTrie) Get(key []byte) ([]byte, error) {
	var result []byte
	err := t.dm.WithDomainsRo(t.roNum, func(sd *state.SharedDomains) error {
		t.ctx.SetDomains(sd)
		acc, err := t.ctx.StorageRaw(storageKey(t.addr, key))
		result = acc
		return err
	})
	return result, err
}

func (t *DeferredStorageTrie) Update(key []byte, value []byte) error {
	t.ctx.PutStorage(storageKey(t.addr, key), value)
	return nil
}

func (t *DeferredStorageTrie) Delete(key []byte) error {
	t.ctx.PutStorage(storageKey(t.addr, key), nil)
	return nil
}

func (t *DeferredStorageTrie) Hash() ([]byte, error) {
	t.updated = t.updated || (t.ctx.pendingUpdates.Size() > 0) // If *ever* been updated.

	// Compute state root hash. Storage root hashes are calculated in the process.
	err := t.dm.WithDomainsRo(t.roNum, func(sd *state.SharedDomains) error {
		t.ctx.SetDomains(sd)
		_, err := t.ctx.Hash()
		return err
	})
	if err != nil {
		return nil, err
	}

	// Harvest the storage root hash, which is byproduct of the state root hash calculation.
	storageRoot := t.ctx.trie.LastStorageRootHash(t.addr.Bytes())
	if len(storageRoot) == 0 { // trie didn't calculate the storage root hash.
		if t.updated {
			// If there was an update but the storage root hash is not calculated, something is wrong.
			return nil, fmt.Errorf("%w: addr=%x", errNoStorageRoot, t.addr)
		} else {
			// If the storage trie has never been updated, it is normal that storage root hash is not calculated.
			// Return the initial storage root hash.
			return t.initialRoot.Bytes(), nil
		}
	}
	return storageRoot, nil
}

func (t *DeferredStorageTrie) Commit() ([]byte, error) {
	h, err := t.Hash()
	if err != nil {
		return nil, err
	}
	err = t.dm.WithDomainsRw(t.rwNum, func(sd *state.SharedDomains) error {
		t.ctx.SetDomains(sd)
		return t.ctx.Commit()
	})
	return h, err
}
