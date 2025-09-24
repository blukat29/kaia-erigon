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
	"math/big"
	"os/exec"
	"slices"
	"strings"
	"testing"

	"github.com/erigontech/erigon-lib/common"
	"github.com/erigontech/erigon-lib/common/hexutil"
	"github.com/erigontech/erigon-lib/kv"
	"github.com/erigontech/erigon-lib/log/v3"
	"github.com/erigontech/erigon-lib/types/accounts"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_DomainsManager_BlockNums(t *testing.T) {
	dm, err := NewTemporaryDomainsManager(t.TempDir())
	require.NoError(t, err)
	defer dm.Close()

	noop := func(writer DomainsWriter) error { return nil }

	// Commit blocks in order.
	assert.NoError(t, dm.WithWriter(0, noop))
	assert.NoError(t, dm.WithWriter(1, noop))
	assert.NoError(t, dm.WithWriter(2, noop))

	// Cannot commit a block less than last block.
	assert.ErrorIs(t, dm.withWriter_workerThread(1, noop), errCommitBlockTooLow)
	// Cannot commit a block with a gap from the last block.
	assert.ErrorIs(t, dm.WithWriter(4, noop), errCommitBlockTooHigh)

	// Permitted to commit the last block again.
	assert.NoError(t, dm.WithWriter(2, noop))
	// Permitted to commit the block right after the last block.
	assert.NoError(t, dm.WithWriter(3, noop))
}

func Test_DomainsManager_Reopen(t *testing.T) {
	var (
		addr   = common.HexToAddress("0x1111111111111111111111111111111111111111").Bytes()
		acc    = accounts.SerialiseV3(&accounts.Account{Balance: *uint256.NewInt(90)})
		dir    = t.TempDir()
		logger = log.Root()
	)

	// Write and close.
	dm, err := NewDomainsManager(dir, logger)
	require.NoError(t, err)
	dm.WithWriter(0, func(writer DomainsWriter) error {
		writer.DomainPutOrDel(kv.AccountsDomain, addr, acc)
		return nil
	})
	dm.Close()

	// Reopen and read.
	dm, err = NewDomainsManager(dir, logger)
	require.NoError(t, err)
	dm.WithReader(func(reader DomainsReader) error {
		actualAcc, err := reader.DomainGetAsOf(kv.AccountsDomain, addr, 0)
		assert.NoError(t, err)
		assert.Equal(t, acc, actualAcc)
		return nil
	})
	dm.Close()
}

func Test_DomainsManager_Iterator(t *testing.T) {
	dm, err := NewTemporaryDomainsManager(t.TempDir())
	require.NoError(t, err)
	defer dm.Close()

	var (
		accounts = [][2]string{
			{"0x1111111111111111111111111111111111111111", "0x00015a0000"},
			{"0x2222222222222222222222222222222222222222", "0x00015b0000"},
			{"0x3333333333333333333333333333333333333333", "0x00015c0000"},
		}

		contractAddr = common.HexToAddress("0x1111111111111111111111111111111111111111").Bytes()
		storage      = [][2]string{
			{"0x0000000000000000000000000000000000000000000000000000000000000001", "0x12"},
			{"0x0000000000000000000000000000000000000000000000000000000000000002", "0x23"},
			{"0x0000000000000000000000000000000000000000000000000000000000000003", "0x34"},
		}
	)

	// Commit across 3 blocks.
	for i := 0; i < 3; i++ {
		addr, acc := hexutil.MustDecode(accounts[i][0]), hexutil.MustDecode(accounts[i][1])
		slot, data := hexutil.MustDecode(storage[i][0]), hexutil.MustDecode(storage[i][1])
		require.NoError(t, dm.WithWriter(uint64(i), func(writer DomainsWriter) error {
			writer.DomainPutOrDel(kv.AccountsDomain, addr, acc)
			writer.DomainPutOrDel(kv.StorageDomain, append(contractAddr, slot...), data)
			return nil
		}))
	}

	it, err := NewAccountIterator(dm, 0)
	require.NoError(t, err)
	defer it.Close()
	checkIt(t, it, accounts[:1]) // only 1 account is iterated at block 0.

	it, err = NewAccountIterator(dm, 1)
	require.NoError(t, err)
	defer it.Close()
	checkIt(t, it, accounts[:2])

	it, err = NewAccountIterator(dm, 2)
	require.NoError(t, err)
	defer it.Close()
	checkIt(t, it, accounts[:3])

	it, err = NewStorageIterator(dm, contractAddr, 2)
	require.NoError(t, err)
	defer it.Close()
	checkIt(t, it, storage)
}

func checkIt(t *testing.T, it DomainsIterator, items [][2]string) {
	// Sort items by key.
	expected := slices.Clone(items)
	slices.SortFunc(expected, func(a, b [2]string) int {
		return strings.Compare(a[0], b[0])
	})

	for _, item := range expected {
		expectedKey, expectedValue := hexutil.MustDecode(item[0]), hexutil.MustDecode(item[1])
		key, value, ok, err := it.Next()
		require.NoError(t, err)
		assert.Equal(t, true, ok)
		assert.Equal(t, expectedKey, key)
		assert.Equal(t, expectedValue, value)
	}
	key, value, ok, err := it.Next()
	assert.NoError(t, err)
	assert.False(t, ok)
	assert.Nil(t, key)
	assert.Nil(t, value)
}

func Benchmark_Reader(b *testing.B) {
	dm, err := NewTemporaryDomainsManager(b.TempDir())
	require.NoError(b, err)
	defer dm.Close()

	var (
		addr = common.HexToAddress("0x1111111111111111111111111111111111111111").Bytes()
		acc  = accounts.SerialiseV3(&accounts.Account{Balance: *uint256.NewInt(90)})
	)

	// Commit some data
	require.NoError(b, dm.WithWriter(0, func(writer DomainsWriter) error {
		writer.DomainPutOrDel(kv.AccountsDomain, addr, acc)
		return nil
	}))

	b.Run("keep RoTx open", func(b *testing.B) {
		dm.WithReader(func(reader DomainsReader) error {
			for i := 0; i < b.N; i++ {
				reader.DomainGetAsOf(kv.AccountsDomain, addr, 0)
			}
			return nil
		})
	})

	b.Run("open RoTx every read", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			dm.withReader_callerThread(func(reader DomainsReader) error {
				reader.DomainGetAsOf(kv.AccountsDomain, addr, 0)
				return nil
			})
		}
	})

	b.Run("reuse RoTx in thread pool", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			dm.withReader_workerThread(func(reader DomainsReader) error {
				reader.DomainGetAsOf(kv.AccountsDomain, addr, 0)
				return nil
			})
		}
	})
}

func Benchmark_Writer(b *testing.B) {
	putItems := func(writer DomainsWriter, num, count int) {
		for i := range count {
			n := int64(num*1000 + i)
			k := common.BigToAddress(big.NewInt(n)).Bytes()
			v := common.BigToHash(big.NewInt(n)).Bytes()
			writer.DomainPutOrDel(CustomDomain, k, v)
		}
	}

	b.Run("open RwTx every block", func(b *testing.B) {
		dm, err := NewDomainsManager(b.TempDir(), log.Root())
		require.NoError(b, err)
		defer dm.Close()

		for i := 0; i < b.N; i++ {
			writer, _ := NewDomainsWriter(dm.db, dm.agg, NewDomainsWriteBuffer())
			writer.SetBlockNum(uint64(i))
			putItems(writer, i, 10)
			writer.WriteBlockNum(uint64(i))
			writer.Commit()
		}

		out, _ := exec.Command("du", "-sh", dm.dirs.DataDir).Output()
		b.Logf("N=%d datadir=%s", b.N, out)
	})

	b.Run("reuse RwTx", func(b *testing.B) {
		dm, err := NewDomainsManager(b.TempDir(), log.Root())
		require.NoError(b, err)
		defer dm.Close()

		writer, _ := NewDomainsWriter(dm.db, dm.agg, NewDomainsWriteBuffer())
		for i := 0; i < b.N; i++ {
			writer.SetBlockNum(uint64(i))
			putItems(writer, i, 10)
			writer.WriteBlockNum(uint64(i))
			// periodically commit and reopen
			if i%128 == 127 {
				writer.Commit()
				writer, _ = NewDomainsWriter(dm.db, dm.agg, NewDomainsWriteBuffer())
			}
		}
		writer.Commit()

		out, _ := exec.Command("du", "-sh", dm.dirs.DataDir).Output()
		b.Logf("N=%d datadir=%s", b.N, out)
	})
}
