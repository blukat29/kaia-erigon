// Copyright 2021 The go-ethereum Authors
// (original work)
// Copyright 2024 The Erigon Authors
// (modifications)
// This file is part of Erigon.
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
	"testing"

	"github.com/erigontech/erigon-lib/kv"
	"github.com/stretchr/testify/assert"
)

func Test_WriteBufferFixedLen_EdgeCases(t *testing.T) {
	wb := NewWriteBufferFixedLen()
	wb.SetTxNum(1)

	// Normal case
	wb.Put([]byte("1111"), []byte("vvvv"))
	v, ok := wb.GetAsOf([]byte("1111"), 1)
	assert.True(t, ok)
	assert.Equal(t, []byte("vvvv"), v)

	// Nonexistent keys
	_, ok = wb.GetAsOf([]byte("1110"), 0)
	assert.False(t, ok)
	_, ok = wb.GetAsOf([]byte("1112"), 0)
	assert.False(t, ok)

	// Short key
	wb.Put([]byte("111"), []byte("vvv"))
	v, ok = wb.GetAsOf([]byte("111"), 1)
	assert.True(t, ok)
	assert.Equal(t, []byte("vvv"), v)
}

func Test_WriteVufferFixedLen_TxNums(t *testing.T) {
	var (
		key = []byte("1111")

		inputs = []struct {
			txNum uint64
			value []byte
		}{
			{3, []byte("a")},
			{5, []byte("bb")},
			{6, []byte("ccc")},
			{8, []byte("dddd")},
		}
		expected = []struct {
			txNum uint64
			value []byte
			ok    bool
		}{
			{0, nil, false},
			{1, nil, false},        // before minTxNum
			{2, nil, false},        // before first data in the buffer
			{3, []byte("a"), true}, // first data
			{4, []byte("a"), true},
			{5, []byte("bb"), true},
			{6, []byte("ccc"), true},
			{7, []byte("ccc"), true},
			{8, []byte("dddd"), true}, // last data
			{9, []byte("dddd"), true}, // after last data in the buffer
		}
	)

	wb := NewWriteBufferFixedLen()

	for _, input := range inputs {
		wb.SetTxNum(input.txNum)
		wb.Put(key, input.value)
	}

	for _, test := range expected {
		wb.SetTxNum(test.txNum)
		value, ok := wb.GetAsOf(key, test.txNum)
		assert.Equal(t, test.value, value, test.txNum)
		assert.Equal(t, test.ok, ok, test.txNum)
	}
}

func Test_DomainsWriteBuffer_ZeroKeys(t *testing.T) {
	// Zero-byte keys occurs in CommitmentDomain to store Branches. prefix 0x00 and 0x0000 are different.
	wb := NewWriteBufferFixedLen()

	k1 := []byte{0}
	k2 := []byte{0, 0}
	k3 := []byte{0, 0, 0}

	wb.Put(k1, []byte("x"))
	wb.Put(k2, []byte("y"))

	v, ok := wb.GetAsOf(k1, 1)
	assert.True(t, ok)
	assert.Equal(t, []byte("x"), v)

	v, ok = wb.GetAsOf(k2, 1)
	assert.True(t, ok)
	assert.Equal(t, []byte("y"), v)

	v, ok = wb.GetAsOf(k3, 1)
	assert.False(t, ok)
	assert.Nil(t, v)
}

func Test_DomainsWriteBuffer(t *testing.T) {
	dwb := NewDomainsWriteBuffer()
	dwb.SetTxNum(1)

	// AccountsDomain
	dwb.Put(kv.AccountsDomain, []byte("1111"), []byte("x"))
	v, ok := dwb.GetAsOf(kv.AccountsDomain, []byte("1111"), 1)
	assert.True(t, ok)
	assert.Equal(t, []byte("x"), v)

	// StorageDomain
	dwb.Put(kv.StorageDomain, []byte("1111"), []byte("y"))
	v, ok = dwb.GetAsOf(kv.StorageDomain, []byte("1111"), 1)
	assert.True(t, ok)
	assert.Equal(t, []byte("y"), v)

	// CommitmentDomain
	dwb.Put(kv.CommitmentDomain, []byte("1111"), []byte("z"))
	v, ok = dwb.GetAsOf(kv.CommitmentDomain, []byte("1111"), 1)
	assert.True(t, ok)
	assert.Equal(t, []byte("z"), v)

	// ReceiptDomain
	dwb.Put(kv.ReceiptDomain, []byte("1111"), []byte("w"))
	v, ok = dwb.GetAsOf(kv.ReceiptDomain, []byte("1111"), 1)
	assert.True(t, ok)
	assert.Equal(t, []byte("w"), v)
}
