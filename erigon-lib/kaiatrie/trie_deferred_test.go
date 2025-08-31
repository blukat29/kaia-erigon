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
	"github.com/erigontech/erigon-lib/common/hexutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_DeferredAccountTrie_ModeErigonV3(t *testing.T) {
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
		trie := NewDeferredAccountTrie(dm, 0, true, ModeErigonV3) // start from block 0, commit to 0 (genesis)
		trie.SetTrace(false)

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

		// Inspect post-commit.
		checkTrieGet(t, trie, accounts1)
		checkTrieHash(t, trie, expectedHash1)
	}
	{
		t.Log("Opening trie at block 1")
		trie := NewDeferredAccountTrie(dm, 0, false, ModeErigonV3) // start from block 0, commit to 1
		trie.SetTrace(false)

		// Commit second batch at block 1.
		checkTrieUpdate(t, trie, accounts2)
		checkTrieHash(t, trie, expectedHash2)

		// Inspect block 1.
		checkTrieGet(t, trie, accountsMerged)
		checkTrieHash(t, trie, expectedHash2)

		// Commit block 1.
		checkTrieCommit(t, trie, expectedHash2)

		// Inspect post-commit.
		checkTrieGet(t, trie, accountsMerged)
		checkTrieHash(t, trie, expectedHash2)
	}
	{
		t.Log("Opening trie at block 2")
		trie := NewDeferredAccountTrie(dm, 1, false, ModeErigonV3) // start from block 1, commit to 2
		trie.SetTrace(false)

		// Inspect block 2.
		checkTrieGet(t, trie, accountsMerged)
		checkTrieHash(t, trie, expectedHash2)
	}
	{
		t.Log("Opening trie at block 0")
		trie := NewDeferredAccountTrie(dm, 0, true, ModeErigonV3) // start from block 0, commit to 0 (genesis)
		trie.SetTrace(false)

		// Inspect block 0.
		checkTrieUpdate(t, trie, accounts1)
		_, err := trie.Hash()
		assert.ErrorIs(t, err, errNotLatest)
	}
}

func Test_DeferredStorageTrie_ModeErigonV3(t *testing.T) {
	var (
		// Test_HexPatriciaHashed_ProcessWithDozensOfStorageKeys data in ErigonV3 account format.
		// Manually created using accounts.SerialiseV3.
		accs = [][2]string{
			{"0x0000000000000000000000000000000000000000", "0x0001040000"},
			{"0x0000000000000000000000000000000000000001", "0x0001050000"},
			{"0x0000000000000000000000000000000000000002", "0x0001060000"},
			{"0x0000000000000000000000000000000000000003", "0x0001070000"},
			{"0x0000000000000000000000000000000000000004", "0x000204d10000"},
			{"0x0000000000000000000000000000000000000005", "0x0001090000"},
			{"0x00000000000000000000000000000000000000b9", "0x0001060000"},
			{"0x00000000000000000000000000000000000000ba", "0x00026b860000"},
			{"0x00000000000000000000000000000000000000f5", "0x0001040000"},
			{"0x00000000000000000000000000000000000000ff", "0x0302958c030dbc8a0000"},
		}
		storage = [][3]string{
			{"0x0000000000000000000000000000000000000003", "0x56", "0x050505"},
			{"0x0000000000000000000000000000000000000003", "0x87", "0x060606"},
			{"0x0000000000000000000000000000000000000004", "0x01", "0x0401"},
			{"0x0000000000000000000000000000000000000005", "0x02", "0x8989"},
			{"0x00000000000000000000000000000000000000f5", "0x04", "0x9898"},
			{"0x00000000000000000000000000000000000000f5", "0x05", "0x1234"},
			{"0x00000000000000000000000000000000000000f5", "0x06", "0x5678"},
			{"0x00000000000000000000000000000000000000f5", "0x07", "0x9abc"},
			{"0x00000000000000000000000000000000000000f5", "0x08", "0xdef0"},
			{"0x00000000000000000000000000000000000000f5", "0x09", "0x1111"},
			{"0x00000000000000000000000000000000000000f5", "0x0a", "0x2222"},
			{"0x00000000000000000000000000000000000000f5", "0x0b", "0x3333"},
			{"0x00000000000000000000000000000000000000f5", "0x0c", "0x4444"},
			{"0x00000000000000000000000000000000000000f5", "0x0d", "0x5555"},
			{"0x00000000000000000000000000000000000000f5", "0x0e", "0x6666"},
			{"0x00000000000000000000000000000000000000f5", "0x0f", "0x7777"},
			{"0x00000000000000000000000000000000000000f5", "0x10", "0x8888"},
			{"0x00000000000000000000000000000000000000f5", "0x11", "0x9999"},
			{"0x00000000000000000000000000000000000000f5", "0xd680a8cdb8eeb05a00b8824165b597d7a2c2f608057537dd2cee058569114be0", "0xaaaa"},
			{"0x00000000000000000000000000000000000000f5", "0xe9018287c0d9d38524c16f7450cf3ed7ca7b2a466a4746910462343626cb7e9b", "0xbbbb"},
			{"0x00000000000000000000000000000000000000f5", "0xe5635458dccace734b0f3fe6bae307a6d23282dae083218bd0db7ecf8b784b41", "0xcccc"},
			{"0x00000000000000000000000000000000000000f5", "0x0a1c82a16bce90d07e4aed8d44cb584b25f39d8d8dd61dea068f144e985326a2", "0xdddd"},
			{"0x00000000000000000000000000000000000000f5", "0x778e0ba7ae9d62a62b883cfb447343673f37854d335595b4934b2c20ff936a5f", "0xeeee"},
			{"0x00000000000000000000000000000000000000f5", "0x787ec6ab994586c0f3116e311c61479d4a171287ef1b4a97afcce56044d698dc", "0xffff"},
			{"0x00000000000000000000000000000000000000f5", "0x1bf6be2031cd9a8e204ffae1fea4dcfef0c85fb20d189a0a7b0880ef9b7bb3c7", "0x0000"},
			{"0x00000000000000000000000000000000000000f5", "0xab4756ebb7abc2631dddf5f362155e571c947465add47812794d8641ff04c283", "0x1111"},
			{"0x00000000000000000000000000000000000000f5", "0xf094bf04ad37fc7aa047784f3346e12ed72b799fc7dc70c9d8eac296829c592e", "0x2222"},
			{"0x00000000000000000000000000000000000000f5", "0xc88ebea9f05008643aa43f6f610eec0f81c3d736c3a85b12a09034359d744021", "0x4444"},
			{"0x00000000000000000000000000000000000000f5", "0x58a60d4461d743243c8d77a05708351bde842bf3702dfb3276a6a948603dca7d", "0xffff"},
			{"0x00000000000000000000000000000000000000f5", "0x377c067adec6f257f25dff4bc98fd74800df84974189199801ed8b560c805a95", "0xaaaa"},
			{"0x00000000000000000000000000000000000000f5", "0xc8a1d3e638914407d095a9a0f785d5dac4ad580bca47c924d6864e1431b74a23", "0xeeee"},
			{"0x00000000000000000000000000000000000000f5", "0x1f00000000000000000000000000000000000000f5", "0x00000000000000000000000000000000000000f5"},
		}

		// Storage root found from hph.SetTrace(true) "lastStorageRootHash" message.
		expectedStorageRoots = [][2]string{
			{"0x0000000000000000000000000000000000000003", "2f0721d5fd2d21c0b0a06a2c801360c7c6a4385548af2f49e43923722ae4f711"},
			{"0x0000000000000000000000000000000000000004", "84b97f8a4152affef95b1936223f249fc82c4ea3219379e62e69d46af4bb88e4"},
			{"0x0000000000000000000000000000000000000005", "7f9fa90ae22d01dd0fb915475be8fa119b0439565827b4aa11f6e5bcfe484b57"},
			{"0x00000000000000000000000000000000000000f5", "4be65fb2298453c458e81039ef8340298dd3c192f30b81450e32028712d1e5d4"},
		}
		expectedStateRoot = "31f33923def47a1e387de2d43420b8600d72ccafcaf27d25a9c2a0d8f81c3867"
	)

	dm, err := NewTemporaryDomainsManager(t.TempDir())
	require.NoError(t, err)
	defer dm.Close()

	{
		t.Log("Update storage without presence of the account")
		tries := make(map[string]*DeferredStorageTrie)
		for _, s := range storage {
			addrS, addr, key, value := s[0], hexutil.MustDecode(s[0]), hexutil.MustDecode(s[1]), hexutil.MustDecode(s[2])
			if _, ok := tries[addrS]; !ok {
				tries[addrS] = NewDeferredStorageTrie(dm, addr, 0, true, ModeErigonV3)
			}
			trie := tries[addrS]
			require.NoError(t, trie.Update(key, value))
		}

		for _, s := range expectedStorageRoots {
			addrS, expectedRoot := s[0], s[1]
			trie := tries[addrS]
			checkTrieHash(t, trie, expectedRoot)
		}
	}
	{
		t.Log("Update accounts")
		trie := NewDeferredAccountTrie(dm, 0, true, ModeErigonV3)
		for _, acc := range accs {
			addr, acc := hexutil.MustDecode(acc[0]), hexutil.MustDecode(acc[1])
			require.NoError(t, trie.Update(addr, acc))
		}
		// Because ErigonV3 account serialization doesn't include storage root, we have to feed it to HPH.
		// It won't happen if we use RawBytes account serialization.
		for _, s := range storage {
			addr, key, value := hexutil.MustDecode(s[0]), hexutil.MustDecode(s[1]), hexutil.MustDecode(s[2])
			trie.ctx.PutStorage(storageKey(addr, key), value)
		}
		checkTrieHash(t, trie, expectedStateRoot)
	}
}
