package core

import (
	consensus_engine "benzene/consensus/engine"
	"benzene/core/rawdb"
	"benzene/core/types"
	"benzene/params"

	crand "crypto/rand"
	"math"
	"math/big"
	mrand "math/rand"
	"sync/atomic"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethdb"
	lru "github.com/hashicorp/golang-lru"
)

const (
	headerCacheLimit = 512
	tdCacheLimit     = 1024
	numberCacheLimit = 2048
)

type HeaderChain struct {
	config *params.ChainConfig

	chainDb       ethdb.Database
	genesisHeader *types.Header

	currentHeader     atomic.Value // Current head of the header chain (may be above the block chain!)
	currentHeaderHash common.Hash  // Hash of the current head of the header chain (prevent recomputing all the time)

	headerCache *lru.Cache // Cache for the most recent block headers
	numberCache *lru.Cache // Cache for the most recent block numbers

	procInterrupt func() bool

	rand   *mrand.Rand
	engine consensus_engine.Engine
}

// NewHeaderChain creates a new HeaderChain structure.
//  getValidator should return the parent's validator
//  procInterrupt points to the parent's interrupt semaphore
//  wg points to the parent's shutdown wait group
func NewHeaderChain(chainDb ethdb.Database, config *params.ChainConfig, engine consensus_engine.Engine, procInterrupt func() bool) (*HeaderChain, error) {
	headerCache, _ := lru.New(headerCacheLimit)
	numberCache, _ := lru.New(numberCacheLimit)

	// Seed a fast but crypto originating random generator
	seed, err := crand.Int(crand.Reader, big.NewInt(math.MaxInt64))
	if err != nil {
		return nil, err
	}

	hc := &HeaderChain{
		config:        config,
		chainDb:       chainDb,
		headerCache:   headerCache,
		numberCache:   numberCache,
		procInterrupt: procInterrupt,
		rand:          mrand.New(mrand.NewSource(seed.Int64())),
		engine:        engine,
	}

	hc.genesisHeader = hc.GetHeaderByNumber(0)
	if hc.genesisHeader == nil {
		return nil, ErrNoGenesis
	}

	hc.currentHeader.Store(hc.genesisHeader)
	if head := rawdb.ReadHeadBlockHash(chainDb); head != (common.Hash{}) {
		if chead := hc.GetHeaderByHash(head); chead != nil {
			hc.currentHeader.Store(chead)
		}
	}
	hc.currentHeaderHash = hc.CurrentHeader().Hash()

	return hc, nil
}

// GetBlockNumber retrieves the block number belonging to the given hash
// from the cache or database
func (hc *HeaderChain) GetBlockNumber(hash common.Hash) *uint64 {
	if cached, ok := hc.numberCache.Get(hash); ok {
		number := cached.(uint64)
		return &number
	}
	number := rawdb.ReadHeaderNumber(hc.chainDb, hash)
	if number != nil {
		hc.numberCache.Add(hash, *number)
	}
	return number
}

// GetHeader retrieves a block header from the database by hash and number,
// caching it if found.
func (hc *HeaderChain) GetHeader(hash common.Hash, number uint64) *types.Header {
	// Short circuit if the header's already in the cache, retrieve otherwise
	if header, ok := hc.headerCache.Get(hash); ok {
		return header.(*types.Header)
	}
	header := rawdb.ReadHeader(hc.chainDb, hash, number)
	if header == nil {
		return nil
	}
	// Cache the found header for next time and return
	hc.headerCache.Add(hash, header)
	return header
}

// GetHeaderByHash retrieves a block header from the database by hash, caching it if
// found.
func (hc *HeaderChain) GetHeaderByHash(hash common.Hash) *types.Header {
	number := hc.GetBlockNumber(hash)
	if number == nil {
		return nil
	}
	return hc.GetHeader(hash, *number)
}

// HasHeader checks if a block header is present in the database or not.
func (hc *HeaderChain) HasHeader(hash common.Hash, number uint64) bool {
	if hc.numberCache.Contains(hash) || hc.headerCache.Contains(hash) {
		return true
	}
	return rawdb.HasHeader(hc.chainDb, hash, number)
}

// GetHeaderByNumber retrieves a block header from the database by number,
// caching it (associated with its hash) if found.
func (hc *HeaderChain) GetHeaderByNumber(number uint64) *types.Header {
	hash := rawdb.ReadCanonicalHash(hc.chainDb, number)
	if hash == (common.Hash{}) {
		return nil
	}
	return hc.GetHeader(hash, number)
}

// CurrentHeader retrieves the current head header of the canonical chain. The
// header is retrieved from the HeaderChain's internal cache.
func (hc *HeaderChain) CurrentHeader() *types.Header {
	return hc.currentHeader.Load().(*types.Header)
}

// SetCurrentHeader sets the current head header of the canonical chain.
func (hc *HeaderChain) SetCurrentHeader(head *types.Header) error {
	if err := rawdb.WriteHeadHeaderHash(hc.chainDb, head.Hash()); err != nil {
		return err
	}

	hc.currentHeader.Store(head)
	hc.currentHeaderHash = head.Hash()
	return nil
}

// DeleteCallback is a callback function that is called by SetHead before
// each header is deleted.
type DeleteCallback func(rawdb.DatabaseDeleter, common.Hash, uint64) error

// SetHead rewinds the local chain to a new head. Everything above the new head
// will be deleted and the new one set.
func (hc *HeaderChain) SetHead(head uint64, delFn DeleteCallback) error {
	height := uint64(0)

	if hdr := hc.CurrentHeader(); hdr != nil {
		height = hdr.Number().Uint64()
	}
	batch := hc.chainDb.NewBatch()
	for hdr := hc.CurrentHeader(); hdr != nil && hdr.Number().Uint64() > head; hdr = hc.CurrentHeader() {
		hash := hdr.Hash()
		num := hdr.Number().Uint64()
		if delFn != nil {
			if err := delFn(batch, hash, num); err != nil {
				return err
			}
		}
		if err := rawdb.DeleteHeader(batch, hash, num); err != nil {
			return err
		}
		hc.currentHeader.Store(hc.GetHeader(hdr.ParentHash(), hdr.Number().Uint64()-1))
	}
	// Roll back the canonical chain numbering
	for i := height; i > head; i-- {
		if err := rawdb.DeleteCanonicalHash(batch, i); err != nil {
			return err
		}
	}
	if err := rawdb.WriteHeadHeaderHash(batch, hc.currentHeaderHash); err != nil {
		return err
	}

	if err := batch.Write(); err != nil {
		return err
	}

	// Clear out any stale content from the caches
	hc.headerCache.Purge()
	hc.numberCache.Purge()

	if hc.CurrentHeader() == nil {
		hc.currentHeader.Store(hc.genesisHeader)
	}
	hc.currentHeaderHash = hc.CurrentHeader().Hash()

	return nil
}

// SetGenesis sets a new genesis block header for the chain
func (hc *HeaderChain) SetGenesis(head *types.Header) {
	hc.genesisHeader = head
}

// Config retrieves the header chain's chain configuration.
func (hc *HeaderChain) Config() *params.ChainConfig { return hc.config }

// Engine retrieves the header chain's consensus engine.
func (hc *HeaderChain) Engine() consensus_engine.Engine { return hc.engine }

// GetBlock implements consensus.ChainReader, and returns nil for every input as
// a header chain does not have blocks available for retrieval.
func (hc *HeaderChain) GetBlock(hash common.Hash, number uint64) *types.Block {
	return nil
}