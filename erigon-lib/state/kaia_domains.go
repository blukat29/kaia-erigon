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
	"errors"
	"fmt"
	"math"
	"sync"

	"github.com/erigontech/erigon-lib/common/datadir"
	"github.com/erigontech/erigon-lib/config3"
	"github.com/erigontech/erigon-lib/kv"
	"github.com/erigontech/erigon-lib/kv/mdbx"
	"github.com/erigontech/erigon-lib/kv/rawdbv3"
	"github.com/erigontech/erigon-lib/log/v3"
)

var (
	commitBlockTooLow  = errors.New("block number too low to commit")
	commitBlockTooHigh = errors.New("block number too high to commit")
)

type DomainsUserFn func(sd *SharedDomains) error

type txWithAggTx struct {
	kv.Tx
	aggTx *AggregatorRoTx
}

func newTxWithAggTx(tx kv.Tx, agg *AggregatorRoTx) *txWithAggTx {
	return &txWithAggTx{Tx: tx, aggTx: agg}
}

func (tx *txWithAggTx) AggTx() any { return tx.aggTx }

type DomainsManager struct {
	mu     sync.Mutex
	dirs   datadir.Dirs
	logger log.Logger

	db  kv.RwDB
	agg *Aggregator
}

func NewTemporaryDomainsManager(dir string) (*DomainsManager, error) {
	dirs := datadir.New(dir)
	logger := log.Root()

	// kv_mdbx_temporary.go
	db, err := mdbx.New(kv.ChainDB, logger).
		InMem(dirs.Chaindata). // spill directory, not the permanent one.
		Open(context.Background())
	if err != nil {
		return nil, err
	}

	return newDomainsManager(dirs, logger, db)
}

func newDomainsManager(dirs datadir.Dirs, logger log.Logger, db kv.RwDB) (*DomainsManager, error) {
	// eth/backend.go:setUpBlockReader
	agg, err := NewAggregator2(context.Background(), dirs, config3.DefaultStepSize, db, logger)
	if err != nil {
		return nil, err
	}
	if err := agg.OpenFolder(); err != nil {
		return nil, err
	}
	return &DomainsManager{
		dirs:   dirs,
		logger: logger,
		db:     db,
		agg:    agg,
	}, nil
}

func (dm *DomainsManager) Close() {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	// Order matters here.
	if dm.agg != nil {
		dm.agg.Close()
	}
	if dm.db != nil {
		dm.db.Close()
	}
}

func (dm *DomainsManager) WithDomainsRo(blockNum uint64, fn DomainsUserFn) error {
	return dm.WithDomains(blockNum, false, fn)
}

func (dm *DomainsManager) WithDomainsRw(blockNum uint64, fn DomainsUserFn) error {
	return dm.WithDomains(blockNum, true, fn)
}

func (dm *DomainsManager) WithDomains(blockNum uint64, commit bool, fn DomainsUserFn) error {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	ctx := context.Background()

	tx, err := dm.db.BeginRw(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	aggTx := dm.agg.BeginFilesRo()
	defer aggTx.Close()

	sd, err := NewSharedDomains(newTxWithAggTx(tx, aggTx), dm.logger)
	if err != nil {
		return err
	}
	defer sd.Close()

	if commit {
		if err := setTxNumsForCommit(sd, tx, blockNum); err != nil {
			return err
		}
	} else {
		if err := setTxNumsForRead(sd, blockNum); err != nil {
			return err
		}
	}

	if err := fn(sd); err != nil {
		return err
	}

	if commit {
		if err := writeTxNums(tx, blockNum); err != nil {
			return err
		}
		if err := sd.Flush(ctx, tx); err != nil {
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

func setTxNumsForCommit(sd *SharedDomains, tx kv.RwTx, blockNum uint64) error {
	lastBlockNum, _, err := rawdbv3.TxNums.Last(tx)
	if err != nil {
		return err
	}
	if blockNum < lastBlockNum {
		return fmt.Errorf("%w (want: %d, last %d)", commitBlockTooLow, blockNum, lastBlockNum)
	}
	if lastBlockNum+1 < blockNum {
		return fmt.Errorf("%w (want: %d, last %d)", commitBlockTooHigh, blockNum, lastBlockNum)
	}

	// Consolidate all state changes in a single block into one TxNum.
	// e.g. Genesis block has one transaction, so it's final TxNum is 1.
	sd.SetBlockNum(blockNum)
	sd.SetTxNum(blockNum + 1)
	// Read Latest, not-yet-committed data.
	sd.GetCommitmentContext().SetLimitReadAsOfTxNum(0, false)
	return nil
}

func setTxNumsForRead(sd *SharedDomains, blockNum uint64) error {
	sd.SetBlockNum(blockNum)
	sd.SetTxNum(blockNum + 1)
	// Read already-committed data up to txNum = less than txNum+1 = less than blockNum+2.
	sd.GetCommitmentContext().SetLimitReadAsOfTxNum(blockNum+2, false)
	// Reload hph state with the newly set LimitReadAsOfTxNum.
	sd.GetCommitmentContext().SeekCommitment(sd.Tx(), sd.aggTx.d[kv.CommitmentDomain], 0, math.MaxUint64)
	return nil
}

func writeTxNums(tx kv.RwTx, blockNum uint64) error {
	lastBlockNum, lastTxNum, err := rawdbv3.TxNums.Last(tx)
	if err != nil {
		return err
	}

	shouldWriteGenesis := (blockNum == 0 && lastTxNum == 0) // Nothing in TxNums yet and we're writing block 0.
	shouldWriteNext := (lastBlockNum+1 == blockNum)         // First time we're writing the block at `blockNum`.
	if shouldWriteGenesis || shouldWriteNext {
		return rawdbv3.TxNums.Append(tx, blockNum, blockNum+1)
	}
	return nil
}
