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

	"github.com/erigontech/erigon-lib/common/hexutil"
	"github.com/erigontech/erigon-lib/state"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_SingleTxAccountTrie(t *testing.T) {
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
			{"0x1337beef00000000000000000000000000000000", "0x00083782dace9d921e950000"}, // no change example
		}
		expectedHash2 = "4125375597c6290eb53103f518ea9486a121c3874d844307f7bc1ad7f9fa0c54"
	)

	dm, err := NewTemporaryDomainsManager(t.TempDir())
	require.NoError(t, err)
	defer dm.Close()

	// Inspect empty state.
	require.NoError(t, dm.WithDomainsRw(0, func(sd *state.SharedDomains) error {
		trie := NewSingleTxAccountTrie(sd)
		hash := trie.Hash()
		assert.Equal(t, "56e81f171bcc55a6ff8345e692c0f86e5b48e01b996cadc001622fb5e363b421", hex.EncodeToString(hash))
		return nil
	}))

	// Commit first batch at block 0.
	require.NoError(t, dm.WithDomainsRw(0, func(sd *state.SharedDomains) error {
		trie := NewSingleTxAccountTrie(sd)
		for _, acc := range accounts1 {
			addr, acc := hexutil.MustDecode(acc[0]), hexutil.MustDecode(acc[1])
			require.NoError(t, trie.Update(addr, acc))
		}
		assert.Equal(t, expectedHash1, hex.EncodeToString(trie.Hash()))
		return nil
	}))

	// Inspect block 0.
	require.NoError(t, dm.WithDomainsRw(0, func(sd *state.SharedDomains) error {
		trie := NewSingleTxAccountTrie(sd)
		for _, acc := range accounts1 {
			addr, expectedAcc := hexutil.MustDecode(acc[0]), hexutil.MustDecode(acc[1])
			acc, err := trie.Get(addr)
			require.NoError(t, err)
			assert.Equal(t, expectedAcc, acc)
		}
		assert.Equal(t, expectedHash1, hex.EncodeToString(trie.Hash()))
		return nil
	}))

	// Commit second batch at block 1.
	require.NoError(t, dm.WithDomainsRw(1, func(sd *state.SharedDomains) error {
		trie := NewSingleTxAccountTrie(sd)
		for _, acc := range accounts2 {
			addr, acc := hexutil.MustDecode(acc[0]), hexutil.MustDecode(acc[1])
			require.NoError(t, trie.Update(addr, acc))
		}
		assert.Equal(t, expectedHash2, hex.EncodeToString(trie.Hash()))
		return nil
	}))

	// Inspect block 1.
	require.NoError(t, dm.WithDomainsRw(1, func(sd *state.SharedDomains) error {
		trie := NewSingleTxAccountTrie(sd)
		for _, acc := range accounts2 {
			addr, expectedAcc := hexutil.MustDecode(acc[0]), hexutil.MustDecode(acc[1])
			acc, err := trie.Get(addr)
			require.NoError(t, err)
			assert.Equal(t, expectedAcc, acc)
		}
		assert.Equal(t, expectedHash2, hex.EncodeToString(trie.Hash()))
		return nil
	}))

}
