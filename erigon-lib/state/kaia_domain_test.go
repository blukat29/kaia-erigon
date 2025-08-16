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

package state

import (
	"context"
	"os"
	"testing"

	"github.com/erigontech/erigon-lib/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testDomainManager(t *testing.T) *DomainManager {
	tempDir := t.TempDir()
	dm, err := newTemporaryDomainManager(tempDir)
	require.NoError(t, err)

	t.Cleanup(func() {
		dm.Close()
		os.RemoveAll(tempDir)
	})

	return dm
}

func TestDomainManager_BasicOperations(t *testing.T) {
	var (
		dm = testDomainManager(t)

		addr        = common.HexToAddress("0x1234567890abcdef1234567890abcdef12345678").Bytes()
		slot        = common.HexToHash("0x1111111122222222333333334444444455555555666666667777777788888888").Bytes()
		storageKey  = append(addr, slot...)
		accountData = []byte("test_account_data")
		storageData = []byte("test_storage_data")
	)
	require.Equal(t, 52, len(storageKey))

	// Write data in first transaction
	err := dm.WithTx(func(da DomainAccessor) (bool, error) {
		da.SetBlockNum(0)
		require.NoError(t, da.PutAccount(addr, accountData))
		require.NoError(t, da.PutStorage(storageKey, storageData))
		
		// Flush domain writers before committing
		rwDa := da.(*RwDomainAccessor)
		require.NoError(t, rwDa.Flush(context.Background()))
		
		return true, nil // Commit the transaction
	})
	require.NoError(t, err)

	// Read data in second transaction
	err = dm.WithTx(func(da DomainAccessor) (bool, error) {
		da.SetBlockNum(0)

		val, err := da.GetAccount(addr)
		assert.NoError(t, err)
		assert.Equal(t, accountData, val)

		val, err = da.GetStorage(storageKey)
		assert.NoError(t, err)
		assert.Equal(t, storageData, val)

		return false, nil // No need to commit reads
	})
	require.NoError(t, err)
}
