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
	"bytes"
	"errors"
	"fmt"

	"github.com/erigontech/erigon-lib/commitment"
	"github.com/erigontech/erigon-lib/common"
	"github.com/erigontech/erigon-lib/common/hexutil"
	"github.com/erigontech/erigon-lib/rlp"
)

var (
	_ Trie = (*DeferredAccountTrie)(nil)

	emptyEncAccountE3 = hexutil.MustDecode("0x00000000") // accounts.SerialiseV3(&accounts.Account{})
	errNoStorageRoot  = errors.New("storage root hash not calculated")
)

type Trie interface {
	Get(key []byte) ([]byte, error)
	Put(key []byte, value []byte) error
	Hash() ([]byte, error)
	Commit() ([]byte, error)
}

type DeferredAccountTrie struct {
	dm  *DomainsManager
	ctx *DeferredContext
}

// Creates an account trie that begins with the given block number.
func NewDeferredAccountTrie(dm *DomainsManager, blockNum uint64, writeGenesis bool) *DeferredAccountTrie {
	var roNum, rwNum uint64
	if writeGenesis {
		roNum = 0
		rwNum = 0
	} else {
		roNum = blockNum
		rwNum = blockNum + 1
	}
	ctx := NewDeferredContext(dm, dm.Tmpdir(), ModeRawBytes, roNum, rwNum)
	return &DeferredAccountTrie{
		dm:  dm,
		ctx: ctx,
	}
}

func (at *DeferredAccountTrie) Get(key []byte) ([]byte, error) {
	return at.ctx.GetAccount(key)
}

func (at *DeferredAccountTrie) Put(key []byte, value []byte) error {
	at.ctx.PutAccount(key, value)
	return nil
}

func (at *DeferredAccountTrie) Hash() ([]byte, error) {
	return at.ctx.Hash()
}

func (at *DeferredAccountTrie) Commit() ([]byte, error) {
	h, err := at.Hash()
	if err != nil {
		return nil, err
	}
	err = at.ctx.Commit()
	return h, err
}

type DeferredStorageTrie struct {
	at   *DeferredAccountTrie
	addr common.Address

	initialRoot      common.Hash
	updated          bool
	mayNeedSurrogate bool
}

func NewDeferredStorageTrie(at *DeferredAccountTrie, addr, storageRoot []byte) *DeferredStorageTrie {
	return &DeferredStorageTrie{
		at:   at,
		addr: common.BytesToAddress(addr),

		initialRoot:      normalizeRootHash(storageRoot),
		updated:          false,
		mayNeedSurrogate: true,
	}
}

func (st *DeferredStorageTrie) Get(key []byte) ([]byte, error) {
	return st.at.ctx.GetStorage(storageKey(st.addr, key))
}

func (st *DeferredStorageTrie) GetRLP(key []byte) ([]byte, error) {
	value, err := st.at.ctx.GetStorage(storageKey(st.addr, key))
	if err != nil {
		return nil, err
	}
	valueRLP, err := rlp.EncodeToBytes(bytes.TrimLeft(value[:], "\x00"))
	if err != nil {
		return nil, fmt.Errorf("rlp encode failed: %w", err)
	}
	return valueRLP, nil
}

func (st *DeferredStorageTrie) Put(key []byte, value []byte) error {
	if st.mayNeedSurrogate {
		if acc, err := st.at.ctx.GetAccount(st.addr.Bytes()); err != nil {
			return err
		} else if len(acc) == 0 {
			// Add a surrogate account so HPH can calculate the storage root hash for this account even if
			// the account does not exist just yet. Usually happens in contract deployment transaction's constructor().
			st.at.ctx.PutAccount(st.addr.Bytes(), surrogateAccount(st.at.ctx.accountMode))
		}
		st.mayNeedSurrogate = false
	}

	st.updated = true
	st.at.ctx.PutStorage(storageKey(st.addr, key), value)
	return nil
}

func (st *DeferredStorageTrie) PutRLP(key []byte, valueRLP []byte) error {
	if len(valueRLP) == 0 {
		return st.Put(key, nil)
	}
	_, value, _, err := rlp.Split(valueRLP)
	if err != nil {
		return fmt.Errorf("rlp decode failed: %w", err)
	}
	st.Put(key, value)
	return nil
}

func (st *DeferredStorageTrie) Hash() ([]byte, error) {
	if !st.updated {
		return st.initialRoot[:], nil
	}
	h, err := st.at.ctx.StorageRootHash(st.addr.Bytes())
	if len(h) == 0 {
		return nil, fmt.Errorf("%w: addr=%x", errNoStorageRoot, st.addr)
	}
	return h, err
}

func (st *DeferredStorageTrie) Commit() ([]byte, error) {
	h, err := st.Hash()
	if err != nil {
		return nil, err
	}
	err = st.at.ctx.Commit()
	return h, err
}

func storageKey(addr common.Address, key []byte) []byte {
	return append(addr.Bytes(), key...)
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

func normalizeRootHash(root []byte) common.Hash {
	if bytes.Equal(root, make([]byte, 32)) {
		return common.BytesToHash(commitment.EmptyRootHash)
	}
	if len(root) == 0 {
		return common.BytesToHash(commitment.EmptyRootHash)
	}
	return common.BytesToHash(root)
}
