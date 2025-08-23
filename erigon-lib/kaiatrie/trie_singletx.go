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

	"github.com/erigontech/erigon-lib/kv"
	"github.com/erigontech/erigon-lib/state"
)

var (
	_ Trie = (*SingleTxAccountTrie)(nil)
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

func (t *SingleTxAccountTrie) hash() ([]byte, error) {
	return t.sd.ComputeCommitment(context.Background(), true, t.sd.BlockNum(), "")
}

func (t *SingleTxAccountTrie) Hash() []byte {
	h, err := t.hash()
	if err != nil {
		return []byte{}
	}
	return h
}

func (t *SingleTxAccountTrie) Commit() ([]byte, error) {
	h, err := t.hash()
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
