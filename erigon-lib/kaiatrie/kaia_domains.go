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
	"sync"

	"github.com/erigontech/erigon-lib/common/datadir"
	"github.com/erigontech/erigon-lib/config3"
	"github.com/erigontech/erigon-lib/kv"
	"github.com/erigontech/erigon-lib/kv/mdbx"
	"github.com/erigontech/erigon-lib/kv/temporal"
	"github.com/erigontech/erigon-lib/log/v3"
	"github.com/erigontech/erigon-lib/state"
)

// DomainManager mediates concurrent access to the SharedDomains and underlying database.
// Note that MDBX does not support concurrent transactions.
// In Erigon, the database access is controlled by tossing database transactions top-down.
// In Kaia, we need to manage concurrent access because multiple FlatTrie instances can exist
// without coordination.
type DomainManager struct {
	mu     sync.Mutex
	logger log.Logger

	db  kv.TemporalRwDB
	agg *state.Aggregator
}

func NewTemporaryDomainManager(dir string) (*DomainManager, error) {
	dirs := datadir.New(dir)
	logger := log.Root()

	rawDB, err := openTemporaryDB(dirs, logger)
	if err != nil {
		return nil, err
	}
	return newKaiaDomainManager(rawDB, dirs, logger)
}

func newKaiaDomainManager(rawDB kv.RwDB, dirs datadir.Dirs, logger log.Logger) (*DomainManager, error) {
	agg, err := openAggregator2(dirs, rawDB, logger)
	if err != nil {
		return nil, err
	}

	// eth/backend.go:New
	// upgrade by attaching the aggregator.
	db := temporal.New(rawDB, agg)

	return &DomainManager{
		logger: logger,
		db:     db,
		agg:    agg,
	}, nil
}

func (domm *DomainManager) WithTx(fn func(sd *state.SharedDomains) (commit bool, err error)) error {
	domm.mu.Lock()
	defer domm.mu.Unlock()

	sd, tx, err := openSharedDomains(domm.db, domm.logger)
	if err != nil {
		return err
	}
	defer func() { // order matters
		sd.Close()
		tx.Rollback() // safe to rollback after commit. See kv/Readme.md.
	}()

	commit, err := fn(sd)
	if err != nil {
		return err
	}
	if commit {
		if err := sd.Flush(context.Background(), tx); err != nil {
			return err
		}
		return tx.Commit()
	}
	return nil
}

func (domm *DomainManager) Close() error {
	domm.mu.Lock()
	defer domm.mu.Unlock()

	if domm.agg != nil {
		domm.agg.Close()
	}
	if domm.db != nil {
		domm.db.Close()
	}
	return nil
}

func openTemporaryDB(dirs datadir.Dirs, logger log.Logger) (kv.RwDB, error) {
	// kv_mdbx_temporary.go
	db, err := mdbx.New(kv.ChainDB, logger).
		InMem(dirs.Chaindata). // spill directory, not the permanent one.
		Open(context.Background())
	return db, err
}

func openAggregator2(dirs datadir.Dirs, rawDB kv.RwDB, logger log.Logger) (*state.Aggregator, error) {
	// eth/backend.go:setUpBlockReader
	agg, err := state.NewAggregator2(context.Background(), dirs, config3.DefaultStepSize, rawDB, logger)
	if err != nil {
		return nil, err
	}
	if err := agg.OpenFolder(); err != nil {
		return nil, err
	}
	return agg, nil
}

func openSharedDomains(db kv.RwDB, logger log.Logger) (*state.SharedDomains, kv.RwTx, error) {
	// exec3.go:ExecV3
	tx, err := db.BeginRw(context.Background())
	if err != nil {
		return nil, nil, err
	}

	sd, err := state.NewSharedDomains(tx, logger)
	if err != nil {
		return nil, nil, err
	}

	return sd, tx, nil
}
