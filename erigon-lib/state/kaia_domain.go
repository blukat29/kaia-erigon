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
	"fmt"
	"sync"

	"github.com/erigontech/erigon-lib/common/datadir"
	"github.com/erigontech/erigon-lib/config3"
	"github.com/erigontech/erigon-lib/kv"
	"github.com/erigontech/erigon-lib/kv/mdbx"
	"github.com/erigontech/erigon-lib/log/v3"
	btree2 "github.com/tidwall/btree"
)

var (
	_ DomainAccessor = (*RwDomainAccessor)(nil)
)

// Simplfied interface to DomainShared.
type DomainAccessor interface {
	StepSize() uint64
	SetBlockNum(blockNum uint64)

	GetAccount(addr []byte) ([]byte, error)
	GetStorage(key []byte) ([]byte, error)
	GetBranch(prefix []byte) ([]byte, uint64, error)

	PutAccount(addr []byte, acc []byte) error
	PutStorage(key []byte, value []byte) error
	PutBranch(prefix []byte, data []byte, prevData []byte, prevStep uint64) error

	DelAccount(addr []byte) error
	DelStorage(key []byte) error
}

// Simplified DomainShared with minimal read/write operations.
type RwDomainAccessor struct {
	roTx          kv.RwTx
	aggTx         *AggregatorRoTx
	domainWriters [kv.DomainLen]*domainBufferedWriter

	txNum uint64
}

func newRwDomainAccessor(rwTx kv.RwTx, aggTx *AggregatorRoTx) *RwDomainAccessor {
	da := &RwDomainAccessor{
		roTx:  rwTx,
		aggTx: aggTx,
		txNum: 0,
	}
	for id, d := range aggTx.d {
		da.domainWriters[id] = d.NewWriter()
	}
	return da
}

func (da *RwDomainAccessor) StepSize() uint64 {
	return da.aggTx.a.StepSize()
}

func (da *RwDomainAccessor) SetBlockNum(blockNum uint64) {
	da.txNum = blockNum + 1
	for _, d := range da.domainWriters {
		d.SetTxNum(da.txNum)
	}
}

// Analogous to SharedDomainsCommitmentContext.readAccount
func (da *RwDomainAccessor) GetAccount(addr []byte) ([]byte, error) {
	v, _, err := da.aggTx.GetAsOf(da.roTx, kv.AccountsDomain, addr, da.txNum)
	// v, _, _, err := da.aggTx.GetLatest(kv.AccountsDomain, addr, da.roTx)
	return v, err
}

// Analogous to SharedDomainsCommitmentContext.readStorage
func (da *RwDomainAccessor) GetStorage(key []byte) ([]byte, error) {
	v, _, err := da.aggTx.GetAsOf(da.roTx, kv.StorageDomain, key, da.txNum)
	return v, err
}

// Analogous to SharedDomainsCommitmentContext.Branch
func (da *RwDomainAccessor) GetBranch(prefix []byte) ([]byte, uint64, error) {
	v, _, err := da.aggTx.GetAsOf(da.roTx, kv.StorageDomain, prefix, da.txNum)
	step := da.txNum / da.StepSize()
	return v, step, err
}

// Analogous to SharedDomains.DomainPut and SharedDomains.updateAccountData
func (da *RwDomainAccessor) PutAccount(addr []byte, acc []byte) error {
	prevVal, prevStep, _, err := da.aggTx.GetLatest(kv.AccountsDomain, addr, da.roTx)
	if err != nil {
		return err
	}
	fmt.Println("PutAccount", addr, acc, prevVal, prevStep)
	return da.domainWriters[kv.AccountsDomain].PutWithPrev(addr, nil, acc, prevVal, prevStep)
}

// Analogous to SharedDomains.DomainPut and SharedDomains.writeAccountStorage
func (da *RwDomainAccessor) PutStorage(key []byte, value []byte) error {
	prevVal, prevStep, _, err := da.aggTx.GetLatest(kv.StorageDomain, key, da.roTx)
	if err != nil {
		return err
	}
	return da.domainWriters[kv.StorageDomain].PutWithPrev(key, nil, value, prevVal, prevStep)
}

// Analogous to SharedDomains.DomainPut and SharedDomains.updateCommitmentData
func (da *RwDomainAccessor) PutBranch(prefix []byte, data []byte, prevData []byte, prevStep uint64) error {
	return da.domainWriters[kv.StorageDomain].PutWithPrev(prefix, nil, data, prevData, prevStep)
}

// Analogous to SharedDomains.DomainDel and SharedDomains.deleteAccount
func (da *RwDomainAccessor) DelAccount(addr []byte) error {
	// Delete any storage for this account.
	haveRamUpdates := false
	emptyRamIter := new(btree2.Map[string, dataWithPrevStep]).Iter()
	it := func(k []byte, v []byte, step uint64) error {
		return da.domainWriters[kv.StorageDomain].DeleteWithPrev(k, nil, v, step)
	}
	da.aggTx.d[kv.StorageDomain].debugIteratePrefix(addr, haveRamUpdates, emptyRamIter, it, da.txNum, da.StepSize(), da.roTx)

	// Delete the account.
	prevVal, prevStep, _, err := da.aggTx.GetLatest(kv.AccountsDomain, addr, da.roTx)
	if err != nil {
		return err
	}
	return da.domainWriters[kv.AccountsDomain].DeleteWithPrev(addr, nil, prevVal, prevStep)
}

// Analogous to SharedDomains.DomainDel and SharedDomains.deleteStorage
func (da *RwDomainAccessor) DelStorage(key []byte) error {
	prevVal, prevStep, _, err := da.aggTx.GetLatest(kv.StorageDomain, key, da.roTx)
	if err != nil {
		return err
	}
	return da.domainWriters[kv.StorageDomain].DeleteWithPrev(key, nil, prevVal, prevStep)
}

func (da *RwDomainAccessor) Flush(ctx context.Context) error {
	for _, dw := range da.domainWriters {
		if dw != nil {
			if err := dw.Flush(ctx, da.roTx); err != nil {
				return err
			}
		}
	}
	return nil
}

type DomainManager struct {
	mu     sync.Mutex
	dirs   datadir.Dirs
	logger log.Logger

	db  kv.RwDB
	agg *Aggregator
}

func newTemporaryDomainManager(dir string) (*DomainManager, error) {
	dirs := datadir.New(dir)
	logger := log.Root()

	// kv_mdbx_temporary.go
	db, err := mdbx.New(kv.ChainDB, logger).
		InMem(dirs.Chaindata). // spill directory, not the permanent one.
		Open(context.Background())
	if err != nil {
		return nil, err
	}

	// eth/backend.go:setUpBlockReader
	agg, err := NewAggregator2(context.Background(), dirs, config3.DefaultStepSize, db, logger)
	if err != nil {
		return nil, err
	}
	if err := agg.OpenFolder(); err != nil {
		return nil, err
	}

	return &DomainManager{
		dirs:   dirs,
		logger: logger,
		db:     db,
		agg:    agg,
	}, nil
}

func (dm *DomainManager) Close() {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	if dm.agg != nil {
		dm.agg.Close()
	}
	if dm.db != nil {
		dm.db.Close()
	}
}

func (dm *DomainManager) WithTx(fn func(da DomainAccessor) (bool, error)) error {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	tx, err := dm.db.BeginRw(context.Background())
	if err != nil {
		return err
	}
	defer tx.Rollback() // safe to rollback after commit. See kv/Readme.md.
	da := newRwDomainAccessor(tx, dm.agg.BeginFilesRo())

	commit, err := fn(da)
	if err != nil {
		return err
	}
	if commit {
		return tx.Commit()
	} else {
		return nil
	}
}
