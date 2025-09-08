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
	"slices"
	"strings"
	"testing"

	"github.com/erigontech/erigon-lib/common"
	"github.com/erigontech/erigon-lib/common/hexutil"
	"github.com/erigontech/erigon-lib/kv"
	"github.com/erigontech/erigon-lib/log/v3"
	"github.com/erigontech/erigon-lib/state"
	"github.com/erigontech/erigon-lib/types/accounts"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func checkIt(t *testing.T, it *DomainsIterator, items [][2]string) {
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

func Test_DomainsManager_BlockNums(t *testing.T) {
	noop := func(sd *state.SharedDomains) error { return nil }
	dm, err := NewTemporaryDomainsManager(t.TempDir())
	require.NoError(t, err)
	defer dm.Close()

	// Commit blocks in order.
	assert.NoError(t, dm.WithDomainsRw(0, noop))
	assert.NoError(t, dm.WithDomainsRw(1, noop))
	assert.NoError(t, dm.WithDomainsRw(2, noop))

	// Cannot commit a block less than last block.
	assert.ErrorIs(t, dm.WithDomainsRw(1, noop), errCommitBlockTooLow)
	// Cannot commit a block with a gap from the last block.
	assert.ErrorIs(t, dm.WithDomainsRw(4, noop), errCommitBlockTooHigh)

	// Permitted to commit the last block again.
	assert.NoError(t, dm.WithDomainsRw(2, noop))
	// Permitted to commit the block right after the last block.
	assert.NoError(t, dm.WithDomainsRw(3, noop))
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
	dm.WithDomainsRw(0, func(sd *state.SharedDomains) error {
		sd.DomainPut(kv.AccountsDomain, addr, nil, acc, nil, 0)
		return nil
	})
	dm.Close()

	// Reopen and read.
	dm, err = NewDomainsManager(dir, logger)
	require.NoError(t, err)
	dm.WithDomainsRo(0, func(sd *state.SharedDomains) error {
		actualAcc, err := sd.GetCommitmentContext().AccountRaw(addr)
		assert.NoError(t, err)
		assert.Equal(t, acc, actualAcc)
		return nil
	})
	dm.Close()
}

func Test_DomainsManager_Accounts(t *testing.T) {
	ctx := context.Background()
	dm, err := NewTemporaryDomainsManager(t.TempDir())
	require.NoError(t, err)
	defer dm.Close()

	var (
		addr = common.HexToAddress("0x1111111111111111111111111111111111111111").Bytes()
		accs = map[uint64][]byte{
			0: accounts.SerialiseV3(&accounts.Account{Balance: *uint256.NewInt(90)}),
			1: accounts.SerialiseV3(&accounts.Account{Balance: *uint256.NewInt(91)}),
			2: accounts.SerialiseV3(&accounts.Account{Balance: *uint256.NewInt(92)}),
		}
		hashes = make(map[uint64][]byte)

		commit = func(sd *state.SharedDomains) error {
			n := sd.BlockNum()
			acc := accs[n]
			assert.NoError(t, sd.DomainPut(kv.AccountsDomain, addr, nil, acc, nil, 0), n)

			h, err := sd.ComputeCommitment(ctx, true, n, "")
			assert.NoError(t, err)
			hashes[n] = h
			t.Logf("commit num: %d, hash: %x", n, h)
			return nil
		}
		query = func(sd *state.SharedDomains) error {
			n := sd.BlockNum()
			expectedAcc := accs[n]
			actualAcc, err := sd.GetCommitmentContext().AccountRaw(addr)
			assert.NoError(t, err)
			assert.Equal(t, expectedAcc, actualAcc, n)

			expectedHash := hashes[n]
			actualHash, err := sd.ComputeCommitment(ctx, false, n, "")
			assert.NoError(t, err)
			assert.Equal(t, expectedHash, actualHash, n)
			t.Logf("query num: %d, hash: %x", n, actualHash)
			return nil
		}
	)

	// Commit blocks in order.
	require.NoError(t, dm.WithDomainsRw(0, commit))
	require.NoError(t, dm.WithDomainsRw(1, commit))
	require.NoError(t, dm.WithDomainsRw(2, commit))

	// Query historic account and root hashes.
	require.NoError(t, dm.WithDomainsRo(0, query))
	require.NoError(t, dm.WithDomainsRo(1, query))
	require.NoError(t, dm.WithDomainsRo(2, query))
}

func Test_DomainsManager_RoConcurrent(t *testing.T) {
	dm, err := NewTemporaryDomainsManager(t.TempDir())
	require.NoError(t, err)
	defer dm.Close()

	var (
		accs = map[string][]byte{
			string(common.HexToAddress("0x1111111111111111111111111111111111111111").Bytes()): accounts.SerialiseV3(&accounts.Account{Balance: *uint256.NewInt(90)}),
			string(common.HexToAddress("0x2222222222222222222222222222222222222222").Bytes()): accounts.SerialiseV3(&accounts.Account{Balance: *uint256.NewInt(91)}),
			string(common.HexToAddress("0x3333333333333333333333333333333333333333").Bytes()): accounts.SerialiseV3(&accounts.Account{Balance: *uint256.NewInt(92)}),
		}
	)

	// Commit some data
	require.NoError(t, dm.WithDomainsRw(0, func(sd *state.SharedDomains) error {
		for addr, acc := range accs {
			sd.DomainPut(kv.AccountsDomain, []byte(addr), nil, acc, nil, 0)
		}
		return nil
	}))

	// Interleave two WithDomainsRo threads.
	ch := make(chan bool, 1)

	go func() {
		require.NoError(t, dm.WithDomainsRo(0, func(sd *state.SharedDomains) error {
			for addr, acc := range accs {
				actualAcc, err := sd.GetCommitmentContext().AccountRaw([]byte(addr))
				t.Logf("T1: read %x = %x", addr, actualAcc)
				assert.NoError(t, err)
				assert.Equal(t, acc, actualAcc)
				ch <- true
			}
			return nil
		}))
	}()

	require.NoError(t, dm.WithDomainsRo(0, func(sd *state.SharedDomains) error {
		for addr, acc := range accs {
			<-ch
			actualAcc, err := sd.GetCommitmentContext().AccountRaw([]byte(addr))
			t.Logf("T2: read %x = %x", addr, actualAcc)
			assert.NoError(t, err)
			assert.Equal(t, acc, actualAcc)
		}
		return nil
	}))
}

func Benchmark_DomainsRo(b *testing.B) {
	dm, err := NewTemporaryDomainsManager(b.TempDir())
	require.NoError(b, err)
	defer dm.Close()

	var (
		addr = common.HexToAddress("0x1111111111111111111111111111111111111111").Bytes()
		acc  = accounts.SerialiseV3(&accounts.Account{Balance: *uint256.NewInt(90)})
	)

	// Commit some data
	require.NoError(b, dm.WithDomainsRw(0, func(sd *state.SharedDomains) error {
		sd.DomainPut(kv.AccountsDomain, addr, nil, acc, nil, 0)
		return nil
	}))

	b.Run("keep RoTx open", func(b *testing.B) {
		dm.WithDomainsRo(0, func(sd *state.SharedDomains) error {
			for i := 0; i < b.N; i++ {
				sd.GetCommitmentContext().AccountRaw(addr)
			}
			return nil
		})
	})

	b.Run("open RoTx every read", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			dm.withDomainsRo_callerThread(0, func(sd *state.SharedDomains) error {
				sd.GetCommitmentContext().AccountRaw(addr)
				return nil
			})
		}
	})

	b.Run("reuse RoTx in worker threads", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			dm.withDomainsRo_workerThread(0, func(sd *state.SharedDomains) error {
				sd.GetCommitmentContext().AccountRaw(addr)
				return nil
			})
		}
	})
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
		require.NoError(t, dm.WithDomainsRw(uint64(i), func(sd *state.SharedDomains) error {
			sd.DomainPut(kv.AccountsDomain, addr, nil, acc, nil, 0)
			sd.DomainPut(kv.StorageDomain, append(contractAddr, slot...), nil, data, nil, 0)
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
