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
	"encoding/hex"
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

	ctx := NewDeferredContext(t.TempDir())
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
		branch     = hexutil.MustDecode("0x400c400c1214222222222222222222222222222222222222222220e48f2888f07ef9b5a59bb140aeff3c1b714d0f3dcaef4af1dadd2639f73adab916143333333333333333333333333333333333333333343333333333333333333333333333333333333333000000000000000000000000000000000000000000000000000000000000004420e3f7075ef98a49398a24584eefa3457adf3e053a36e1a72ce2045f73e92d25ab121411111111111111111111111111111111111111112085aed9cba77b3b48319c7c608412d5b47c69fa1e75e04af86ffc76e852abfac7")
		prevBranch = []byte{}
		prevStep   = uint64(0)
	)

	check := func(sd *state.SharedDomains) {
		acc, err := sd.GetCommitmentContext().AccountRaw(addr1)
		require.NoError(t, err)
		assert.Equal(t, acc1, acc)

		data, err := sd.GetCommitmentContext().StorageRaw(append(addr3, slot...))
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
			sd.DomainPut(kv.StorageDomain, append(addr3, slot...), nil, data, nil, 0)
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

		ctx := NewDeferredContext(t.TempDir())
		ctx.PutAccount(addr1, acc1)
		ctx.PutAccount(addr2, acc2)
		ctx.PutAccount(addr3, acc3)
		ctx.PutStorage(append(addr3, slot...), data)
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

func Test_Context_Override(t *testing.T) {
	var (
		// Test_HexPatriciaHashed_UniqueRepresentation2 data in ErigonV3 account format.
		// Manually created using accounts.SerialiseV3.
		accounts1 = [][2]string{
			{"0x71562b71999873db5b286df957af199ec94617f7", "0x0103043b98a7830000"},
			{"0x3a220f351252089d385b29beca14e27f204c296a", "0x00030dbc8a0000"},
			{"0x0000000000000000000000000000000000000000", "0x00081bc16d674eca1e950000"},
			{"0x1337beef00000000000000000000000000000000", "0x00083782dace9d921e950000"},
		}
		expectedHash1 = "920d630d52432c87f551191217322df4be72ce0dc22286f5d6dba01a99be5b4e"

		accounts2 = [][2]string{ // overwrites existing accounts
			{"0x71562b71999873db5b286df957af199ec94617f7", "0x01040602220adf74630000"},
			{"0x3a220f351252089d385b29beca14e27f204c296a", "0x00030c840a0000"},
			{"0x0000000000000000000000000000000000000000", "0x000829a2241af62e1e950000"},
			// no change to 0x1337beef account. Must be loaded from DB.
		}
		expectedHash2 = "4125375597c6290eb53103f518ea9486a121c3874d844307f7bc1ad7f9fa0c54"
	)
	_ = expectedHash1
	_ = accounts2
	_ = expectedHash2

	dm, err := NewTemporaryDomainsManager(t.TempDir())
	require.NoError(t, err)
	defer dm.Close()

	{
		t.Log("1. Commit first batch, only operate on new (pending) accounts")
		// Emulate NewTrie
		ctx := NewDeferredContext(t.TempDir())
		ctx.SetTrace(false)

		// Emulate trie.Update
		for _, account := range accounts1 {
			addr, acc := common.HexToAddress(account[0]), hexutil.MustDecode(account[1])
			ctx.PutAccount(addr.Bytes(), acc)
		}

		// Emulate trie.Hash
		dm.WithDomainsRo(0, func(sd *state.SharedDomains) error {
			ctx.SetDomains(sd)
			hash, err := ctx.Hash()
			require.NoError(t, err)
			assert.Equal(t, expectedHash1, hex.EncodeToString(hash))
			return nil
		})

		// Emulate trie.Commit
		dm.WithDomainsRw(0, func(sd *state.SharedDomains) error {
			ctx.SetDomains(sd)
			ctx.Commit()
			return nil
		})
	}
	{
		t.Log("2. Commit second batch, calculate root hash with new and existing accounts")

		// Emulate NewTrie
		ctx := NewDeferredContext(t.TempDir())
		ctx.SetTrace(false)
		dm.WithDomainsRo(0, func(sd *state.SharedDomains) error {
			ctx.SetDomains(sd)
			return nil
		})

		// Emulate trie.Update
		for _, account := range accounts2 {
			addr, acc := common.HexToAddress(account[0]), hexutil.MustDecode(account[1])
			ctx.PutAccount(addr.Bytes(), acc)
		}

		// Emulate trie.Hash
		dm.WithDomainsRo(0, func(sd *state.SharedDomains) error {
			ctx.SetDomains(sd)
			hash, err := ctx.Hash()
			require.NoError(t, err)
			assert.Equal(t, expectedHash2, hex.EncodeToString(hash))
			return nil
		})

		// Emulate trie.Commit
		dm.WithDomainsRw(1, func(sd *state.SharedDomains) error {
			ctx.SetDomains(sd)
			ctx.Commit()
			return nil
		})
	}
	{
		t.Log("3. Inspect committed blocks")

		// Check block 0
		ctx := NewDeferredContext(t.TempDir())
		ctx.SetTrace(true)
		dm.WithDomainsRo(0, func(sd *state.SharedDomains) error {
			ctx.SetDomains(sd)

			// Emulate trie.Get
			for _, account := range accounts1 {
				addr, expectedAcc := common.HexToAddress(account[0]), hexutil.MustDecode(account[1])
				acc, err := ctx.AccountRaw(addr.Bytes())
				require.NoError(t, err)
				assert.Equal(t, expectedAcc, acc)
			}

			// Emulate trie.Hash; Cannot calculate hash of historic block because the database won't store
			// the historic branches and hph state.
			_, err := ctx.Hash()
			assert.ErrorIs(t, err, errNotLatest)
			return nil
		})

		// Check block 1
		dm.WithDomainsRo(1, func(sd *state.SharedDomains) error {
			ctx.SetDomains(sd)

			// Emulate trie.Get
			for _, account := range accounts2 {
				addr, expectedAcc := common.HexToAddress(account[0]), hexutil.MustDecode(account[1])
				acc, err := ctx.AccountRaw(addr.Bytes())
				require.NoError(t, err)
				assert.Equal(t, expectedAcc, acc)
			}

			// Emulate trie.Hash; Can calculate hash of the latest block.
			hash, err := ctx.Hash()
			require.NoError(t, err)
			assert.Equal(t, expectedHash2, hex.EncodeToString(hash))
			return nil
		})
	}

}
