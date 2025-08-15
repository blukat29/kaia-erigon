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
	"github.com/erigontech/erigon-lib/kv"
	"github.com/erigontech/erigon-lib/state"
	"github.com/erigontech/erigon-lib/types/accounts"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_KaiaContext_Account(t *testing.T) {
	domm, err := NewTemporaryDomainManager(t.TempDir())
	require.NoError(t, err)
	defer domm.Close()

	var (
		addr = common.HexToAddress("0x1111111111111111111111111111111111111111").Bytes()
		acc0 = accounts.SerialiseV3(&accounts.Account{Balance: *uint256.NewInt(1)})
		acc1 = accounts.SerialiseV3(&accounts.Account{Balance: *uint256.NewInt(2)})
		acc2 = accounts.SerialiseV3(&accounts.Account{Balance: *uint256.NewInt(3)})
	)
	t.Logf("acc0: %x", acc0)
	t.Logf("acc1: %x", acc1)
	t.Logf("acc2: %x", acc2)

	putAccount := func(sd *state.SharedDomains, blockNum uint64, addr []byte, acc []byte) {
		sd.SetTxNum(blockNum + 1)
		sd.SetBlockNum(blockNum)
		err := sd.DomainPut(kv.AccountsDomain, addr, nil, acc, nil, 0)
		require.NoError(t, err)
	}
	getAccount := func(sd *state.SharedDomains, blockNum uint64, addr []byte) []byte {
		ctx := sd.GetCommitmentContext()
		ctx.SetLimitReadAsOfTxNum(blockNum+1, false)
		acc, err := ctx.AccountRaw(addr)
		require.NoError(t, err)
		return acc
	}
	_ = getAccount

	// Populate accounts at different block numbers.
	err = domm.WithTx(func(sd *state.SharedDomains) (commit bool, err error) {
		putAccount(sd, 0, addr, acc0)
		putAccount(sd, 1, addr, acc1)
		putAccount(sd, 2, addr, acc2)

		assert.Equal(t, acc0, getAccount(sd, 0, addr))
		assert.Equal(t, acc1, getAccount(sd, 1, addr))
		return true, nil
	})
	require.NoError(t, err)
	return

	// Read accounts at different block numbers.
	err = domm.WithTx(func(sd *state.SharedDomains) (commit bool, err error) {
		// acc, step := getAccount(sd, 0, addr)
		// require.Equal(t, acc1, acc)
		// require.Equal(t, uint64(0), step)

		// acc, step = getAccount(sd, 1, addr)
		// require.Equal(t, acc2, acc)
		// require.Equal(t, uint64(1), step)

		// acc, step := getAccount(sd, 2, addr)
		// require.Equal(t, acc3, acc)
		// require.Equal(t, uint64(2), step)

		return false, nil
	})
	require.NoError(t, err)
}
