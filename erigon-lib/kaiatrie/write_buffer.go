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

	"github.com/erigontech/erigon-lib/kv"
	"github.com/tidwall/btree"
)

type WriteBuffer struct {
	txNum uint64                     // current tx num
	m     *btree.Map[string, []byte] // (key || txNum) => value
}

func NewWriteBufferFixedLen() *WriteBuffer {
	return &WriteBuffer{
		m: btree.NewMap[string, []byte](128),
	}
}

func (b *WriteBuffer) SetTxNum(txNum uint64) {
	b.txNum = txNum
}

func (b *WriteBuffer) Clear() {
	b.m.Clear()
}

func (b *WriteBuffer) Put(key, value []byte) {
	b.m.Set(b.makeBufferKey(key, b.txNum), value)
}

func (b *WriteBuffer) GetAsOf(key []byte, txNum uint64) ([]byte, bool) {
	var (
		bottomKey = b.makeBufferKey(key, 0)
		searchKey = b.makeBufferKey(key, txNum)
		result    = []byte(nil)
		ok        = false
	)
	b.m.Descend(searchKey, func(iterKey string, value []byte) bool {
		if strings.Compare(bottomKey, iterKey) <= 0 {
			result = value
			ok = true
		}
		return false // stop Descend
	})
	return result, ok
}

// [keyLen][key][txNum]
func (b *WriteBuffer) makeBufferKey(key []byte, txNum uint64) string {
	keyLen := []byte{byte(len(key))}
	numBuf := make([]byte, 8)
	binary.BigEndian.PutUint64(numBuf, txNum)
	buf := append(append(keyLen, key...), numBuf...)
	return string(buf)
}

type DomainsWriteBuffer struct {
	buffers [kv.DomainLen]*WriteBuffer
}

func NewDomainsWriteBuffer() *DomainsWriteBuffer {
	buf := &DomainsWriteBuffer{}
	for i := range kv.DomainLen {
		buf.buffers[i] = NewWriteBufferFixedLen()
	}
	return buf
}

func (buf *DomainsWriteBuffer) SetTxNum(txNum uint64) {
	for _, b := range buf.buffers {
		if b != nil {
			b.SetTxNum(txNum)
		}
	}
}

func (buf *DomainsWriteBuffer) Put(domain kv.Domain, key, value []byte) {
	if buf.buffers[domain] != nil {
		buf.buffers[domain].Put(key, value)
	}
}

func (buf *DomainsWriteBuffer) GetAsOf(domain kv.Domain, key []byte, txNum uint64) ([]byte, bool) {
	if buf.buffers[domain] != nil {
		return buf.buffers[domain].GetAsOf(key, txNum)
	}
	return nil, false
}

func (buf *DomainsWriteBuffer) Clear() {
	for _, b := range buf.buffers {
		if b != nil {
			b.Clear()
		}
	}
}
