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
	"testing"

	"github.com/erigontech/erigon-lib/common"
	"github.com/erigontech/erigon-lib/kv"
	"github.com/erigontech/erigon-lib/state"
	"github.com/erigontech/erigon-lib/types/accounts"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
			dm.WithDomainsRo(0, func(sd *state.SharedDomains) error {
				sd.GetCommitmentContext().AccountRaw(addr)
				return nil
			})
		}
	})
}
