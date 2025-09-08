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
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"

	"github.com/c2h5oh/datasize"
	"github.com/erigontech/erigon-lib/common"
	"github.com/erigontech/erigon-lib/common/datadir"
	"github.com/erigontech/erigon-lib/config3"
	"github.com/erigontech/erigon-lib/kv"
	"github.com/erigontech/erigon-lib/kv/mdbx"
	"github.com/erigontech/erigon-lib/kv/order"
	"github.com/erigontech/erigon-lib/kv/rawdbv3"
	"github.com/erigontech/erigon-lib/kv/stream"
	"github.com/erigontech/erigon-lib/log/v3"
	"github.com/erigontech/erigon-lib/state"
	"golang.org/x/sync/semaphore"
)

var (
	keyRootPrefix = []byte("stateroot") // "stateroot" || roothash => blockNum

	errCommitBlockTooLow  = errors.New("block number too low to commit")
	errCommitBlockTooHigh = errors.New("block number too high to commit")
	errStorageKeyTooShort = errors.New("storage key too short")
)

type DomainsUserFn func(sd *state.SharedDomains) error

type txWithAggTx struct {
	kv.Tx
	aggTx *state.AggregatorRoTx
}

func newTxWithAggTx(tx kv.Tx, agg *state.AggregatorRoTx) *txWithAggTx {
	return &txWithAggTx{Tx: tx, aggTx: agg}
}

func (tx *txWithAggTx) AggTx() any { return tx.aggTx }

type roTask struct {
	blockNum uint64
	fn       DomainsUserFn
	retCh    chan error
}

type DomainsManager struct {
	mu     sync.Mutex
	dirs   datadir.Dirs
	logger log.Logger

	db  kv.RwDB
	agg *state.Aggregator

	wg       sync.WaitGroup
	workers  []*roWorker
	roTaskCh chan *roTask
}

func NewTemporaryDomainsManager(dir string) (*DomainsManager, error) {
	dirs := datadir.New(dir)
	logger := log.Root()

	// kv_mdbx_temporary.go
	db, err := mdbx.New(kv.ChainDB, logger).
		InMem(dirs.Chaindata). // dirs is only for the spill. Not persistent.
		Open(context.Background())
	if err != nil {
		return nil, err
	}

	return newDomainsManager(dirs, logger, db, runtime.GOMAXPROCS(0))
}

func NewDomainsManager(dir string, logger_ foreignLogger) (*DomainsManager, error) {
	dirs := datadir.New(dir)
	logger := loggerFromForeign(logger_)

	// node.go:OpenDatabase()
	// Follow the Erigon default settings in general.
	roTxsLimiter := semaphore.NewWeighted(32) // same as OpenDatabase()
	opts := mdbx.New(kv.ChainDB, logger).
		Path(dir).
		GrowthStep(16 * datasize.MB).      // same as OpenDatabase()
		PageSize(4 * datasize.KB).         // DbPageSizeFlag default
		MapSize(1 * datasize.TB).          // DbSizeLimitFlag default
		DBVerbosity(kv.DBVerbosityLvl(2)). // WARN
		RoTxsLimiter(roTxsLimiter).
		Readonly(false).
		Exclusive(true)
	db, err := opts.Open(context.Background())
	if err != nil {
		return nil, err
	}

	return newDomainsManager(dirs, logger, db, runtime.GOMAXPROCS(0))
}

func newDomainsManager(dirs datadir.Dirs, logger log.Logger, db kv.RwDB, numWorkers int) (*DomainsManager, error) {
	// eth/backend.go:setUpBlockReader
	agg, err := state.NewAggregator2(context.Background(), dirs, config3.DefaultStepSize, db, logger)
	if err != nil {
		return nil, err
	}
	if err := agg.OpenFolder(); err != nil {
		return nil, err
	}

	dm := &DomainsManager{
		dirs:     dirs,
		logger:   logger,
		db:       db,
		agg:      agg,
		roTaskCh: make(chan *roTask, numWorkers),
	}
	for i := 0; i < numWorkers; i++ {
		w := &roWorker{
			dm:     dm,
			taskCh: dm.roTaskCh,
		}
		dm.wg.Add(1)
		dm.workers = append(dm.workers, w)
		go w.loop()
	}
	return dm, nil
}

func (dm *DomainsManager) Close() {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	// Order matters here.
	close(dm.roTaskCh)
	dm.wg.Wait()
	if dm.agg != nil {
		dm.agg.Close()
	}
	if dm.db != nil {
		dm.db.Close()
	}
}

func (dm *DomainsManager) WithDomainsRo(blockNum uint64, fn DomainsUserFn) error {
	return dm.withDomainsRo_workerThread(blockNum, fn)
}

func (dm *DomainsManager) withDomainsRo_callerThread(blockNum uint64, fn DomainsUserFn) error {
	ctx := context.Background()

	tx, err := dm.db.BeginRo(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	aggTx := dm.agg.BeginFilesRo()
	defer aggTx.Close()

	sd, err := state.NewSharedDomains(newTxWithAggTx(tx, aggTx), dm.logger)
	if err != nil {
		return err
	}
	defer sd.Close()

	if err := setTxNumsForRead(sd, blockNum); err != nil {
		return err
	}
	if err := fn(sd); err != nil {
		return err
	}
	return nil
}

func (dm *DomainsManager) withDomainsRo_workerThread(blockNum uint64, fn DomainsUserFn) error {
	task := &roTask{
		blockNum: blockNum,
		fn:       fn,
		retCh:    make(chan error),
	}
	dm.roTaskCh <- task
	return <-task.retCh
}

type roWorker struct {
	dm     *DomainsManager
	taskCh chan *roTask

	forceReopen atomic.Int32

	// Will be reused as long as the requested blockNum is the same. Reopened otherwise.
	blockNum uint64
	tx       kv.Tx
	aggTx    *state.AggregatorRoTx
	sd       *state.SharedDomains
}

func (w *roWorker) getSd(num uint64) (*state.SharedDomains, error) {
	// Last used sd is still valid.
	if w.sd != nil && w.blockNum == num && w.forceReopen.Load() == 0 {
		return w.sd, nil
	}
	w.forceReopen.Store(0)

	// Otherwise, we need to create a new sd.
	if w.sd != nil {
		w.sd.Close()
		w.aggTx.Close()
		w.tx.Rollback()
	}

	tx, err := w.dm.db.BeginRo(context.Background())
	if err != nil {
		return nil, err
	}
	aggTx := w.dm.agg.BeginFilesRo()
	sd, err := state.NewSharedDomains(newTxWithAggTx(tx, aggTx), w.dm.logger)
	if err != nil {
		aggTx.Close()
		tx.Rollback()
		return nil, err
	}
	if err := setTxNumsForRead(sd, num); err != nil {
		sd.Close()
		aggTx.Close()
		tx.Rollback()
		return nil, err
	}

	w.sd = sd
	w.aggTx = aggTx
	w.tx = tx
	w.blockNum = num
	return w.sd, nil
}

func (w *roWorker) close() {
	if w.sd != nil {
		w.sd.Close()
		w.aggTx.Close()
		w.tx.Rollback()
	}
}

func (w *roWorker) loop() {
	defer w.dm.wg.Done()
	defer w.close()

	for task := range w.taskCh {
		sd, err := w.getSd(task.blockNum)
		if err != nil {
			task.retCh <- err
		} else {
			task.retCh <- task.fn(sd)
		}
	}
}

func (dm *DomainsManager) WithDomainsRw(blockNum uint64, fn DomainsUserFn) error {
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

	sd, err := state.NewSharedDomains(newTxWithAggTx(tx, aggTx), dm.logger)
	if err != nil {
		return err
	}
	defer sd.Close()

	if err := setTxNumsForCommit(sd, tx, blockNum); err != nil {
		return err
	}
	if err := fn(sd); err != nil {
		return err
	}
	if err := writeTxNums(tx, blockNum); err != nil {
		return err
	}
	if err := sd.Flush(ctx, tx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	for _, w := range dm.workers {
		w.forceReopen.Store(1) // Signal ro workers to reopen their tx after db commit.
	}
	return nil
}

func setTxNumsForCommit(sd *state.SharedDomains, tx kv.RwTx, blockNum uint64) error {
	lastBlockNum, _, err := rawdbv3.TxNums.Last(tx)
	if err != nil {
		return err
	}
	if blockNum < lastBlockNum {
		return fmt.Errorf("%w (want: %d, last %d)", errCommitBlockTooLow, blockNum, lastBlockNum)
	}
	if lastBlockNum+1 < blockNum {
		return fmt.Errorf("%w (want: %d, last %d)", errCommitBlockTooHigh, blockNum, lastBlockNum)
	}

	// Consolidate all state changes in a single block into one TxNum.
	// e.g. Genesis block has one transaction, so it's final TxNum is 1.
	sd.SetBlockNum(blockNum)
	sd.SetTxNum(blockNum + 1)
	// Read Latest, not-yet-committed data.
	sd.GetCommitmentContext().SetLimitReadAsOfTxNum(0, false)
	return nil
}

func setTxNumsForRead(sd *state.SharedDomains, blockNum uint64) error {
	sd.SetBlockNum(blockNum)
	sd.SetTxNum(blockNum + 1)
	// Read already-committed data up to blockNum=blockNum == up to txNum=blockNum+1 == less than txNum=blockNum+2.
	sd.GetCommitmentContext().SetLimitReadAsOfTxNum(blockNum+2, false)
	// Reload hph state with the newly set LimitReadAsOfTxNum.
	return sd.ReloadCommitment()
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

func rootKey(rootHash []byte) []byte {
	return append(keyRootPrefix, common.BytesToHash(rootHash).Bytes()...)
}

// An iterator bound to a specific domain and block num.
type DomainsIterator struct {
	blockNum uint64
	tx       kv.Tx
	aggTx    *state.AggregatorRoTx
	it       stream.KV

	isStorage bool
}

func NewDomainIterator(dm *DomainsManager, domain kv.Domain, isStorage bool, startKey, endKey []byte, blockNum uint64) (*DomainsIterator, error) {
	ctx := context.Background()

	tx, err := dm.db.BeginRo(ctx)
	if err != nil {
		return nil, err
	}

	aggTx := dm.agg.BeginFilesRo()

	// Read already-committed data up to blockNum=blockNum == up to txNum=blockNum+1 == less than txNum=blockNum+2.
	it, err := aggTx.RangeAsOf(ctx, tx, domain, startKey, endKey, blockNum+2, order.Asc, kv.Unlim)
	if err != nil {
		aggTx.Close()
		tx.Rollback()
		return nil, err
	}

	return &DomainsIterator{
		blockNum:  blockNum,
		tx:        tx,
		aggTx:     aggTx,
		it:        it,
		isStorage: isStorage,
	}, nil
}

func NewAccountIterator(dm *DomainsManager, blockNum uint64) (*DomainsIterator, error) {
	return NewDomainIterator(dm, kv.AccountsDomain, false, nil, nil, blockNum)
}

func NewStorageIterator(dm *DomainsManager, addrB []byte, blockNum uint64) (*DomainsIterator, error) {
	// core/state/dump.go:DumpToCollector
	var (
		addr      = common.BytesToAddress(addrB)
		startKey  = addr.Bytes()
		endKey, _ = kv.NextSubtree(startKey)
	)
	return NewDomainIterator(dm, kv.StorageDomain, true, startKey, endKey, blockNum)
}

func (dit *DomainsIterator) Next() ([]byte, []byte, bool, error) {
	if !dit.it.HasNext() {
		return nil, nil, false, nil
	}
	k, v, err := dit.it.Next()
	if err != nil {
		return nil, nil, false, err
	}
	if len(v) == 0 {
		// Skip non-existent entries. MDBX would iterate over all keys ever created
		// regardless of the requested txNum. Sometimes not-yet-existent entries show up.
		// We ignore them because they don't exist at this blockNum.
		return nil, nil, false, nil
	}
	if dit.isStorage {
		if len(k) <= 20 {
			return nil, nil, false, errStorageKeyTooShort
		}
		k = k[20:]
	}
	return k, v, true, nil
}

func (dit *DomainsIterator) Close() {
	dit.it.Close()
	dit.aggTx.Close()
	dit.tx.Rollback()
}
