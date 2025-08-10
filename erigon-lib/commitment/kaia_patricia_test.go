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

package commitment

import (
	"context"
	"encoding/hex"
	"testing"

	"github.com/erigontech/erigon-lib/common/hexutil"
	"github.com/erigontech/erigon-lib/common/length"
	"github.com/erigontech/erigon-lib/rlp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// This file tests that HexPatriciaHashed can achieve following goals of supporting Kaia state trie:
// (1) Calculate state root hash from Kaia's custom account formats
// (2) Calculate storage root of an account and return the storage root hash
//
// Some test cases were taken from hex_patricia_hashed_test.go
// Test_HexPatriciaHashed_UniqueRepresentation2 ~ Test_Kaia_HexPatriciaHashed_UniqueRepresentation2

// Using the ErigonV3 encoding as specimen, test that Update{RawBytes} can represent the same accounts
// as Update{Balance, Nonce, CodeHash}
func Test_Kaia_HexPatriciaHashed_UniqueRepresentation2(t *testing.T) {
	var (
		// Taken from Test_HexPatriciaHashed_UniqueRepresentation2
		builder = NewUpdateBuilder().
			Balance("71562b71999873db5b286df957af199ec94617f7", 999860099).
			Nonce("71562b71999873db5b286df957af199ec94617f7", 3).
			Balance("3a220f351252089d385b29beca14e27f204c296a", 900234).
			Balance("0000000000000000000000000000000000000000", 2000000000000138901).
			Balance("1337beef00000000000000000000000000000000", 4000000000000138901)
		accounts = [][2]string{ // accountRLP taken from accountForHashing()
			{"0x71562b71999873db5b286df957af199ec94617f7", "0xf84803843b98a783a056e81f171bcc55a6ff8345e692c0f86e5b48e01b996cadc001622fb5e363b421a0c5d2460186f7233c927e7db2dcc703c0e500b653ca82273b7bfad8045d85a470"},
			{"0x3a220f351252089d385b29beca14e27f204c296a", "0xf84780830dbc8aa056e81f171bcc55a6ff8345e692c0f86e5b48e01b996cadc001622fb5e363b421a0c5d2460186f7233c927e7db2dcc703c0e500b653ca82273b7bfad8045d85a470"},
			{"0x0000000000000000000000000000000000000000", "0xf84c80881bc16d674eca1e95a056e81f171bcc55a6ff8345e692c0f86e5b48e01b996cadc001622fb5e363b421a0c5d2460186f7233c927e7db2dcc703c0e500b653ca82273b7bfad8045d85a470"},
			{"0x1337beef00000000000000000000000000000000", "0xf84c80883782dace9d921e95a056e81f171bcc55a6ff8345e692c0f86e5b48e01b996cadc001622fb5e363b421a0c5d2460186f7233c927e7db2dcc703c0e500b653ca82273b7bfad8045d85a470"},
		}
		stateRoot = "920d630d52432c87f551191217322df4be72ce0dc22286f5d6dba01a99be5b4e"
	)
	{
		t.Logf("1. using Update{Balance, Nonce, CodeHash}")
		ctx := context.Background()
		kc := NewKaiaPatriciaContext(ModeErigonV3, 0)
		hph := NewHexPatriciaHashed(length.Addr, kc, t.TempDir())

		plainKeys, updates := builder.Build()
		upds := WrapKeyUpdates(t, ModeDirect, KeyToHexNibbleHash, plainKeys, updates)
		defer upds.Close()
		require.NoError(t, kc.applyUpdates(length.Addr, plainKeys, updates))

		rootHash, err := hph.Process(ctx, upds, "")
		require.NoError(t, err)
		assert.Equal(t, stateRoot, hex.EncodeToString(rootHash))
		t.Logf("rootHash %x\n", rootHash)
	}
	{
		t.Logf("2. using Update{RawBytes}")
		ctx := context.Background()
		kc := NewKaiaPatriciaContext(ModeRawBytes, 0)
		kc.setTrace(true)
		hph := NewHexPatriciaHashed(length.Addr, kc, t.TempDir())

		plainKeys, updates, upd := buildRawBytesUpdates(t, accounts)
		defer upd.Close()
		require.NoError(t, kc.applyUpdates(length.Addr, plainKeys, updates))

		rootHash, err := hph.Process(ctx, upd, "")
		require.NoError(t, err)
		assert.Equal(t, stateRoot, hex.EncodeToString(rootHash))
		t.Logf("rootHash %x\n", rootHash)
	}
}

func Test_Kaia_HexPatriciaHashed_KaiaAccount_RawBytes(t *testing.T) {
	testcases := []struct {
		desc      string
		accounts  [][2]string // address, accountRLP
		stateRoot string
	}{
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
	for _, tc := range testcases {
		ctx := context.Background()
		kc := NewKaiaPatriciaContext(ModeRawBytes, 0)
		hph := NewHexPatriciaHashed(length.Addr, kc, t.TempDir())

		plainKeys, updates, upd := buildRawBytesUpdates(t, tc.accounts)
		require.NoError(t, kc.applyUpdates(length.Addr, plainKeys, updates))

		rootHash, err := hph.Process(ctx, upd, "")
		upd.Close()
		require.NoError(t, err)

		assert.Equal(t, tc.stateRoot, "0x"+hex.EncodeToString(rootHash), tc.desc)
		t.Logf("rootHash %x\n", rootHash)
	}
}

func Test_Kaia_HexPatriciaHashed_StorageRoot(t *testing.T) {
	var (
		// Kairos block #505584, contract 0x9fdd7a341308e969527bd6c928068edee8399807
		address = "0x9fdd7a341308e969527bd6c928068edee8399807"
		storage = [][2]string{
			{"0x0000000000000000000000000000000000000000000000000000000000000003", mustDecodeRLP(t, "0xa0424820546f6b656e000000000000000000000000000000000000000000000010")},
			{"0x0000000000000000000000000000000000000000000000000000000000000004", mustDecodeRLP(t, "0xa04248540000000000000000000000000000000000000000000000000000000006")},
			{"0x0000000000000000000000000000000000000000000000000000000000000005", mustDecodeRLP(t, "0x95efef9fe22a5e1ae68baea7069dcb1ac607ed78cf12")},
			{"0x0000000000000000000000000000000000000000000000000000000000000002", mustDecodeRLP(t, "0x8c033b2e3c9fd0803ce8000000")},
			{"0x3eaa2d76dda4c78c477b7231cb487c2b8fa646a998125bc96085f54b529e14a6", mustDecodeRLP(t, "0x8c033b2e3c9fd0803ce8000000")},
		}

		// kaia.getAccount('0x9fdd7a341308e969527bd6c928068edee8399807', 505584)
		storageRoot = "0x41fbe8aca458c42a31464a6eff4e221b66f6ffd341a6836c33bf677f63810329"
		accountRLP  = "0x02f849c501808003c0a041fbe8aca458c42a31464a6eff4e221b66f6ffd341a6836c33bf677f63810329a0e4fc5786883b715cd4ea3e4970357eafcd8d76c992023c590fe934d655c20dcb80"

		// Hypothetical state trie with only one account because we can't reproduce all accounts in Kairos block #505584 in this test.
		stateRoot = "0x019626d8de9176415ca8169f98742ae112e7783c3e38605f9622d626b27a3b6c"
	)

	{
		t.Logf("1. calculate storage root")
		ctx := context.Background()
		kc := NewKaiaPatriciaContext(ModeRawBytes, 0)
		kc.setTrace(true)
		hph := NewHexPatriciaHashed(length.Addr, kc, t.TempDir())

		plainKeys, updates, upd := buildStorageUpdates(t, address, storage)
		require.NoError(t, kc.applyUpdates(length.Addr, plainKeys, updates))

		rootHash, err := hph.Process(ctx, upd, "")
		upd.Close()
		require.NoError(t, err)

		assert.Equal(t, storageRoot, "0x"+hex.EncodeToString(rootHash))
		t.Logf("rootHash %x\n", rootHash)
	}
	// Assume that calculated storageRoot was used to encode the accountRLP.
	{
		t.Logf("2. calculate state root")
		ctx := context.Background()
		kc := NewKaiaPatriciaContext(ModeRawBytes, 0)
		hph := NewHexPatriciaHashed(length.Addr, kc, t.TempDir())

		plainKeys, updates, upd := buildRawBytesUpdates(t, [][2]string{{address, accountRLP}})
		require.NoError(t, kc.applyUpdates(length.Addr, plainKeys, updates))

		rootHash, err := hph.Process(ctx, upd, "")
		upd.Close()
		require.NoError(t, err)

		assert.Equal(t, stateRoot, "0x"+hex.EncodeToString(rootHash))
		t.Logf("rootHash %x\n", rootHash)
	}
}

func mustDecodeRLP(t *testing.T, s string) string {
	b := hexutil.MustDecode(s)
	var v []byte
	require.NoError(t, rlp.DecodeBytes(b, &v))
	return "0x" + hex.EncodeToString(v)
}
