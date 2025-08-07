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

	"github.com/erigontech/erigon-lib/common/length"
	"github.com/erigontech/erigon-lib/types/accounts"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// This file tests that KaiaPatriciaContext is a proper PatriciaContext implementation.
// The tests are analogous to the ones in hex_patricia_hashed_test.go, but using KaiaPatriciaContext instead of MockState.
// Some tests are skipped because we want to test the PatriciaContext layer, not the HexPatriciaHashed itself.
//
// Test_HexPatriciaHashed_ResetThenSingularUpdates ~ Test_KaiaPatriciaContext_ResetThenSingularUpdates
// Test_HexPatriciaHashed_UniqueRepresentation ~ Test_KaiaPatriciaContext_UniqueRepresentation

// Check hph.Reset() scenario.
func Test_KaiaPatriciaContext_ResetThenSingularUpdates(t *testing.T) {
	accountKeyLen := 1 // for simplicity.
	ctx := context.Background()
	kc := NewKaiaPatriciaContext(0)
	hph := NewHexPatriciaHashed(accountKeyLen, kc, t.TempDir())

	// First updates.
	plainKeys, updates := NewUpdateBuilder().
		Balance("00", 4).
		Balance("01", 5).
		Balance("02", 6).
		Balance("03", 7).
		Balance("04", 8).
		Storage("04", "01", "0401").
		Storage("03", "56", "050505").
		Storage("03", "57", "060606").
		Balance("05", 9).
		Storage("05", "02", "8989").
		Storage("05", "04", "9898").
		Build()
	upds := WrapKeyUpdates(t, ModeDirect, KeyToHexNibbleHash, plainKeys, updates)
	defer upds.Close()
	require.NoError(t, kc.applyUpdates(accountKeyLen, plainKeys, updates))

	expectedFirstRootHash := "d43a75e40587c8040173b20aa9adb260f3e1cd3e358a9169d1c277c2077c4fa1"
	firstRootHash, err := hph.Process(ctx, upds, "")
	require.NoError(t, err)
	require.Equal(t, expectedFirstRootHash, hex.EncodeToString(firstRootHash))

	// Second updates.
	hph.Reset() // Reset to force reading from the PatriciaContext.

	plainKeys, updates = NewUpdateBuilder().
		Storage("03", "58", "050506").
		Build()
	WrapKeyUpdatesInto(t, upds, plainKeys, updates)
	require.NoError(t, kc.applyUpdates(accountKeyLen, plainKeys, updates))

	expectedSecondRootHash := "24f7291ff51979eec190d1fb25785ced14533711901d12e140f32ae0443074b9"
	secondRootHash, err := hph.Process(ctx, upds, "")
	require.NoError(t, err)
	require.Equal(t, expectedSecondRootHash, hex.EncodeToString(secondRootHash))

	// Third updates.
	hph.Reset()

	plainKeys, updates = NewUpdateBuilder().
		Storage("03", "58", "020807").
		Build()
	WrapKeyUpdatesInto(t, upds, plainKeys, updates)
	require.NoError(t, kc.applyUpdates(accountKeyLen, plainKeys, updates))

	expectedThirdRootHash := "e1a1d720adf0f81e7a4eece1108d195f21ded08d061d0a6c79897d26b7a0b70c"
	thirdRootHash, err := hph.Process(ctx, upds, "")
	require.NoError(t, err)
	require.Equal(t, expectedThirdRootHash, hex.EncodeToString(thirdRootHash))
}

// Check full-length address (length.Addr) and various update types (Balance, Nonce, CodeHash, Storage).
func Test_KaiaPatriciaContext_UniqueRepresentation(t *testing.T) {
	accountKeyLen := length.Addr
	ctx := context.Background()
	kc := NewKaiaPatriciaContext(0)
	hph := NewHexPatriciaHashed(accountKeyLen, kc, t.TempDir())

	plainKeys, updates := NewUpdateBuilder().
		Balance("68ee6c0e9cdc73b2b2d52dbd79f19d24fe25e2f9", 4).
		Balance("18f4dcf2d94402019d5b00f71d5f9d02e4f70e40", 900234).
		Balance("8e5476fc5990638a4fb0b5fd3f61bb4b5c5f395e", 1233).
		Storage("8e5476fc5990638a4fb0b5fd3f61bb4b5c5f395e", "24f3a02dc65eda502dbf75919e795458413d3c45b38bb35b51235432707900ed", "0401").
		Balance("27456647f49ba65e220e86cba9abfc4fc1587b81", 065606).
		Balance("b13363d527cdc18173c54ac5d4a54af05dbec22e", 4*1e17).
		Balance("d995768ab23a0a333eb9584df006da740e66f0aa", 5).
		Balance("eabf041afbb6c6059fbd25eab0d3202db84e842d", 6).
		Balance("8e5476fc5990638a4fb0b5fd3f61bb4b5c5f395e", 1237).
		Balance("93fe03620e4d70ea39ab6e8c0e04dd0d83e041f2", 7).
		Balance("ba7a3b7b095d3370c022ca655c790f0c0ead66f5", 5*1e17).
		Storage("ba7a3b7b095d3370c022ca655c790f0c0ead66f5", "0fa41642c48ecf8f2059c275353ce4fee173b3a8ce5480f040c4d2901603d14e", "050505").
		CodeHash("ba7a3b7b095d3370c022ca655c790f0c0ead66f5", "24f3a02dc65eda502dbf75919e795458413d3c45b38bb35b51235432707900ed").
		Balance("a8f8d73af90eee32dc9729ce8d5bb762f30d21a4", 9*1e16).
		Storage("93fe03620e4d70ea39ab6e8c0e04dd0d83e041f2", "de3fea338c95ca16954e80eb603cd81a261ed6e2b10a03d0c86cf953fe8769a4", "060606").
		Balance("14c4d3bba7f5009599257d3701785d34c7f2aa27", 6*1e18).
		Nonce("18f4dcf2d94402019d5b00f71d5f9d02e4f70e40", 169356).
		Storage("a8f8d73af90eee32dc9729ce8d5bb762f30d21a4", "9f49fdd48601f00df18ebc29b1264e27d09cf7cbd514fe8af173e534db038033", "8989").
		Storage("68ee6c0e9cdc73b2b2d52dbd79f19d24fe25e2f9", "d1664244ae1a8a05f8f1d41e45548fbb7aa54609b985d6439ee5fd9bb0da619f", "9898").
		Build()
	upds := WrapKeyUpdates(t, ModeDirect, KeyToHexNibbleHash, plainKeys, updates)
	defer upds.Close()
	require.NoError(t, kc.applyUpdates(accountKeyLen, plainKeys, updates))

	expectedRootHash := "d1eec9891c103a63d17302e3833276df91028d86867ffe2da0bdc9c5f17100dd"
	rootHash, err := hph.Process(ctx, upds, "")
	require.NoError(t, err)
	require.Equal(t, expectedRootHash, hex.EncodeToString(rootHash))
}

// KaiaPatriciaContext helpers and tests for it.

// Batch inject states for testing.
func (kc *KaiaPatriciaContext) applyUpdates(accountKeyLen int, plainKeys [][]byte, updates []Update) error {
	for i, key := range plainKeys {
		update := updates[i]

		if len(key) == accountKeyLen {
			if update.Flags&DeleteUpdate != 0 {
				delete(kc.pendingAccounts, string(key))
			} else {
				kc.PutAccount(key, accounts.SerialiseV3(&accounts.Account{
					Nonce:    update.Nonce,
					Balance:  update.Balance,
					CodeHash: update.CodeHash,
				}))
			}
		} else {
			if update.Flags&DeleteUpdate != 0 {
				delete(kc.pendingStorages, string(key))
			} else {
				kc.PutStorage(key, update.Storage[:update.StorageLen])
			}
		}
	}
	return nil
}

func Test_KaiaPatriciaContext_ApplyUpdatesForTest(t *testing.T) {
	accountKeyLen := 1 // for simplicity
	plainKeys, updates := NewUpdateBuilder().
		Balance("00", 4).
		Nonce("00", 246462653).
		Balance("01", 5).
		CodeHash("03", "aaaaaaaaaaf7233c927e7db2dcc703c0e500b653ca82273b7bfad8045d85a870").
		Delete("00").
		Storage("04", "01", "0401").
		Storage("03", "56", "050505").
		Build()

	kc := NewKaiaPatriciaContext(0)
	kc.setTrace(true)
	err := kc.applyUpdates(accountKeyLen, plainKeys, updates)
	require.NoError(t, err)

	u, err := kc.Account([]byte{0x00})
	require.NoError(t, err)
	assert.Equal(t, uint64(0), u.Nonce)
	assert.Equal(t, uint64(0), u.Balance.Uint64())
	assert.Equal(t, "c5d2460186f7233c927e7db2dcc703c0e500b653ca82273b7bfad8045d85a470", hex.EncodeToString(u.CodeHash[:]))

	u, err = kc.Account([]byte{0x01})
	require.NoError(t, err)
	assert.Equal(t, uint64(0), u.Nonce)
	assert.Equal(t, uint64(5), u.Balance.Uint64())
	assert.Equal(t, "c5d2460186f7233c927e7db2dcc703c0e500b653ca82273b7bfad8045d85a470", hex.EncodeToString(u.CodeHash[:]))

	u, err = kc.Account([]byte{0x03})
	require.NoError(t, err)
	assert.Equal(t, uint64(0), u.Nonce)
	assert.Equal(t, uint64(0), u.Balance.Uint64())
	assert.Equal(t, "aaaaaaaaaaf7233c927e7db2dcc703c0e500b653ca82273b7bfad8045d85a870", hex.EncodeToString(u.CodeHash[:]))

	u, err = kc.Storage([]byte{0x04, 0x01})
	require.NoError(t, err)
	assert.Equal(t, "0401", hex.EncodeToString(u.Storage[:u.StorageLen]))

	u, err = kc.Storage([]byte{0x03, 0x56})
	require.NoError(t, err)
	assert.Equal(t, "050505", hex.EncodeToString(u.Storage[:u.StorageLen]))
}
