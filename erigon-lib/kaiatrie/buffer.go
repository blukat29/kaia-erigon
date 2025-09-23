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
	"encoding/binary"
	"strings"

	"github.com/erigontech/erigon-lib/common/length"
	"github.com/erigontech/erigon-lib/kv"
	"github.com/tidwall/btree"
)

type WriteBuffer struct {
	keyLen int
	txNum  uint64                     // current tx num
	m      *btree.Map[string, []byte] // (key || txNum) => value
}

func NewWriteBuffer(keyLen int) *WriteBuffer {
	return &WriteBuffer{
		keyLen: keyLen,
		m:      btree.NewMap[string, []byte](128),
	}
}

func (wb *WriteBuffer) SetTxNum(txNum uint64) {
	wb.txNum = txNum
}

func (wb *WriteBuffer) Put(key, value []byte) {
	wb.m.Set(wb.makeBufferKey(key, wb.txNum), value)
}

func (wb *WriteBuffer) GetAsOf(key []byte, txNum uint64) ([]byte, bool) {
	var (
		bottomKey = wb.makeBufferKey(key, 0)
		searchKey = wb.makeBufferKey(key, txNum)
		result    = []byte(nil)
		ok        = false
	)
	wb.m.Descend(searchKey, func(iterKey string, value []byte) bool {
		if strings.Compare(bottomKey, iterKey) <= 0 {
			result = value
			ok = true
		}
		return false // stop Descend
	})
	return result, ok
}

func (wb *WriteBuffer) makeBufferKey(key []byte, txNum uint64) string {
	buf := make([]byte, wb.keyLen+8)
	if wb.keyLen <= len(key) {
		copy(buf, key[:wb.keyLen])
	} else {
		copy(buf, key)
	}
	binary.BigEndian.PutUint64(buf[wb.keyLen:], txNum)
	return string(buf)
}

type DomainsWriteBuffer struct {
	buffers [kv.DomainLen]*WriteBuffer
}

func NewDomainsWriteBuffer() *DomainsWriteBuffer {
	return &DomainsWriteBuffer{
		buffers: [kv.DomainLen]*WriteBuffer{
			NewWriteBuffer(length.Addr), // AccountsDomain
			NewWriteBuffer(length.Hash), // StorageDomain
			nil,                         // CodeDomain
			NewWriteBuffer(128),         // CommitmentDomain
			nil,                         // ReceiptDomain
			nil,                         // RCacheDomain
		},
	}
}

func (dwb *DomainsWriteBuffer) SetTxNum(txNum uint64) {
	for _, buffer := range dwb.buffers {
		if buffer != nil {
			buffer.SetTxNum(txNum)
		}
	}
}

func (dwb *DomainsWriteBuffer) Put(domain kv.Domain, key, value []byte) {
	if dwb.buffers[domain] != nil {
		dwb.buffers[domain].Put(key, value)
	}
}

func (dwb *DomainsWriteBuffer) GetAsOf(domain kv.Domain, key []byte, txNum uint64) ([]byte, bool) {
	if dwb.buffers[domain] != nil {
		return dwb.buffers[domain].GetAsOf(key, txNum)
	}
	return nil, false
}
