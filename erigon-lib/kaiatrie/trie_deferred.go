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
		return t.ctx.Commit()
	})
	return h, err
}

type DeferredStorageTrie struct {
	dm   *DomainsManager
	ctx  *DeferredContext
	addr []byte

	roNum uint64
	rwNum uint64
}

func NewDeferredStorageTrie(dm *DomainsManager, addr []byte, blockNum uint64, genesis bool, accountMode AccountMode) *DeferredStorageTrie {
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
		acc, _ := ctx.AccountRaw(addr)
		if acc == nil {
			// Add a surrogate account so HPH can calculate the storage root hash for this account even if
			// the account does not exist just yet. Usually happens in contract deployment transaction's constructor().
			ctx.PutAccount(addr, emptyEncAccountE3)
		}
		return nil
	})

	return &DeferredStorageTrie{
		dm:    dm,
		ctx:   ctx,
		addr:  addr,
		roNum: roNum,
		rwNum: rwNum,
	}
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
	storageRoot := t.ctx.trie.LastStorageRootHash(t.addr)
	if len(storageRoot) == 0 {
		return nil, fmt.Errorf("%w: addr=%x", errNoStorageRoot, t.addr)
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
