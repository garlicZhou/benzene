package core

import (
	consensus_engine "benzene/consensus/engine"
	"benzene/core/types"
	"benzene/params"
	"github.com/ethereum/go-ethereum/log"

	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/pkg/errors"
	"sync"
)

// Collection is a collection of per-shard blockchains.
type Collection interface {
	// ShardChain returns the blockchain for the given shard,
	// opening one as necessary.
	ShardChain(shardID uint64) (*BlockChain, error)

	// CloseShardChain closes the given shard chain.
	CloseShardChain(shardID uint64) error

	// Close closes all shard chains.
	Close() error
}

// CollectionImpl is the main implementation of the shard chain collection.
// See the Collection interface for details.
type CollectionImpl struct {
	chainDbs       map[uint64]ethdb.Database
	engine         consensus_engine.Engine
	mtx            sync.Mutex
	pool           map[uint64]*BlockChain
	cacheConfig    *CacheConfig
	chainConfig    *params.ChainConfig
	shouldPreserve func(*types.Block) bool
	txLookupLimit  *uint64
}

// NewCollection creates and returns a new shard chain collection.
//
// dbFactory is the shard chain database factory to use.
//
// dbInit is the shard chain initializer to use when the database returned by
// the factory is brand new (empty).
func NewCollection(
	chainDbs map[uint64]ethdb.Database,
	engine consensus_engine.Engine,
	cacheConfig *CacheConfig,
	chainConfig *params.ChainConfig,
	shouldPreserve func(block *types.Block) bool,
	txLookupLimit *uint64,
) *CollectionImpl {
	return &CollectionImpl{
		chainDbs:        chainDbs,
		engine:         engine,
		pool:           make(map[uint64]*BlockChain),
		cacheConfig:    cacheConfig,
		chainConfig:    chainConfig,
		shouldPreserve: shouldPreserve,
		txLookupLimit:  txLookupLimit,
	}
}

// ShardChain returns the blockchain for the given shard,
// opening one as necessary.
func (sc *CollectionImpl) ShardChain(shardID uint64) (*BlockChain, error) {
	sc.mtx.Lock()
	defer sc.mtx.Unlock()
	if bc, ok := sc.pool[shardID]; ok {
		return bc, nil
	}
	var db ethdb.Database
	defer func() {
		if db != nil {
			db.Close()
		}
	}()
	var err error
	bc, err := NewBlockChain(
		sc.chainDbs[shardID], sc.cacheConfig, sc.chainConfig, sc.engine, sc.shouldPreserve, sc.txLookupLimit,
	)
	if err != nil {
		return nil, errors.Wrapf(err, "cannot create blockchain")
	}
	db = nil // don't close
	sc.pool[shardID] = bc
	return bc, nil
}

// CloseShardChain closes the given shard chain.
func (sc *CollectionImpl) CloseShardChain(shardID uint64) error {
	sc.mtx.Lock()
	defer sc.mtx.Unlock()
	bc, ok := sc.pool[shardID]
	if !ok {
		return errors.Errorf("shard chain not found %d", shardID)
	}
	log.Info("closing shard chain", "shardID", shardID)
	delete(sc.pool, shardID)
	bc.Stop()
	bc.ChainDb().Close()
	log.Info("closed shard chain", "shardID", shardID)
	return nil
}

// Close closes all shard chains.
func (sc *CollectionImpl) Close() error {
	newPool := make(map[uint64]*BlockChain)
	sc.mtx.Lock()
	oldPool := sc.pool
	sc.pool = newPool
	sc.mtx.Unlock()
	for shardID, bc := range oldPool {
		log.Info("closing shard chain", "shardID", shardID)
		bc.Stop()
		bc.ChainDb().Close()
		log.Info("closed shard chain", "shardID", shardID)
	}
	return nil
}
