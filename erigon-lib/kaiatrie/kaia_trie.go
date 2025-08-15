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
	"github.com/erigontech/erigon-lib/common"
	"github.com/erigontech/erigon-lib/common/length"
	"github.com/erigontech/erigon-lib/state"
)

type KaiaAccountTrie struct {
	domm    *DomainManager                  // final database layer to read and write.
	kctx    *commitment.KaiaPatriciaContext // not-yet committed account, storage, branches with database as fallback.
	hph     *commitment.HexPatriciaHashed   // merkle hash calculator
	updates *commitment.Updates             // not-yet hashed Updates

	readNum  *uint64
	writeNum *uint64

	trace bool // for debugging.
}

func NewKaiaAccountTrie(domm *DomainManager, rootHash common.Hash, writeGenesis bool) (*KaiaAccountTrie, error) {
	isRootEmpty := (rootHash == (common.Hash{}) || rootHash == common.BytesToHash(commitment.EmptyRootHash))
	tmpDir := domm.TmpDir()

	var kctx *commitment.KaiaPatriciaContext
	if isRootEmpty && !writeGenesis {
		// Temporary trie. No DB backend, No commit.
		kctx = commitment.NewKaiaPatriciaContext(commitment.ModeRawBytes, 0)
	} else if isRootEmpty && writeGenesis {
		// Writing to genesis. Read from block 0, commit to block 0.
	} else if !isRootEmpty && !writeGenesis {
		// Writing to existing block. Read from N, commit to N+1.
	} else {
		// Illegal.
		return nil, errors.New("cannot writeGenesis to existing state")
	}

	hph := commitment.NewHexPatriciaHashed(length.Addr, kctx, tmpDir)
	updates := commitment.NewUpdates(commitment.ModeDirect, tmpDir, commitment.KeyToHexNibbleHash)

	return &KaiaAccountTrie{
		domm:    domm,
		updates: updates,
		kctx:    kctx,
		hph:     hph,
	}, nil
}

func (t *KaiaAccountTrie) SetTrace(trace bool) {
	t.trace = trace
}

func (t *KaiaAccountTrie) tracef(format string, args ...any) {
	if t.trace {
		fmt.Printf("kaiatrie: "+format, args...)
	}
}

func (t *KaiaAccountTrie) Update(address common.Address, accountRLP []byte) {
	key := address.Bytes()
	t.tracef("AccountTrie.Update %x = %x\n", key, accountRLP)

	t.updates.TouchPlainKey(string(key), accountRLP, t.updates.TouchAccount)
	t.kctx.PutAccount(key, accountRLP)
}

func (t *KaiaAccountTrie) Get(address common.Address) ([]byte, error) {
	key := address.Bytes()
	t.tracef("AccountTrie.Get %x\n", key)

	acc, err := t.kctx.Account(key)
	if err != nil {
		return nil, err
	}
	return acc.RawBytes, nil
}

func (t *KaiaAccountTrie) Hash() (common.Hash, error) {
	rootHash := common.Hash{}
	t.domm.WithTx(func(sd *state.SharedDomains) (commit bool, err error) {
		t.kctx.SetUnderlyingCtx(sd.GetCommitmentContext())
		defer t.kctx.SetUnderlyingCtx(nil)

		hash, err := t.hph.Process(context.Background(), t.updates, "")
		if err != nil {
			return false, err
		}
		rootHash = common.BytesToHash(hash)
		return false, nil
	})
	return rootHash, nil
}

func (t *KaiaAccountTrie) Commit() (common.Hash, error) {
	rootHash, err := t.Hash()
	if err != nil {
		return common.Hash{}, err
	}

	if t.writeNum == nil {
		// Do not commit anything
		return rootHash, nil
	}

	err = t.domm.WithTx(func(sd *state.SharedDomains) (commit bool, err error) {
		t.kctx.SetUnderlyingCtx(sd.GetCommitmentContext())
		defer t.kctx.SetUnderlyingCtx(nil)
		return true, nil
	})
	return rootHash, err
}
