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

	"github.com/erigontech/erigon-lib/common/hexutil"
	"github.com/erigontech/erigon-lib/kv"
	"github.com/erigontech/erigon-lib/state"
)

var (
	_ Trie = (*SingleTxAccountTrie)(nil)

	emptyEncAccountE3 = hexutil.MustDecode("0x00000000") // accounts.SerialiseV3(&accounts.Account{})
	emptyRoot         = hexutil.MustDecode("0x56e81f171bcc55a6ff8345e692c0f86e5b48e01b996cadc001622fb5e363b421")
)

type SingleTxAccountTrie struct {
	sd *state.SharedDomains
}

func NewSingleTxAccountTrie(sd *state.SharedDomains) *SingleTxAccountTrie {
	return &SingleTxAccountTrie{sd: sd}
}

func (t *SingleTxAccountTrie) Get(key []byte) ([]byte, error) {
	return t.sd.GetCommitmentContext().AccountRaw(key)
}

func (t *SingleTxAccountTrie) Update(key []byte, value []byte) error {
	return t.sd.DomainPut(kv.AccountsDomain, key, nil, value, nil, 0)
}

func (t *SingleTxAccountTrie) Delete(key []byte) error {
	return t.sd.DomainDel(kv.AccountsDomain, key, nil, nil, 0)
}

func (t *SingleTxAccountTrie) Hash() ([]byte, error) {
	return t.sd.ComputeCommitment(context.Background(), true, t.sd.BlockNum(), "")
}

func (t *SingleTxAccountTrie) Commit() ([]byte, error) {
	h, err := t.Hash()
	if err != nil {
		return nil, err
	}

	rwTx := t.sd.Tx().(kv.RwTx)
	t.sd.Flush(context.Background(), rwTx)
	if err := rwTx.Commit(); err != nil {
		return nil, err
	}

	return h, nil
}

type SingleTxStorageTrie struct {
	sd   *state.SharedDomains
	addr []byte
}

func NewSingleTxStorageTrie(sd *state.SharedDomains, addr []byte) *SingleTxStorageTrie {
	encAccount, _ := sd.GetCommitmentContext().AccountRaw(addr)
	if encAccount == nil {
		// Add a surrogate account so HPH can calculate the storage root hash for this account even if
		// the account does not exist just yet. Usually happens in contract deployment transaction's constructor().
		sd.DomainPut(kv.AccountsDomain, addr, nil, emptyEncAccountE3, nil, 0)
	}
	return &SingleTxStorageTrie{sd: sd, addr: addr}
}

func (t *SingleTxStorageTrie) Get(key []byte) ([]byte, error) {
	u, err := t.sd.GetCommitmentContext().Storage(t.storageKey(key))
	if err != nil {
		return nil, err
	}
	return u.Storage[:], nil
}

func (t *SingleTxStorageTrie) Update(key []byte, value []byte) error {
	return t.sd.DomainPut(kv.StorageDomain, t.storageKey(key), nil, value, nil, 0)
}

func (t *SingleTxStorageTrie) Delete(key []byte) error {
	return t.sd.DomainDel(kv.StorageDomain, t.storageKey(key), nil, nil, 0)
}

func (t *SingleTxStorageTrie) Hash() ([]byte, error) {
	_, err := t.sd.ComputeCommitment(context.Background(), true, t.sd.BlockNum(), "")
	if err != nil {
		return nil, err
	}
	// Note that LastStorageRootHash is only filled if there was a storage update.
	// In DeferredTrie, we are given the previous storage root hash via the OpenTrie argument, so we can return it.
	// But SingleTxStorageTrie doesn't have that. That is okay because SingleTxStorageTrie is only used for testing.
	storageRoot := t.sd.GetCommitmentContext().Trie().LastStorageRootHash(t.addr)
	if len(storageRoot) == 0 {
		return nil, errors.New("storage root hash not calculated")
	}
	return storageRoot, nil
}

func (t *SingleTxStorageTrie) Commit() ([]byte, error) {
	h, err := t.Hash()
	if err != nil {
		return nil, err
	}

	rwTx := t.sd.Tx().(kv.RwTx)
	t.sd.Flush(context.Background(), rwTx)
	if err := rwTx.Commit(); err != nil {
		return nil, err
	}

	return h, nil
}

func (t *SingleTxStorageTrie) storageKey(key []byte) []byte {
	return append(t.addr, key...)
}
