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
	"github.com/erigontech/erigon-lib/rlp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mustDecodeRLP(t *testing.T, s string) string {
	b := hexutil.MustDecode(s)
	var v []byte
	require.NoError(t, rlp.DecodeBytes(b, &v))
	return "0x" + hex.EncodeToString(v)
}

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

func Test_DeferredAccountTrie_ModeRawBytes_Commit(t *testing.T) {
	var (
		// Kairos block #1
		accounts = [][2]string{
			{"0x0000000000000000000000000000000000000400", "0x02f849c580808003c0a056e81f171bcc55a6ff8345e692c0f86e5b48e01b996cadc001622fb5e363b421a06c39846f5ab402760078b7bfd16c99e687c75bcb5ec65ac8f3054bad18136f0980"},
			{"0x4937a6f664630547f6b0c3c235c4f03a64ca36b1", "0x01da8095446c3b15f9926687d2c40534fdb5640000000000008001c0"},
			{"0xb74ff9dea397fe9e231df545eb53fe2adf776cb2", "0x01cd8088853a0d2313c000008001c0"},
		}
		expectedHashes = []string{
			"fbf14f63f97468e460a42b309bbad44fdc682ab3a7f95fa5d3508c9cf0009946",
			"be751ffacf5fcf9998d4f90b87164ff95166d55e445c8d27cfecbe0e67a032b6",
			"60e8f25e2fb479e625347c1f11e2f07c9cd7d0a5320013294d89281b6fceed4f",
		}
	)

	dm, err := NewTemporaryDomainsManager(t.TempDir())
	require.NoError(t, err)
	defer dm.Close()

	{
		t.Log("Commit block #0 (genesis)")
		trie := NewDeferredAccountTrie(dm, 0, true, ModeRawBytes) // start from block 0, commit to 0 (genesis)
		checkTrieHash(t, trie, hex.EncodeToString(commitment.EmptyRootHash))
		require.NoError(t, trie.Update(hexutil.MustDecode(accounts[0][0]), hexutil.MustDecode(accounts[0][1])))
		checkTrieCommit(t, trie, expectedHashes[0])
	}
	{
		t.Log("Commit block #1")
		trie := NewDeferredAccountTrie(dm, 0, false, ModeRawBytes) // start from block 0, commit to 1
		checkTrieHash(t, trie, expectedHashes[0])
		require.NoError(t, trie.Update(hexutil.MustDecode(accounts[1][0]), hexutil.MustDecode(accounts[1][1])))
		checkTrieCommit(t, trie, expectedHashes[1])
	}
	{
		t.Log("Commit block #2")
		trie := NewDeferredAccountTrie(dm, 1, false, ModeRawBytes) // start from block 1, commit to 2
		checkTrieHash(t, trie, expectedHashes[1])
		require.NoError(t, trie.Update(hexutil.MustDecode(accounts[2][0]), hexutil.MustDecode(accounts[2][1])))
		checkTrieCommit(t, trie, expectedHashes[2])
		checkTrieGet(t, trie, accounts)
	}
}

func Test_DeferredAccountTrie_ModeRawBytes_Examples(t *testing.T) {
	// expectedHash calculated with kaia SecureTrie.
	testcases := []struct {
		desc         string
		accounts     [][2]string // address, accountRLP
		expectedHash string
	}{
		{
			"Kairos block #1 (3/3)",
			[][2]string{
				{"0x0000000000000000000000000000000000000400", "0x02f849c580808003c0a056e81f171bcc55a6ff8345e692c0f86e5b48e01b996cadc001622fb5e363b421a06c39846f5ab402760078b7bfd16c99e687c75bcb5ec65ac8f3054bad18136f0980"},
				{"0x4937a6f664630547f6b0c3c235c4f03a64ca36b1", "0x01da8095446c3b15f9926687d2c40534fdb5640000000000008001c0"},
				{"0xb74ff9dea397fe9e231df545eb53fe2adf776cb2", "0x01cd8088853a0d2313c000008001c0"},
			},
			"60e8f25e2fb479e625347c1f11e2f07c9cd7d0a5320013294d89281b6fceed4f",
		},
		{
			"Samples from TestAccountSerializer",
			[][2]string{
				{"0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266", "0x01c580808001c0"},
				{"0x70997970C51812dc3A010C7d01b50e0d17dc79C8", "0x01c92a84123456788001c0"},
				{"0x3C44CdDdB6a900fa2b585dd299e03d12FA4293BC", "0x01ea2a84123456788002a1038318535b54105d4a7aae60c08fc45f9687181b4fdfc625bd1a753fa7397fed75"},
				{"0x90F79bf6EB2c4f870365E785982E1f101E93b906", "0x02f84dc92a84123456788001c0a000112233445566778899aabbccddeeff00112233445566778899aabbccddeeffa0aaaaaaaabbbbbbbbccccccccddddddddaaaaaaaabbbbbbbbccccccccdddddddd10"},
				{"0x15d34AAf54267DB7D7c367839AAf71A00a2C6A65", "0x01f84dc92a84123456788001c0a000112233445566778899aabbccddeeff00112233445566778899aabbccddeeffa0c5d2460186f7233c927e7db2dcc703c0e500b653ca82273b7bfad8045d85a47080"},
				{"0x9965507D1a55bcC2695C58ba16FB37d819B0A4dc", "0x01f84dc92a84123456788001c0a000112233445566778899aabbccddeeff00112233445566778899aabbccddeeffa0aaaaaaaabbbbbbbbccccccccddddddddaaaaaaaabbbbbbbbccccccccdddddddd10"},
			},
			"ce5a189eee967ca8e1eec27adf378078e5c0a17ef18eae77a13c30c2c44a6e2f",
		},
		{
			// This account happens to be valid in both ErigonV3 (DeserialiseV3) and Kaia (NewAccountSerializer)
			// This TC ensures that this account is treated as RawBytes, not ErigonV3.
			"Account RLP is ambiguous between ErigonV3 and Kaia; Kairos block #506176",
			[][2]string{
				{"0x22876e7f94872f0b8ae8c2433429d31d89278534", "0x01cc018701b0028e44b0008001c0"},
			},
			"591ebca6660ab9abe2f4481cc851e155ffa393295f3603ff76da87aedb5e1167",
		},
		{
			// This account's RLP encoding is very long because of the complex AccountKey it has.
			// This TC ensures that HexPatriciaHashed can handle it. HPH used to have 128-byte fixed buffer.
			"Very long account; Kairos block #509948",
			[][2]string{
				{"0xcb727cc119e7157e5530aa9af1865647ee354d7f", "0x01f8ca018802bbcbb808a1f0008005f8bca302a1022292ca24cb5b3937ad02b7f70e5f5280d8694ca14547380ada4f15554d825b35a302a103b7ec00e211d0c887f3b3b019cfd3aa403132f04e911e59f90dabca036cb3acc7b87204f86f05f86ce302a1022292ca24cb5b3937ad02b7f70e5f5280d8694ca14547380ada4f15554d825b35e302a103b7ec00e211d0c887f3b3b019cfd3aa403132f04e911e59f90dabca036cb3acc7e301a10342f3364427e59a09c359d9f4516f043ecfd580ee11246c82f2483fb57bbb37d2"},
			},
			"2cea07c80a738b0493230213c34beed17a73f311b7017d9aaa921b5dd73bed89",
		},
	}

	for _, tc := range testcases {
		dm, err := NewTemporaryDomainsManager(t.TempDir())
		require.NoError(t, err)
		defer dm.Close()

		trie := NewDeferredAccountTrie(dm, 0, true, ModeRawBytes)
		trie.SetTrace(true)
		for _, acc := range tc.accounts {
			addr, acc := hexutil.MustDecode(acc[0]), hexutil.MustDecode(acc[1])
			require.NoError(t, trie.Update(addr, acc))
		}
		checkTrieHash(t, trie, tc.expectedHash)
	}
}

func Test_DeferredStorageTrie_ModeRawBytes(t *testing.T) {
	var (
		// Kairos block #505584, contract 0x9fdd7a341308e969527bd6c928068edee8399807
		addr    = common.HexToAddress("0x9fdd7a341308e969527bd6c928068edee8399807").Bytes()
		storage = [][2]string{
			{"0x0000000000000000000000000000000000000000000000000000000000000003", mustDecodeRLP(t, "0xa0424820546f6b656e000000000000000000000000000000000000000000000010")},
			{"0x0000000000000000000000000000000000000000000000000000000000000004", mustDecodeRLP(t, "0xa04248540000000000000000000000000000000000000000000000000000000006")},
			{"0x0000000000000000000000000000000000000000000000000000000000000005", mustDecodeRLP(t, "0x95efef9fe22a5e1ae68baea7069dcb1ac607ed78cf12")},
			{"0x0000000000000000000000000000000000000000000000000000000000000002", mustDecodeRLP(t, "0x8c033b2e3c9fd0803ce8000000")},
			{"0x3eaa2d76dda4c78c477b7231cb487c2b8fa646a998125bc96085f54b529e14a6", mustDecodeRLP(t, "0x8c033b2e3c9fd0803ce8000000")},
		}

		// kaia.getAccount('0x9fdd7a341308e969527bd6c928068edee8399807', 505584)
		storageRoot = "41fbe8aca458c42a31464a6eff4e221b66f6ffd341a6836c33bf677f63810329"
		accountRLP  = "0x02f849c501808003c0a041fbe8aca458c42a31464a6eff4e221b66f6ffd341a6836c33bf677f63810329a0e4fc5786883b715cd4ea3e4970357eafcd8d76c992023c590fe934d655c20dcb80"

		// Hypothetical state trie with only one account because we can't reproduce all accounts in Kairos block #505584 in this test.
		stateRoot = "019626d8de9176415ca8169f98742ae112e7783c3e38605f9622d626b27a3b6c"
	)
	_ = accountRLP
	_ = stateRoot

	dm, err := NewTemporaryDomainsManager(t.TempDir())
	require.NoError(t, err)
	defer dm.Close()

	{
		t.Log("Commit storage trie")
		trie := NewDeferredStorageTrie(dm, addr, 0, true, ModeRawBytes)
		for _, s := range storage {
			key, value := hexutil.MustDecode(s[0]), hexutil.MustDecode(s[1])
			require.NoError(t, trie.Update(key, value))
		}
		checkTrieCommit(t, trie, storageRoot)
	}
	{
		t.Log("Commit account trie")
		trie := NewDeferredAccountTrie(dm, 0, true, ModeRawBytes)
		require.NoError(t, trie.Update(addr, hexutil.MustDecode(accountRLP)))
		checkTrieCommit(t, trie, stateRoot)
	}
	{
		t.Log("Inspect storage trie")
		trie := NewDeferredStorageTrie(dm, addr, 0, false, ModeRawBytes)
		checkTrieGet(t, trie, storage)
	}
	{
		t.Log("Inspect account trie")
		trie := NewDeferredAccountTrie(dm, 0, false, ModeRawBytes)
		acc, err := trie.Get(addr)
		require.NoError(t, err)
		require.Equal(t, hexutil.MustDecode(accountRLP), acc)
	}
}
