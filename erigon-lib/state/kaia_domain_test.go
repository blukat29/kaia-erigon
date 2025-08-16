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

package state

import (
	"context"
	"testing"

	"github.com/erigontech/erigon-lib/common"
	"github.com/erigontech/erigon-lib/kv"
	"github.com/erigontech/erigon-lib/types/accounts"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestKaia_DomainsManager_BlockNums(t *testing.T) {
	noop := func(sd *SharedDomains) error { return nil }
	dm, err := NewTemporaryDomainsManager(t.TempDir())
	require.NoError(t, err)
	defer dm.Close()

	// Commit blocks in order.
	assert.NoError(t, dm.WithDomainsRw(0, noop))
	assert.NoError(t, dm.WithDomainsRw(1, noop))
	assert.NoError(t, dm.WithDomainsRw(2, noop))

	// Cannot commit a block less than last block.
	assert.ErrorIs(t, dm.WithDomainsRw(1, noop), commitBlockTooLow)
	// Cannot commit a block with a gap from the last block.
	assert.ErrorIs(t, dm.WithDomainsRw(4, noop), commitBlockTooHigh)

	// Permitted to commit the last block again.
	assert.NoError(t, dm.WithDomainsRw(2, noop))
	// Permitted to commit the block right after the last block.
	assert.NoError(t, dm.WithDomainsRw(3, noop))
}

func TestKaia_DomainsManager(t *testing.T) {
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

		commit = func(sd *SharedDomains) error {
			n := sd.BlockNum()
			acc := accs[n]
			assert.NoError(t, sd.DomainPut(kv.AccountsDomain, addr, nil, acc, nil, 0), n)

			h, err := sd.ComputeCommitment(ctx, true, n, "")
			assert.NoError(t, err)
			hashes[n] = h
			t.Logf("commit num: %d, hash: %x", n, h)
			return nil
		}
		query = func(sd *SharedDomains) error {
			n := sd.BlockNum()
			expectedAcc := accs[n]
			actualAcc, err := sd.GetCommitmentContext().readAccount(addr)
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
