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
		root1         = common.HexToHash(expectedHash1).Bytes()

		accounts2 = [][2]string{ // overwrites existing accounts
			{"0x71562b71999873db5b286df957af199ec94617f7", "0x01040602220adf74630000"},
			{"0x3a220f351252089d385b29beca14e27f204c296a", "0x00030c840a0000"},
			{"0x0000000000000000000000000000000000000000", "0x000829a2241af62e1e950000"},
		}
		expectedHash2 = "4125375597c6290eb53103f518ea9486a121c3874d844307f7bc1ad7f9fa0c54"
		root2         = common.HexToHash(expectedHash2).Bytes()

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
		trie := NewDeferredAccountTrie(dm, nil, 0, true, ModeErigonV3) // start from block 0, commit to 0 (genesis)
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
		trie := NewDeferredAccountTrie(dm, root1, 0, false, ModeErigonV3) // start from block 0, commit to 1
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
		trie := NewDeferredAccountTrie(dm, root2, 1, false, ModeErigonV3) // start from block 1, commit to 2
		trie.SetTrace(false)

		// Inspect block 2.
		checkTrieGet(t, trie, accountsMerged)
		checkTrieHash(t, trie, expectedHash2)
	}
	{
		t.Log("Opening trie at block 0")
		trie := NewDeferredAccountTrie(dm, nil, 0, true, ModeErigonV3) // start from block 0, commit to 0 (genesis)
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
				tries[addrS] = NewDeferredStorageTrie(dm, addr, nil, 0, true, ModeErigonV3)
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
		trie := NewDeferredAccountTrie(dm, nil, 0, true, ModeErigonV3)
		for _, acc := range accs {
			addr, acc := hexutil.MustDecode(acc[0]), hexutil.MustDecode(acc[1])
			require.NoError(t, trie.Update(addr, acc))
		}
		// Because ErigonV3 account serialization doesn't include storage root, we have to feed it to HPH.
		// It won't happen if we use RawBytes account serialization.
		for _, s := range storage {
			addr, key, value := common.HexToAddress(s[0]), hexutil.MustDecode(s[1]), hexutil.MustDecode(s[2])
			trie.ctx.PutStorage(storageKey(addr, key), value)
		}
		checkTrieHash(t, trie, expectedStateRoot)
	}
}

func Test_DeferredAccountTrie_ModeErigonV3_Delete(t *testing.T) {
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

		accounts2 = [][2]string{
			{"0x00000000000000000000000000000000000000f5", "0x0001040000"},
			{"0x00000000000000000000000000000000000000ff", "0x0302958c030dbc8a0000"},
		}
		accounts2Deleted = [][2]string{
			{"0x00000000000000000000000000000000000000f5", "0x"},
			{"0x00000000000000000000000000000000000000ff", "0x"},
		}
	)
	_ = expectedHash1
	_ = accounts1
	_ = accounts2

	dm, err := NewTemporaryDomainsManager(t.TempDir())
	require.NoError(t, err)
	defer dm.Close()

	{
		t.Log("Commit both batches to block 0") // state = acounts1 + accounts2
		trie := NewDeferredAccountTrie(dm, nil, 0, true, ModeErigonV3)
		for _, a := range accounts1 {
			addr, acc := hexutil.MustDecode(a[0]), hexutil.MustDecode(a[1])
			require.NoError(t, trie.Update(addr, acc))
		}
		for _, a := range accounts2 {
			addr, acc := hexutil.MustDecode(a[0]), hexutil.MustDecode(a[1])
			require.NoError(t, trie.Update(addr, acc))
		}
		_, err := trie.Commit()
		require.NoError(t, err)
	}
	{
		t.Log("Commit deletion of the second batch to block 1") // state = accounts1
		trie := NewDeferredAccountTrie(dm, nil, 0, false, ModeErigonV3)
		trie.SetTrace(false)
		for _, a := range accounts2 {
			addr := hexutil.MustDecode(a[0])
			require.NoError(t, trie.Delete(addr))
		}
		checkTrieCommit(t, trie, expectedHash1)

		checkTrieGet(t, trie, accounts1)        // First batch should still be present
		checkTrieGet(t, trie, accounts2Deleted) // Second batch should have been deleted
	}
	{
		t.Log("Inspect block 0")
		trie := NewDeferredAccountTrie(dm, nil, 0, false, ModeErigonV3)
		checkTrieGet(t, trie, accounts1)
		checkTrieGet(t, trie, accounts2) // Second batch exists at this point
	}
	{
		t.Log("Inspect block 1")
		trie := NewDeferredAccountTrie(dm, nil, 1, false, ModeErigonV3)
		checkTrieGet(t, trie, accounts1)
		checkTrieGet(t, trie, accounts2Deleted)
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
		roots = [][]byte{
			common.HexToHash(expectedHashes[0]).Bytes(),
			common.HexToHash(expectedHashes[1]).Bytes(),
			common.HexToHash(expectedHashes[2]).Bytes(),
		}
	)

	dm, err := NewTemporaryDomainsManager(t.TempDir())
	require.NoError(t, err)
	defer dm.Close()

	{
		t.Log("Commit block #0 (genesis)")
		trie := NewDeferredAccountTrie(dm, nil, 0, true, ModeRawBytes) // start from block 0, commit to 0 (genesis)
		checkTrieHash(t, trie, hex.EncodeToString(commitment.EmptyRootHash))
		require.NoError(t, trie.Update(hexutil.MustDecode(accounts[0][0]), hexutil.MustDecode(accounts[0][1])))
		checkTrieCommit(t, trie, expectedHashes[0])
		checkRootHash(t, dm, expectedHashes[0], 0)
	}
	{
		t.Log("Commit block #1")
		trie := NewDeferredAccountTrie(dm, roots[0], 0, false, ModeRawBytes) // start from block 0, commit to 1
		checkTrieHash(t, trie, expectedHashes[0])
		require.NoError(t, trie.Update(hexutil.MustDecode(accounts[1][0]), hexutil.MustDecode(accounts[1][1])))
		checkTrieCommit(t, trie, expectedHashes[1])
		checkRootHash(t, dm, expectedHashes[0], 0)
		checkRootHash(t, dm, expectedHashes[1], 1)
	}
	{
		t.Log("Commit block #2")
		trie := NewDeferredAccountTrie(dm, roots[1], 1, false, ModeRawBytes) // start from block 1, commit to 2
		checkTrieHash(t, trie, expectedHashes[1])
		require.NoError(t, trie.Update(hexutil.MustDecode(accounts[2][0]), hexutil.MustDecode(accounts[2][1])))
		checkTrieCommit(t, trie, expectedHashes[2])
		checkTrieGet(t, trie, accounts)
		checkRootHash(t, dm, expectedHashes[0], 0)
		checkRootHash(t, dm, expectedHashes[1], 1)
		checkRootHash(t, dm, expectedHashes[2], 2)
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
		{
			"Many random accounts",
			[][2]string{
				{"0x538C7f96B164Bf1b97bB9F4BB472E89F5B1484F2", "0x01d5881aba923e34d9c90988059bd7df529ddd098001c0"},
				{"0x529f9F0D3Ba55B0Cc0d6144C888535841AcbE070", "0x02f859d5882dc3d532ca685bd88871c43b587864c2738003c0a0b20292495154444622255d88310a84656eaaf682db76fa388d26ce11b1523b05a0c5d2460186f7233c927e7db2dcc703c0e500b653ca82273b7bfad8045d85a47010"},
				{"0x2e9164Cf05F568d2c89Ea4d65d76060e64879059", "0x01d5881d2f1a14db5ab74b882581f41178de2f7c8003c0"},
				{"0x32770012fd27A4b65B02AE85256F736EDe3e4Dd8", "0x02f859d5886d2a143f5953a19688760a9480328b55608003c0a03a6730d3c236f12b5aa92c5eac5ff7925ceaaf28b10496149db89612d9259e3aa0c5d2460186f7233c927e7db2dcc703c0e500b653ca82273b7bfad8045d85a47010"},
				{"0x602Fd65c7B66ADff368afbf8d800CE4Be0eCbc55", "0x01f6882d2aa7911e643c10881e3e67c0d79687978002a1020c82938241836cc7de23adcc62414ba1a7fb37b827f4b7aa98192e529ed4b54c"},
				{"0x6fac83Fd6b9024B9Da7fAbCA9137744e90409C63", "0x02f859d5886f55067a71ed16f78829799b80b548cb838003c0a0776270481b69a377d2b06f7889886037af5e224f111897dee845eaf7f0f75f4da0c5d2460186f7233c927e7db2dcc703c0e500b653ca82273b7bfad8045d85a47010"},
				{"0x9c3c6390e5d084Bf33dB4D01fA86791D217E4F2E", "0x01d58874c93a8667db8a438833a5fa544ea879f38001c0"},
				{"0xE2aF2C17EbE7A0F905C208157286e7754dd63a49", "0x02f859d58874d57ad33d3f9724880105b0646a1f09648003c0a0f15991990b1072cef8bfea8af6669f7f63aff65d708d4e21f7850ed7e3f448dea0c5d2460186f7233c927e7db2dcc703c0e500b653ca82273b7bfad8045d85a47010"},
				{"0x810657F32AC66580E0e735BecAe20782AE419f85", "0x01f8a98849d59137347d8fc4881f054321651aa8408004f89303f890e306a1036331bba50e8d01ca456592975c59390ca198595e511ff8b79955128c7202942ae302a10229da7597a1acb79d1702aabac04551f6a48200d33642ab43cd9db03ab61985f0e304a10309bf701d9ec9c65f1267d86afaefad2e3d69389cba56895e272274c1d4bcb3f1e306a10284bdae3d12215640a6ed715c0895e9fe27e546cae6481a0de901f0c95b04addc"},
				{"0x6Ec4AdfFFed84e075A6F850c71dCD510e03ff18F", "0x02f859d5884cfefa0291cfeae6884419cdb9761e279f8003c0a0fbd5778d710bf3afdd581669a3de747347e7559b315758d4135d24facca6b07aa0c5d2460186f7233c927e7db2dcc703c0e500b653ca82273b7bfad8045d85a47010"},
				{"0x5156c4068A43df28c6C6883133A405caA0Bef81b", "0x01f68872762c5ed772dcce88447968a83b98196c8002a10213984ff04d424ad8ccdaecdb53870ff7ef9288d6604cf9492d81718cd3a1698e"},
				{"0xF503E699482bdAf013A65796A2b740D2D9d8326d", "0x02f859d5884d910fd75070f99988769ec45d31ccaad08003c0a01a2c959795fdaff1988be2c0bc818c8fa53841ffe999a43808d73257bf904e81a0c5d2460186f7233c927e7db2dcc703c0e500b653ca82273b7bfad8045d85a47010"},
				{"0x3A6c8D8D317C55035B05ABD76DbA63ABaEd884ef", "0x01f901418870a5d005301b681c8804a76a369bad1adb8005f9012ab85905f8568201c0b84e04f84b01f848e307a1020be8281589b520b5ccb7c94d85f5e87140e7e9ad5f9ba4c153489ba201cec512e309a1036b313fd6a419cf8673a87282077a129c311901a983359297b52b1e2579329f208203c0b87d05f87a8203c08203c0b87204f86f02f86ce305a103e59fa7735d474e502e9adc36509c42a8ac56bc10e6ceed6bc328afa67dc97ef4e305a1030bb820333b5ce2d068b729895d7e0a03a2b9f5c9ddb0943f0637b4ff26cd24f6e308a1022f9188477fbc5f49b24dce2a5bc934847a689cc88d252b7985fa7dacf40b789cb84e05f84ba302a1025dcb03d24444cc597c2878c03140143d83cc4f44ce344416693d0d8851cd55dea302a103f6b7f9dd41027bfa0f179bcc2ee1453104839d23f7bc24cf046f99bab1b863c58203c0"},
				{"0x69fd92cd2db67FF1fca0B68cec41d6e9a7C9CB2f", "0x02f859d5882114f987925e3f568816dda77af75b85438003c0a0c78620cd5de358081b0de93be25226c5906d65b0177e63615d89c9feb949b822a0c5d2460186f7233c927e7db2dcc703c0e500b653ca82273b7bfad8045d85a47010"},
				{"0x314FDB00AA6958C460a1C7445037FFd9294D8FA3", "0x01f68861fa4e01d1c470fe882f20e48386f0d2468002a10396daa10d284f1800b32593de64267f943884d4d6cdd56f00fda166860bd0c471"},
				{"0xe1e448CF980139EBA061c92ab4eB172eF4a183e8", "0x02f859d5887e94f0d808b73d43882900436bf1f316518003c0a071d67623de00dff87f6ae93092d6680e46bacaa7a93b50e18d6154fd3c7f8894a0c5d2460186f7233c927e7db2dcc703c0e500b653ca82273b7bfad8045d85a47010"},
				{"0xB49200971450287d4F58A622D010CD69Daf9981f", "0x01f885882e776c9951709b52885b9939d7170082618004f86f01f86ce307a103a8ebe3d8300d379ec0ca0fc10243d9a06e42c0fc5c353292b04c4cbf47450e56e306a103cef3dbdef93ac6c2b5b52f986b6638101595456f85d66d2b6ac4df223c67ca50e301a102b455202aa5699c7dcd1edba0ece29c604b6c8486ed85879598b96e80aba9ad4b"},
				{"0xA6974aAea5Fc5adAc3BdbA0fBCe3cFb7dd522307", "0x02f859d58801af84ff3eadce728813bcbd065371111c8003c0a0350cf63e63e6a62800190457e6cd437194cb87aa1ca6745fec3d725e79de928da0c5d2460186f7233c927e7db2dcc703c0e500b653ca82273b7bfad8045d85a47010"},
				{"0x234eA9CBB474Cff5c11A1358D8881566C2A43A6e", "0x01f8618840ea4b60d6c19275882ae30e3cf12845428004f84b01f848e309a1028991595cbcbd21470df1802cb0be1588e901b9f5dbd897dfd948d095e23d3e7ee303a10255d1246d4d6c9e8c1dfef68f869d6113b2a8c1026edf9470a09a5ad9c2396173"},
				{"0xbA65cbAaaB528a9333d96f4bA5183ce8984E4A69", "0x02f859d5880e916a77dc153f3e88071e4eaac23e52dc8003c0a097d53d74d0c106e552a999ddb71e2d2fb7689de7f8f89cf34b182da2627f36e6a0c5d2460186f7233c927e7db2dcc703c0e500b653ca82273b7bfad8045d85a47010"},
				{"0x3c78F2aB003B4A4B31Bd0A4b250f83F552Fb4559", "0x01d588236ac1c4903ba32a887d0fa6f828dcfc2f8001c0"},
				{"0x12FEF8E07EC81F8268818C0eb36F52Aa586F330b", "0x02f859d5887fb27a217650fc388816da48f79b9b9b498003c0a074811aee1dc6ec2b2737970ff8a79fe21acbda6464f487b94c97632881313484a0c5d2460186f7233c927e7db2dcc703c0e500b653ca82273b7bfad8045d85a47010"},
				{"0xF4E59917eE188591bF2089f60D21711e2761c35E", "0x01f688055d28838a9fb531885257c1dbe04841498002a10362adba997a5c59243efba0980fa463f7625df5b2d4e3dda60131105907aeb80a"},
				{"0x98EF6bFee38fA6e133B18c5f5006FbAcB84c8d22", "0x02f859d58875f058050f7cce1688493bd536c024c4d68003c0a051c4a86ee80f4691c39a78880a7680b13e6c89bf3e2c55baecc3ae1e040a5db6a0c5d2460186f7233c927e7db2dcc703c0e500b653ca82273b7bfad8045d85a47010"},
				{"0x265e6e690A3c537C8ECF7cA30387747e3DcC1594", "0x01f8f688288009da3de9b6fb88312a96fc04bbad488005f8e0b89604f89303f890e380a1021295e4c234bfc372fd897f693e045cba2c6fd6287be47be38e7bc9b289a07a19e306a102720748d2513670deb9af5639fecae2adea9d39d3d3f1eda1bd5247077aa746c2e301a102c9a771278d8f2837c68eac21b696a79f805f9a3a14a44e52f6a8ae54138743e8e301a10299052bfab48b0270c3fe807d136f3a37319b81444a964d0e6edb348bbdc62d7da302a1025fb4d50859e546377071eb6f29d42915d3410b7186d1ebe145642f01c799e1d2a302a102bc1f7fe85475eb638067ad7a614226be53f5ba72c864429a030c0c218a990318"},
				{"0x3c0eBa7aF792Dc22A15d0AF8398FA57f1dd96E9e", "0x02f859d5884f314b98fc0b2f5588258aff517d9a392a8003c0a0a031ab962887f883ebccf2948068edc4c801db08c9ab1229cf6a88190c9022d3a0c5d2460186f7233c927e7db2dcc703c0e500b653ca82273b7bfad8045d85a47010"},
				{"0x33EC8589e1D7c6CF054Dc41e7dA39062C43AF63b", "0x01f8a98805aa9bd58925cbe6884ab57248ff7f84068004f89301f890e309a103bcbda63f9bbc9be2cdc9c3834e0ec8cad44f032cc5f36ad80079b0a3d879df0ce302a1029d5e3c6c5ac1dac49261f14536aa3c590df50a91e57dd17267d5153ddf08f004e306a1028bddc8459e0f501b4bbfcf34b125dc35b1a2fffe1b6e0c5b091276fd7ffabe43e308a102b8fe5194046b494e9cfd7fe96c9e1fa0cbf022731c4c3a8d8bf0b4f4bfee72fe"},
				{"0xB6ba2F1CD53Bc04A93A722FfF89A744Ed0921DaE", "0x02f859d58806b1672b068ab291883c6ec8d6719c4d288003c0a051dd969bd90b314b3e2720cb30147bfcdfd2a6b978c29288b52ecc81f2d4ca19a0c5d2460186f7233c927e7db2dcc703c0e500b653ca82273b7bfad8045d85a47010"},
				{"0x55e5919a063c12e4Fb6D6e10180A5567D40C56Ef", "0x01d58846f6e723737e433f885f26de8ca06c1fc98003c0"},
				{"0x523628A1f450d7DE4Da2c9e073cE5ef0C546BBBd", "0x02f859d58812f87a09c5a0e3f08863c4b044c2ab63098003c0a0dfd3a8212b7d75d8e8700956d0683c42a9e520a00645dd66ae2fd901788406e5a0c5d2460186f7233c927e7db2dcc703c0e500b653ca82273b7bfad8045d85a47010"},
				{"0x066143F563b7CdAE009180Ae12AFCD69Ff76CDdc", "0x01d5883ce91a3e142d891e883d1fcd2796a0f60e8001c0"},
				{"0x5ecaC1DC96Acab5b67715Baee4ec619f9EE0b9bB", "0x02f859d5880f2a82c2f53a03ed88607b500bc475c3698003c0a05d85168967306f20a7f0097ab7bd60c8a77ef1b64d3cb6150e4c3211c21a03aaa0c5d2460186f7233c927e7db2dcc703c0e500b653ca82273b7bfad8045d85a47010"},
				{"0xe65a18872e1E4761499b57ca17BC262914a5aF3C", "0x01d5886939711bc09b20c28872349fde001df4628003c0"},
				{"0xEec06480095558DDE7C6b9c01274779032Fe749D", "0x02f859d5885deacd82c19fa742886e5c1c9b68fd001d8003c0a032295c745206888b9149e15ee022f44b45e4af15e1935ed7b71fa4211567b8fca0c5d2460186f7233c927e7db2dcc703c0e500b653ca82273b7bfad8045d85a47010"},
				{"0x7e8A3bC5330BEE352b257881891aF48756508AFF", "0x01f9016e8822954e28768380b9883469e1e0c774dcd18005f90157b87204f86f02f86ce303a102e5bc977b6a5ae5622a61c6e46e3d6373e97dd00355b2e03a282da955023d9e9be301a103d4fc7ae5115001f7dbbd2494284a2a6f15c25034209c97cc4a541b61bf38366ee308a10298c876ad5690a81f8bb9726139e2da1e6745c833414831ff859171722bc2e216b8de04f8db01f8d8e308a1020733a1c565265060ce4334d582f42db006a699438b2583f8d612adbbcd7e8da1e309a103602d6f8d5c634a36411185681f9e97dc12faaa87a6e1fd80ffbae8317cb59e79e302a1030350ed2b3d4b40ffc0f02361a251e6011e0ca1ae37c9c5769ee5554a76d1f14fe306a102cf67748d193fb690156d1c86925180b5da3a7d5cf5b37c8f1b1aa1c8f8b542b7e301a102264538854dd14ccf1d7cfc9f769052901d4846235376931391efcc223e055cb5e305a10270e9e19e0d2101b2b359c5f07517804a40a41ca7e7ddd0574568261605091a498203c0"},
				{"0xDFc649fa5a5CB22f31Be691bC2442F6d33D5C42D", "0x02f859d5885eefcb7497fc0bdd8841110f52c73eb7378003c0a084296cb0525f93d7de0d3fb3b5f4a6457b3d5bb8ef526935d8373961d7e45e0ba0c5d2460186f7233c927e7db2dcc703c0e500b653ca82273b7bfad8045d85a47010"},
				{"0x393e642cDC0a9337C64C4da2499c3d5741cc8054", "0x01d588289acead841877088849c124bf4c4b626e8001c0"},
				{"0xBc24262CB56a1DAd8C093129BBF3b746779FdBf0", "0x02f859d5885e7e13795fdc3c9f881f5c58eb21a78d7b8003c0a07207c3c313a3f2ad4a78b273377c44473cab7ede782e3fbfb70aa7e924a7ea4ea0c5d2460186f7233c927e7db2dcc703c0e500b653ca82273b7bfad8045d85a47010"},
				{"0xDBC9BA256D8a0fA1acD38141390A3F291B8D47A7", "0x01d588259f499bcf8d9b898809feb5742cbb945c8003c0"},
				{"0x6597f0870f3282E6A55c8E996244cf76cd658805", "0x02f859d58879b62af1c050b39588595b6d86bf1102b28003c0a0726da5b95062add3415a73813771cb7680e09c667a16ebcd91520a9e34295ee6a0c5d2460186f7233c927e7db2dcc703c0e500b653ca82273b7bfad8045d85a47010"},
				{"0x2573aA0D285F8AfDc5876055b6b2AAD3D45f3f9A", "0x01d58804a6a33e6e8cc11588766252c922c1b6d08003c0"},
				{"0x4f06936219682997d09c012465cfF6dd0945253B", "0x02f859d588220cac406f79cdef884d7798b3c76915698003c0a0195af0c947cd7abb901ccf4e25eba614e91ae821ef71bbecb1620b510bfb1113a0c5d2460186f7233c927e7db2dcc703c0e500b653ca82273b7bfad8045d85a47010"},
				{"0x231b77770Ddfe6aD24652F038103A67990DaE6FF", "0x01f90287881449b6bcbe26c0e8887f2c0a45676cad7a8005f902708201c0b9026705f90263b9011105f9010da302a103f46dd97727928f3f273c93abd29d9f6ad07d3d1f759b9fea439e15f93f9cb94dac05ea8201c0a302a1023a902df0fa0086d4346850a36bca8fdd0ca2b159a349f0308cd1250bd4c98f318203c0b8ba04f8b703f8b4e307a10237d4292c84f3018a7da23bc558af49fa50f777a2bf7e86a57187d1be6a2efe53e303a103db47ee1f6fd1d758a1baca1991835fbfe2687d66fd91fdb5f0791cc9307a20fae309a102333d6b1b848e20b194f058ec03d0562c0a664c3793b8b25712f14033e5096833e305a1030b3fc3cbb6fdc04a0b418d8e43f8a88420721fbfc2ac21c08adf0b91c6b77705e306a103b6ec66925c64821ec4a84c9ab9502ec64ae9b5aa82a423e6d671e1409e3ad925b9012804f9012405f90120e302a102331993b089619c06d76e89b213bfbf6a8541f7902d87c3eaf1e8b5de4237e041e302a103e70a65347072c274f3a1a96bc8c4c104360e775d24156635ad2be93c6176b064e309a1033ea70b6a1e716d565902e2f12d2f54641084650576f1710c750699af2b297273e301a103a7f09b7cd59530b72f94f53ddb44e7230eee4137a28f3e847503f3471a19c22fe301a102525bbb6188f61b3b1eaf4ead11a36ee53c7e53f682edd826c4ad2fdf0c79eb21e308a103d01385d56efcc11f18cd85691378c79a36836ae18940d0b67f7a152744a9064ee302a102d19103852de8939471dd30771c0039590f6f435d9dd902e4aa2d435f2b45d6e2e304a103906d8798b9b98b98950f3c3de57575226c911f8614cad935eaea800044f9e6d4a302a103cb68bb527a068ff5db3698b7d8b97ec513e5b2add48388e9831e8a59db0f7fa38201c0"},
				{"0xb061d49Ac79E5e44B3d70D8C6AF3d2086B2fFA90", "0x02f859d58848a2a74d117e81a1886d32e98c1f961c688003c0a009861b11713172f1375346781bcc27c286f844e89df6fd175327188d3b0b1bf6a0c5d2460186f7233c927e7db2dcc703c0e500b653ca82273b7bfad8045d85a47010"},
				{"0x8d76429490E7bAF5b355CD17F563102ce1842914", "0x01f6887923cd1f56ad9d60882ed8af6cd0ac6d038002a10235f2c66d008d4af5074d58266bac3cff7c6ff3d38acd4ca3314527390e6238cd"},
				{"0xe408ff1337992d3D98159365621b75E65DE4ab7c", "0x02f859d588583a48d1bda2802688091336db407af9438003c0a0d1ee66b0a6897c6139ec6e1c7cc2258f2dd408c68af8123ec435d69ebfceaeb9a0c5d2460186f7233c927e7db2dcc703c0e500b653ca82273b7bfad8045d85a47010"},
				{"0x1b7DbC286bAa8E6d9BD98Ee9FB6123EeE3549aC8", "0x01d5884613d1ee2dc38329880691e3f1020b16408001c0"},
				{"0x2E1C446f0A54274a8453BA044dA71b95d41CC1c2", "0x02f859d58849861d518632a755887aba486ae4fb646a8003c0a035361149478163b04e1be09d2ab9057752707aab3ad701f80dca573400454c7ca0c5d2460186f7233c927e7db2dcc703c0e500b653ca82273b7bfad8045d85a47010"},
				{"0xc103A717d96898AA8f3b54201b4A45bea1Ca69ee", "0x01f8d5881a5501f946771c818834be66bf0b5cd1928005f8bfa302a103d49d92bc36fb13bbe52de2996e57bf65c1f0fa542a8f75d6f3060b466903620bb89604f89301f890e302a103ee04d8685d2257dff233f9a16111b5464f6649d6ee78c8c88fa40ddfac5843cde307a103f2a18ecb9c0caaca167bfccec40a310d9009ef3da107f8c9a7b5871206e62bf3e306a102b18c147433cddfd0d3af019a4251094e13464e8655ca67dd20c5d87e700402c6e302a1037b3a6981b854bef1c23a32a1440791711974d872bee20c7f55a7d72b12be0a2d8201c0"},
				{"0x5a280DD7418Cb141D90c22b5473Bee657A2Daa38", "0x02f859d58879bc272e394b93bc8811884555c5a194e58003c0a03b97af8e355f33ac2f9f9c16621b700366f3df1a3f7df1056f18596546db9a3ca0c5d2460186f7233c927e7db2dcc703c0e500b653ca82273b7bfad8045d85a47010"},
				{"0x7245101De551b09160046a14f2FeF0e39EF5eAD6", "0x01f901278808d9c2142d8360998825fb389e1a3209bc8005f90110b8de04f8db01f8d8e307a102d8900fad6f846c8eaebf1122b470b9a7011429047eb30d9820bbaf4d9d3bf092e303a102c64cc9ef85424a118eac7eec29b702c11841783a29d71fe6bed852200ad984dfe309a1026af4b5997e6b361c6cece092152d86f6384b83ef7c64a527bf8132e994704962e305a1021b1febcddf43ed73a697dc144dfa079cedff3b5d8b6158c9c94d6fbc7f8bedf7e380a103730f254093ffa6e4a895489120cd6bb23a7098c6b2e2739c4974b31ef8bb34fbe304a103a637d004544bd852441617e5e73fe06c70ca4af462b89a9805395801c2edac90ac05ea8201c0a302a1029f0d90bed4cc801d62847dde9cf028ead638c4cd1e170c659a7b4b1d51bc42488201c08203c0"},
				{"0x58B503013fB633827510c327D681319659319542", "0x02f859d58875cd360469359d9b88767aabe356977ea48003c0a03d0665e763b54aacb6cb20a779163524d740b605bcba7acfa66f7f633ef3738ba0c5d2460186f7233c927e7db2dcc703c0e500b653ca82273b7bfad8045d85a47010"},
				{"0x9DfE48820A02Ddb8F24b11b90DEE51D181f7b048", "0x01d5880efd355043d2993d8841d2efc4a985a9488001c0"},
				{"0x155C153F720b45eB50de0fD0784151e6b11B71b5", "0x02f859d5885857e6cbd733a1c68811757afea466911d8003c0a0378c094dd8877d4dea8641794a755efc76c2a39905e91cc2e66055b087d48aa6a0c5d2460186f7233c927e7db2dcc703c0e500b653ca82273b7bfad8045d85a47010"},
				{"0x2408EbE146B0e96a359D0208A82f68dF7Fbb55FA", "0x01d5886a4bb7907c8ff07f8825dba81707f7bf6b8003c0"},
				{"0x893ea2e00225F0359344159ADDa3deE98d08CA72", "0x02f859d5887cc42851a90d8f068860df04e5684dfbe48003c0a07c266a585c9e648716257749da916dc68eb1499880b23b37a61f86668499016da0c5d2460186f7233c927e7db2dcc703c0e500b653ca82273b7bfad8045d85a47010"},
				{"0x058F2FB67BcA400238B70875657F31cDBaA6cd78", "0x01f8cd881d1c94a455a035808850ca62a7f6c558788004f8b704f8b4e380a103b7d67477a72f390b1cf03923069a5e6b5388093ca651be5fbe6b5f146a3a3f9de304a103371c9913175e0f32aff9cf8946ea6e9b6a799c38723a190b2a3242c382af2cefe302a1022bdb772daf4b6a9f6db94e6acc34e62fc3c02c042b41e34fa2ca686fda948c9fe303a1023ee23c2f194a04ee8954c86d039b94e08b4eafcb7acb138c49a6b5865cc55f1de303a103bd53ce15fae4ee6f67a7c898ab4b10c3af09c0d491a99b88872e4e9a9f72767a"},
				{"0x93C28c108Ef3b893E79B73C2D0127B275b99d444", "0x02f859d5882db1e3da041772458810192c4334383dc18003c0a0839a37065fa40e850c4f26516382cffab1aaf66e701ab1bf773a75ee57ed99a0a0c5d2460186f7233c927e7db2dcc703c0e500b653ca82273b7bfad8045d85a47010"},
				{"0x0290a52BEea7328487897B8C26dE0a62e210491D", "0x01d5884234b8335ab6b9d5885780b60cbf4a02a98003c0"},
				{"0x6A2cbA9c1C0716aff91589B0cD651D5b5f9961b5", "0x02f859d58870335fb49d220e468831e0dbd1eceb06978003c0a0edb0db36127e121cf87a0e9ea25fa289f79db97c822f14fa29412bac2621760ea0c5d2460186f7233c927e7db2dcc703c0e500b653ca82273b7bfad8045d85a47010"},
				{"0x8775C50a08FaCa753Bd5ee4Dd7e7a1b6C71a32DD", "0x01d58829664d5bb02e8114886a272a25541d98a28003c0"},
				{"0x8D4B17Fd0401CFE2c1Cc455eA0292712Ca4de05a", "0x02f859d5883c1c2a2df24ba3b788557038c0bf40c4068003c0a082e5f5360cb6b0a9783dae98b380f2226d90c2e6089c282a6cce7b8e1f13813ea0c5d2460186f7233c927e7db2dcc703c0e500b653ca82273b7bfad8045d85a47010"},
				{"0xD360Ed30301E971678aB71a395701Ce4A141053A", "0x01f6887f38cac0a55f46a6886a4b9a65173987d68002a1031e3b60afd89a41c17fdd030a00b811c64dc878f899c842991a7ddb02d2089500"},
				{"0xd8CeC6039d4ba7e989Ba985844b10709CC72B29f", "0x02f859d58879856e4de9f32ae68850617492ae3cc9a68003c0a0d1cd561c0015247a0eb0ffa78f42b55483b1ebcaa05fb440076415e6fe73d1dca0c5d2460186f7233c927e7db2dcc703c0e500b653ca82273b7bfad8045d85a47010"},
			},
			"3e38fc1b682119631e5d0bcc0c1c23825fdb528ea5ce9e2a82492c36ac093c94",
		},
	}

	for _, tc := range testcases {
		dm, err := NewTemporaryDomainsManager(t.TempDir())
		require.NoError(t, err)
		defer dm.Close()

		trie := NewDeferredAccountTrie(dm, nil, 0, true, ModeRawBytes)
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
		// debug.traceTransaction('0x9fedb5242f3397b344e0cd5d9cd3de3b0be19e6a99f532c4236d1b8729a5a9f3')
		// cat out.json | jq '.structLogs[] | select(.op == "SSTORE")'
		addr    = common.HexToAddress("0x9fdd7a341308e969527bd6c928068edee8399807").Bytes()
		storage = [][2]string{
			{"0x0000000000000000000000000000000000000000000000000000000000000003", "0x424820546f6b656e000000000000000000000000000000000000000000000010"},
			{"0x0000000000000000000000000000000000000000000000000000000000000004", "0x4248540000000000000000000000000000000000000000000000000000000006"},
			{"0x0000000000000000000000000000000000000000000000000000000000000005", "0xefef9fe22a5e1ae68baea7069dcb1ac607ed78cf12"},
			{"0x0000000000000000000000000000000000000000000000000000000000000002", "0x033b2e3c9fd0803ce8000000"},
			{"0x3eaa2d76dda4c78c477b7231cb487c2b8fa646a998125bc96085f54b529e14a6", "0x033b2e3c9fd0803ce8000000"},
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
		trie := NewDeferredStorageTrie(dm, addr, nil, 0, true, ModeRawBytes)
		for _, s := range storage {
			key, value := hexutil.MustDecode(s[0]), hexutil.MustDecode(s[1])
			require.NoError(t, trie.Update(key, value))
		}
		checkTrieCommit(t, trie, storageRoot)
	}
	{
		t.Log("Commit account trie")
		trie := NewDeferredAccountTrie(dm, nil, 0, true, ModeRawBytes)
		require.NoError(t, trie.Update(addr, hexutil.MustDecode(accountRLP)))
		checkTrieCommit(t, trie, stateRoot)
	}
	{
		t.Log("Inspect storage trie")
		trie := NewDeferredStorageTrie(dm, addr, nil, 0, false, ModeRawBytes)
		checkTrieGet(t, trie, storage)
	}
	{
		t.Log("Inspect account trie")
		trie := NewDeferredAccountTrie(dm, nil, 0, false, ModeRawBytes)
		acc, err := trie.Get(addr)
		require.NoError(t, err)
		require.Equal(t, hexutil.MustDecode(accountRLP), acc)
	}
}

func Test_DeferredStorageTrie2_ModeRawBytes(t *testing.T) {
	var (
		// Kairos block #505584, contract 0x9fdd7a341308e969527bd6c928068edee8399807
		addr    = common.HexToAddress("0x9fdd7a341308e969527bd6c928068edee8399807").Bytes()
		storage = [][2]string{
			{"0x0000000000000000000000000000000000000000000000000000000000000003", "0x424820546f6b656e000000000000000000000000000000000000000000000010"},
			{"0x0000000000000000000000000000000000000000000000000000000000000004", "0x4248540000000000000000000000000000000000000000000000000000000006"},
			{"0x0000000000000000000000000000000000000000000000000000000000000005", "0xefef9fe22a5e1ae68baea7069dcb1ac607ed78cf12"},
			{"0x0000000000000000000000000000000000000000000000000000000000000002", "0x033b2e3c9fd0803ce8000000"},
			{"0x3eaa2d76dda4c78c477b7231cb487c2b8fa646a998125bc96085f54b529e14a6", "0x033b2e3c9fd0803ce8000000"},
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
		t.Log("Commit storage and account")
		accountTrie := NewDeferredAccountTrie(dm, nil, 0, true, ModeRawBytes)
		storageTrie := NewDeferredStorageTrie(dm, addr, nil, 0, true, ModeRawBytes)

		for _, s := range storage {
			key, value := hexutil.MustDecode(s[0]), hexutil.MustDecode(s[1])
			require.NoError(t, storageTrie.Update(key, value))
		}
		checkTrieCommit(t, storageTrie, storageRoot)

		require.NoError(t, accountTrie.Update(addr, hexutil.MustDecode(accountRLP)))
		checkTrieCommit(t, accountTrie, stateRoot)
	}
	{
		t.Log("Inspect account and storage tries")
		accountTrie := NewDeferredAccountTrie(dm, nil, 0, false, ModeRawBytes)
		acc, err := accountTrie.Get(addr)
		require.NoError(t, err)
		require.Equal(t, hexutil.MustDecode(accountRLP), acc)

		storageTrie := NewDeferredStorageTrie2(accountTrie, addr, common.HexToHash(storageRoot).Bytes())
		checkTrieGet(t, storageTrie, storage)
	}
}
