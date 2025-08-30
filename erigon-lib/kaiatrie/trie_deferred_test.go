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
	"github.com/stretchr/testify/require"
)

func Test_DeferredAccountTrie(t *testing.T) {
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
		}
		expectedHash2 = "4125375597c6290eb53103f518ea9486a121c3874d844307f7bc1ad7f9fa0c54"

		accountsMerged = [][2]string{
			{"0x71562b71999873db5b286df957af199ec94617f7", "0x01040602220adf74630000"},
			{"0x3a220f351252089d385b29beca14e27f204c296a", "0x00030c840a0000"},
			{"0x0000000000000000000000000000000000000000", "0x000829a2241af62e1e950000"},
			{"0x1337beef00000000000000000000000000000000", "0x00083782dace9d921e950000"},
		}
	)

	dm, err := NewTemporaryDomainsManager(t.TempDir())
	require.NoError(t, err)
	defer dm.Close()

	{
		t.Log("Opening trie at block 0")
		trie := NewDeferredAccountTrie(dm, 0, true) // start from block 0, commit to 0 (genesis)
		trie.SetTrace(true)

		// Inspect empty state.
		checkTrieHash(t, trie, hex.EncodeToString(commitment.EmptyRootHash))

		// Commit first batch at block 0.
		checkTrieUpdate(t, trie, accounts1)
		checkTrieHash(t, trie, expectedHash1)

		// Inspect block 0.
		checkTrieGet(t, trie, accounts1)
		checkTrieHash(t, trie, expectedHash1)

		// Commit block 0.
		checkTrieCommit(t, trie, expectedHash1)
	}
	{
		t.Log("Opening trie at block 1")
		trie := NewDeferredAccountTrie(dm, 0, false) // start from block 0, commit to 1
		trie.SetTrace(true)

		// Commit second batch at block 1.
		checkTrieUpdate(t, trie, accounts2)
		checkTrieHash(t, trie, expectedHash2)

		// Inspect block 1.
		checkTrieGet(t, trie, accountsMerged)
		checkTrieHash(t, trie, expectedHash2)

		// Commit block 1.
		checkTrieCommit(t, trie, expectedHash2)
	}
	{
		t.Log("Opening trie at block 2")
		trie := NewDeferredAccountTrie(dm, 1, false) // start from block 1, commit to 2
		trie.SetTrace(true)

		// Inspect block 2.
		checkTrieGet(t, trie, accountsMerged)
		checkTrieHash(t, trie, expectedHash2)
	}
}
