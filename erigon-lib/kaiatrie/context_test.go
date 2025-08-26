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
	"testing"

	"github.com/erigontech/erigon-lib/commitment"
	"github.com/erigontech/erigon-lib/common"
	"github.com/erigontech/erigon-lib/common/hexutil"
	"github.com/erigontech/erigon-lib/kv"
	"github.com/erigontech/erigon-lib/state"
	"github.com/erigontech/erigon-lib/types/accounts"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test the Update type return values.
func Test_Context_Get(t *testing.T) {
	var (
		addr1 = common.HexToAddress("0x1111111111111111111111111111111111111111").Bytes()
		addr2 = common.HexToAddress("0x2222222222222222222222222222222222222222").Bytes()
		addr3 = common.HexToAddress("0x3333333333333333333333333333333333333333").Bytes()
		acc1  = accounts.SerialiseV3(&accounts.Account{Balance: *uint256.NewInt(91)})
		acc2  = accounts.SerialiseV3(&accounts.Account{Balance: *uint256.NewInt(92), Nonce: 7})
		acc3  = accounts.SerialiseV3(&accounts.Account{Balance: *uint256.NewInt(93), CodeHash: common.HexToHash("0xcc")})

		slot = common.HexToHash("0x44").Bytes()
		data = common.HexToHash("0x55").Bytes()

		prefix     = hexutil.MustDecode("0x00")
		branch     = hexutil.MustDecode("0xbbbbbbbb")
		prevBranch = hexutil.MustDecode("0xaaaaaaaa")
		prevStep   = uint64(9)
	)

	ctx := NewDeferredContext()
	ctx.PutAccount(addr1, acc1)
	ctx.PutAccount(addr2, acc2)
	ctx.PutAccount(addr3, acc3)
	ctx.PutStorage(append(addr1, slot...), data)
	ctx.PutBranch(prefix, branch, prevBranch, prevStep)

	acc, err := ctx.AccountRaw(addr1)
	require.NoError(t, err)
	assert.Equal(t, acc1, acc)

	u, err := ctx.Account(addr2)
	require.NoError(t, err)
	assert.Equal(t, &commitment.Update{
		Flags:      commitment.NonceUpdate | commitment.BalanceUpdate | commitment.CodeUpdate,
		Nonce:      7,
		Balance:    *uint256.NewInt(92),
		CodeHash:   commitment.EmptyCodeHashArray,
		StorageLen: 0,
		Storage:    [32]byte{},
	}, u)

	u, err = ctx.Account(addr3)
	require.NoError(t, err)
	assert.Equal(t, &commitment.Update{
		Flags:      commitment.NonceUpdate | commitment.BalanceUpdate | commitment.CodeUpdate,
		Nonce:      0,
		Balance:    *uint256.NewInt(93),
		CodeHash:   [32]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0xcc},
		StorageLen: 0,
		Storage:    [32]byte{},
	}, u)

	s, err := ctx.StorageRaw(append(addr1, slot...))
	require.NoError(t, err)
	assert.Equal(t, data, s)

	u, err = ctx.Storage(append(addr1, slot...))
	require.NoError(t, err)
	assert.Equal(t, &commitment.Update{
		Flags:      commitment.StorageUpdate,
		StorageLen: 32,
		Storage:    [32]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0x55},
	}, u)

	branch, step, err := ctx.Branch(prefix)
	require.NoError(t, err)
	assert.Equal(t, branch, branch)
	assert.Equal(t, step, prevStep)
}

func Test_Context_Commit(t *testing.T) {
	var (
		addr1 = common.HexToAddress("0x1111111111111111111111111111111111111111").Bytes()
		addr2 = common.HexToAddress("0x2222222222222222222222222222222222222222").Bytes()
		addr3 = common.HexToAddress("0x3333333333333333333333333333333333333333").Bytes()
		acc1  = accounts.SerialiseV3(&accounts.Account{Balance: *uint256.NewInt(91)})
		acc2  = accounts.SerialiseV3(&accounts.Account{Balance: *uint256.NewInt(92), Nonce: 7})
		acc3  = accounts.SerialiseV3(&accounts.Account{Balance: *uint256.NewInt(93), CodeHash: common.HexToHash("0xcc")})

		slot = common.HexToHash("0x44").Bytes()
		data = common.HexToHash("0x55").Bytes()

		// branch data taken from "[SDC] PutBranch" log with sd.SetTrace(true).
		prefix     = hexutil.MustDecode("0x00")
		branch     = hexutil.MustDecode("0x400c400c1214222222222222222222222222222222222222222220e48f2888f07ef9b5a59bb140aeff3c1b714d0f3dcaef4af1dadd2639f73adab91214333333333333333333333333333333333333333320fc58abc9f9531773d80a29f3576ec79b36723cb24c33a12c4a36b342c826a9ea16141111111111111111111111111111111111111111341111111111111111111111111111111111111111000000000000000000000000000000000000000000000000000000000000004420797c0820904940f15aa2b47faec69d002be026d43f37e9761074c6cbcfbbcd91")
		prevBranch = []byte{}
		prevStep   = uint64(0)
	)

	check := func(sd *state.SharedDomains) {
		acc, err := sd.GetCommitmentContext().AccountRaw(addr1)
		require.NoError(t, err)
		assert.Equal(t, acc1, acc)

		data, err := sd.GetCommitmentContext().StorageRaw(append(addr1, slot...))
		require.NoError(t, err)
		assert.Equal(t, data, data)

		branch, step, err := sd.GetCommitmentContext().Branch(prefix)
		require.NoError(t, err)
		assert.Equal(t, branch, branch)
		assert.Equal(t, step, prevStep)
	}

	{
		t.Log("1. Commit directly to SharedDomains")
		dm, err := NewTemporaryDomainsManager(t.TempDir())
		require.NoError(t, err)
		defer dm.Close()

		dm.WithDomainsRw(0, func(sd *state.SharedDomains) error {
			sd.DomainPut(kv.AccountsDomain, addr1, nil, acc1, nil, 0)
			sd.DomainPut(kv.AccountsDomain, addr2, nil, acc2, nil, 0)
			sd.DomainPut(kv.AccountsDomain, addr3, nil, acc3, nil, 0)
			sd.DomainPut(kv.StorageDomain, append(addr1, slot...), nil, data, nil, 0)
			sd.GetCommitmentContext().PutBranch(prefix, branch, prevBranch, prevStep)
			// sd.SetTrace(true)
			return nil
		})

		dm.WithDomainsRo(0, func(sd *state.SharedDomains) error {
			check(sd)
			return nil
		})
	}
	{
		t.Log("2. Write to DeferredContext then commit to SharedDomains later")

		ctx := NewDeferredContext()
		ctx.PutAccount(addr1, acc1)
		ctx.PutAccount(addr2, acc2)
		ctx.PutAccount(addr3, acc3)
		ctx.PutStorage(append(addr1, slot...), data)
		ctx.PutBranch(prefix, branch, prevBranch, prevStep)

		dm, err := NewTemporaryDomainsManager(t.TempDir())
		require.NoError(t, err)
		defer dm.Close()

		dm.WithDomainsRw(0, func(sd *state.SharedDomains) error {
			ctx.SetDomains(sd)
			ctx.Commit()
			return nil
		})

		dm.WithDomainsRo(0, func(sd *state.SharedDomains) error {
			check(sd)
			return nil
		})
	}
}
