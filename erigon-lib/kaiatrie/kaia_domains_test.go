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
	"context"
	"errors"
	"testing"

	"github.com/erigontech/erigon-lib/kv"
	"github.com/erigontech/erigon-lib/state"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_KaiaDomains_Agg(t *testing.T) {
	doms, err := NewTemporaryDomainManager(t.TempDir())
	require.NoError(t, err)
	defer doms.Close()

	tx, err := doms.db.BeginRw(context.Background())
	require.NoError(t, err)
	defer tx.Rollback()

	// Aggregator is required by SharedDomains for historical lookups.
	_, ok := tx.(state.HasAggTx)
	require.True(t, ok)
}

func Test_KaiaDomains_WithTx(t *testing.T) {
	domm, err := NewTemporaryDomainManager(t.TempDir())
	require.NoError(t, err)
	defer domm.Close()

	// Error - should not commit and pass error
	err = domm.WithTx(func(sd *state.SharedDomains) (bool, error) {
		sd.DomainPut(kv.AccountsDomain, []byte("11"), nil, []byte("1111"), nil, 0)
		return true, errors.New("test error")
	})
	assert.NotNil(t, err)
	assert.Nil(t, readValue(domm, kv.AccountsDomain, []byte("11")))

	// Commit=false - should not commit
	err = domm.WithTx(func(sd *state.SharedDomains) (bool, error) {
		sd.DomainPut(kv.AccountsDomain, []byte("11"), nil, []byte("1111"), nil, 0)
		return false, nil
	})
	assert.Nil(t, err)
	assert.Nil(t, readValue(domm, kv.AccountsDomain, []byte("11")))

	// Commit=true - should commit
	err = domm.WithTx(func(sd *state.SharedDomains) (bool, error) {
		sd.DomainPut(kv.AccountsDomain, []byte("11"), nil, []byte("1111"), nil, 0)
		return true, nil
	})
	assert.Nil(t, err)
	err = domm.WithTx(func(sd *state.SharedDomains) (bool, error) {
		sd.DomainPut(kv.AccountsDomain, []byte("22"), nil, []byte("2222"), nil, 0)
		return true, nil
	})
	assert.Nil(t, err)
	assert.Equal(t, []byte("1111"), readValue(domm, kv.AccountsDomain, []byte("11")))
	assert.Equal(t, []byte("2222"), readValue(domm, kv.AccountsDomain, []byte("22")))
}

func readValue(doms *DomainManager, domain kv.Domain, key []byte) []byte {
	var v []byte
	doms.WithTx(func(sd *state.SharedDomains) (bool, error) {
		v, _, _ = sd.GetLatest(domain, key)
		return false, nil
	})
	return v
}
