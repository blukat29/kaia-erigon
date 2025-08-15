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

	"github.com/erigontech/erigon-lib/common"
	"github.com/erigontech/erigon-lib/common/hexutil"
	"github.com/erigontech/erigon-lib/kv"
	"github.com/erigontech/erigon-lib/state"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type accountTrieTC struct {
	desc      string
	accounts  [][2]string // address, accountRLP
	stateRoot string
}

func getAccountTrieTCs() []accountTrieTC {
	return []accountTrieTC{
		{
			"Kairos block #1 (1/3)",
			[][2]string{
				{"0x0000000000000000000000000000000000000400", "0x02f849c580808003c0a056e81f171bcc55a6ff8345e692c0f86e5b48e01b996cadc001622fb5e363b421a06c39846f5ab402760078b7bfd16c99e687c75bcb5ec65ac8f3054bad18136f0980"},
			},
			"0xfbf14f63f97468e460a42b309bbad44fdc682ab3a7f95fa5d3508c9cf0009946",
		},
		{
			"Kairos block #1 (2/3)",
			[][2]string{
				{"0x0000000000000000000000000000000000000400", "0x02f849c580808003c0a056e81f171bcc55a6ff8345e692c0f86e5b48e01b996cadc001622fb5e363b421a06c39846f5ab402760078b7bfd16c99e687c75bcb5ec65ac8f3054bad18136f0980"},
				{"0x4937a6f664630547f6b0c3c235c4f03a64ca36b1", "0x01da8095446c3b15f9926687d2c40534fdb5640000000000008001c0"},
			},
			"0xbe751ffacf5fcf9998d4f90b87164ff95166d55e445c8d27cfecbe0e67a032b6",
		},
		{
			"Kairos block #1 (3/3)",
			[][2]string{
				{"0x0000000000000000000000000000000000000400", "0x02f849c580808003c0a056e81f171bcc55a6ff8345e692c0f86e5b48e01b996cadc001622fb5e363b421a06c39846f5ab402760078b7bfd16c99e687c75bcb5ec65ac8f3054bad18136f0980"},
				{"0x4937a6f664630547f6b0c3c235c4f03a64ca36b1", "0x01da8095446c3b15f9926687d2c40534fdb5640000000000008001c0"},
				{"0xb74ff9dea397fe9e231df545eb53fe2adf776cb2", "0x01cd8088853a0d2313c000008001c0"},
			},
			"0x60e8f25e2fb479e625347c1f11e2f07c9cd7d0a5320013294d89281b6fceed4f",
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
			"0xce5a189eee967ca8e1eec27adf378078e5c0a17ef18eae77a13c30c2c44a6e2f",
		},
	}
}

// Test the Ephemeral mode (empty root hash, no write genesis)
func Test_AccountTrie_Ephemeral(t *testing.T) {
	for _, tc := range getAccountTrieTCs() {
		// Create database and trie.
		domm, err := NewTemporaryDomainManager(t.TempDir())
		require.NoError(t, err)
		defer domm.Close()

		trie, err := NewKaiaAccountTrie(domm, common.Hash{}, false)
		require.NoError(t, err)

		// Update the keys.
		for _, account := range tc.accounts {
			address, accountRLP := common.HexToAddress(account[0]), hexutil.MustDecode(account[1])
			trie.Update(address, accountRLP)
		}

		// 1. Check the root hash.
		rootHash, err := trie.Hash()
		require.NoError(t, err)
		require.Equal(t, common.HexToHash(tc.stateRoot), rootHash, tc.desc)

		// 1-1. Check that Hash() is idempotent.
		rootHash2, err := trie.Hash()
		require.NoError(t, err)
		require.Equal(t, rootHash, rootHash2, tc.desc)

		// 2. Check the keys.
		for _, account := range tc.accounts {
			address, accountRLP := common.HexToAddress(account[0]), hexutil.MustDecode(account[1])
			acc, err := trie.Get(address)
			require.NoError(t, err)
			require.Equal(t, accountRLP, acc, tc.desc)
		}

		// 3. Check that database is still empty.
		domm.WithTx(func(sd *state.SharedDomains) (commit bool, err error) {
			for _, account := range tc.accounts {
				key := string(hexutil.MustDecode(account[0]))
				v, _, _ := sd.GetLatest(kv.AccountsDomain, []byte(key))
				assert.Nil(t, v)
			}
			return false, nil
		})
	}
}
