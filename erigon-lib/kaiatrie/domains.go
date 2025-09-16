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
	"encoding/binary"
	"runtime"
	"sync"

	"github.com/c2h5oh/datasize"
	"github.com/erigontech/erigon-lib/commitment"
	"github.com/erigontech/erigon-lib/common"
	"github.com/erigontech/erigon-lib/common/datadir"
	"github.com/erigontech/erigon-lib/common/length"
	"github.com/erigontech/erigon-lib/config3"
	"github.com/erigontech/erigon-lib/kv"
	"github.com/erigontech/erigon-lib/kv/mdbx"
	"github.com/erigontech/erigon-lib/log/v3"
	"github.com/erigontech/erigon-lib/state"
	"golang.org/x/sync/semaphore"
)

var (
	// Abusing ReceiptDomain for custom data. This is safe because
	// (1) ReceiptDomain is irrelevant to the state trie processing.
	// (2) Kaia will use its own database for receipts, so ReceiptDomain not used for any purpose.
	CustomDomain = kv.ReceiptDomain

	// "stateroot" || roothash => blockNum
	keyRootPrefix = []byte("stateroot")
)

// DomainsManager is a dispatcher for the operations reading from and writing to the MDBX database.
// MDBX explicitly requires database transactions to be created and used within the same goroutine,
// and writing transactions cannot be concurrently executed. DomainsManager is responsible for
// keeping the constraints while being efficient.
type DomainsManager struct {
	mu     sync.Mutex
	dirs   datadir.Dirs
	logger log.Logger

	db  kv.RwDB
	agg *state.Aggregator

	workers   []*readWorker
	workersCh chan *readTask
	workersWg sync.WaitGroup

	hphPool sync.Pool
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
		db.Close()
		return nil, err
	}
	if err := agg.OpenFolder(); err != nil {
		agg.Close()
		db.Close()
		return nil, err
	}

	dm := &DomainsManager{
		dirs:   dirs,
		logger: logger,
		db:     db,
		agg:    agg,

		workers:   make([]*readWorker, numWorkers),
		workersCh: make(chan *readTask, numWorkers*8),

		hphPool: sync.Pool{New: func() any {
			return commitment.NewHexPatriciaHashed(length.Addr, nil, dirs.Tmp)
		}},
	}

	for i := 0; i < numWorkers; i++ {
		dm.workers[i] = &readWorker{
			dm:     dm,
			taskCh: dm.workersCh,
		}
		dm.workersWg.Add(1)
		go dm.workers[i].loop()
	}
	return dm, nil
}

func (dm *DomainsManager) WithReader(fn func(reader DomainsReader) error) error {
	return dm.withReader_workerThread(fn)
}

func (dm *DomainsManager) withReader_callerThread(fn func(reader DomainsReader) error) error {
	reader, err := NewDomainsReader(dm.db, dm.agg)
	if err != nil {
		return err
	}
	defer reader.Close()
	return fn(reader)
}

func (dm *DomainsManager) withReader_workerThread(fn func(reader DomainsReader) error) error {
	task := &readTask{
		fn:    fn,
		retCh: make(chan error),
	}
	dm.workersCh <- task
	return <-task.retCh
}

func (dm *DomainsManager) WithWriter(blockNum uint64, fn func(writer DomainsWriter) error) error {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	writer, err := NewDomainsWriter(dm.db, dm.agg, blockNum)
	if err != nil {
		return err
	}
	defer writer.Close()

	if err := writer.SetBlockNum(blockNum); err != nil {
		return err
	}
	if err := fn(writer); err != nil {
		return err
	}
	if err := writer.WriteBlockNum(blockNum); err != nil {
		return err
	}
	if err := writer.Commit(); err != nil {
		return err
	}
	for _, worker := range dm.workers {
		worker.needReopen.Store(1)
	}
	return nil
}

func (dm *DomainsManager) Close() {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	close(dm.workersCh)
	dm.workersWg.Wait()

	dm.agg.Close()
	dm.db.Close()
}

func (dm *DomainsManager) StepSize() uint64 {
	return dm.agg.StepSize()
}

func (dm *DomainsManager) Tmpdir() string {
	return dm.dirs.Tmp
}

func (dm *DomainsManager) GetHph() *commitment.HexPatriciaHashed {
	return dm.hphPool.Get().(*commitment.HexPatriciaHashed)
}

func (dm *DomainsManager) ReturnHph(hph *commitment.HexPatriciaHashed) {
	dm.hphPool.Put(hph)
}

func (dm *DomainsManager) ReadBlockNumByRoot(root []byte) (blockNum uint64, ok bool, err error) {
	err = dm.WithReader(func(reader DomainsReader) error {
		if data, _, err := reader.DomainGetLatest(CustomDomain, rootKey(root)); err != nil {
			return err
		} else if len(data) < 8 {
			return nil // not found
		} else {
			blockNum = binary.BigEndian.Uint64(data)
			ok = true
			return nil
		}
	})
	return
}

func (dm *DomainsManager) WriteBlockNumByRoot(root []byte, blockNum uint64) error {
	return dm.WithWriter(blockNum, func(writer DomainsWriter) error {
		return writer.DomainPutOrDel(CustomDomain, rootKey(root), binary.BigEndian.AppendUint64([]byte{}, blockNum))
	})
}

// Treat each block as 1 Erigon transaction.
// e.g. Genesis block has one tx #1, so blockNum 0 = txNum 1.
func calcTxNum(blockNum uint64) uint64 {
	return blockNum + 1
}

func rootKey(rootHash []byte) []byte {
	return append(keyRootPrefix, common.BytesToHash(rootHash).Bytes()...)
}
