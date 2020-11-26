package core

import (
	consensus_engine "benzene/consensus/engine"
	"benzene/core/rawdb"
	"benzene/core/state"
	"benzene/core/types"
	"benzene/internal/utils"
	"benzene/params"
	"encoding/json"
	"github.com/ethereum/go-ethereum/metrics"
	"github.com/ethereum/go-ethereum/rlp"
	"math/big"
	"os"
	"strings"
	"sync"

	"fmt"
	"sort"
	"sync/atomic"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/mclock"
	"github.com/ethereum/go-ethereum/common/prque"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/event"

	lru "github.com/hashicorp/golang-lru"

	"github.com/pkg/errors"
)

var (
	// blockInsertTimer
	blockInsertTimer = metrics.NewRegisteredTimer("chain/inserts", nil)
	// ErrNoGenesis is the error when there is no genesis.
	ErrNoGenesis = errors.New("Genesis not found in chain")
)

const (
	bodyCacheLimit      = 256
	blockCacheLimit     = 256
	maxFutureBlocks     = 256
	maxTimeFutureBlocks = 30
	badBlockLimit       = 10
	triesInMemory       = 128
)

// CacheConfig contains the configuration values for the trie caching/pruning
// that's resident in a blockchain.
type CacheConfig struct {
	Disabled      bool          // Whether to disable trie write caching (archive node)
	TrieNodeLimit int           // Memory limit (MB) at which to flush the current in-memory trie to disk
	TrieTimeLimit time.Duration // Time limit after which to flush the current in-memory trie to disk
}

// BlockChain represents the canonical chain given a database with a genesis
// block. The Blockchain manages chain imports, reverts, chain reorganisations.
//
// Importing blocks in to the block chain happens according to the set of rules
// defined by the two stage validator. Processing of blocks is done using the
// Processor which processes the included transaction. The validation of the state
// is done in the second part of the validator. Failing results in aborting of
// the import.
//
// The BlockChain also helps in returning blocks from **any** chain included
// in the database as well as blocks that represents the canonical chain. It's
// important to note that GetBlock can return any block and does not need to be
// included in the canonical one where as GetBlockByNumber always represents the
// canonical chain.
type BlockChain struct {
	chainConfig *params.ChainConfig // Chain & network configuration
	cacheConfig *CacheConfig        // Cache configuration for pruning

	db     ethdb.Database // Low level persistent database to store final content in
	triegc *prque.Prque   // Priority queue mapping block numbers to tries to gc
	gcproc time.Duration  // Accumulates canonical block processing for trie dumping

	hc            *HeaderChain
	rmLogsFeed    event.Feed
	chainFeed     event.Feed
	chainSideFeed event.Feed
	chainHeadFeed event.Feed
	logsFeed      event.Feed
	scope         event.SubscriptionScope
	genesisBlock  *types.Block

	mu      sync.RWMutex // global mutex for locking chain operations
	chainmu sync.RWMutex // blockchain insertion lock
	procmu  sync.RWMutex // block processor lock

	currentBlock     atomic.Value // Current head of the block chain
	currentFastBlock atomic.Value // Current head of the fast-sync chain (may be above the block chain!)

	stateCache   state.Database // State database to reuse between imports (contains state cache)
	bodyCache    *lru.Cache     // Cache for the most recent block bodies
	bodyRLPCache *lru.Cache     // Cache for the most recent block bodies in RLP encoded format
	blockCache   *lru.Cache     // Cache for the most recent entire blocks
	futureBlocks *lru.Cache     // future blocks are blocks added for later processing

	quit    chan struct{}  // blockchain quit channel
	wg      sync.WaitGroup // chain processing wait group for shutting down
	running int32          // 0 if chain is running, 1 when stopped

	// procInterrupt must be atomically called
	procInterrupt int32 // interrupt signaler for block processing

	engine                 consensus_engine.Engine
	processor              Processor // Block transaction processor interface
	validator              Validator // Block and state validator interface
	maxGarbCollectedBlkNum int64

	badBlocks *lru.Cache // Bad block cache
}

// NewBlockChain returns a fully initialised block chain using information
// available in the database. It initialises the default Ethereum validator and
// Processor.
func NewBlockChain(db ethdb.Database, cacheConfig *CacheConfig, chainConfig *params.ChainConfig, engine consensus_engine.Engine) (*BlockChain, error) {

	if cacheConfig == nil {
		cacheConfig = &CacheConfig{
			TrieNodeLimit: 256 * 1024 * 1024,
			TrieTimeLimit: 2 * time.Minute,
		}
	}

	bodyCache, _ := lru.New(bodyCacheLimit)
	bodyRLPCache, _ := lru.New(bodyCacheLimit)
	blockCache, _ := lru.New(blockCacheLimit)
	futureBlocks, _ := lru.New(maxFutureBlocks)
	badBlocks, _ := lru.New(badBlockLimit)

	bc := &BlockChain{
		chainConfig:            chainConfig,
		cacheConfig:            cacheConfig,
		db:                     db,
		triegc:                 prque.New(nil),
		stateCache:             state.NewDatabase(db),
		quit:                   make(chan struct{}),
		bodyCache:              bodyCache,
		bodyRLPCache:           bodyRLPCache,
		blockCache:             blockCache,
		futureBlocks:           futureBlocks,
		engine:                 engine,
		badBlocks:              badBlocks,
		maxGarbCollectedBlkNum: -1,
	}
	bc.SetValidator(NewBlockValidator(chainConfig, bc, engine))
	bc.SetProcessor(NewStateProcessor(chainConfig, bc, engine))

	var err error
	bc.hc, err = NewHeaderChain(db, chainConfig, engine, bc.getProcInterrupt)
	if err != nil {
		return nil, err
	}
	bc.genesisBlock = bc.GetBlockByNumber(0)
	if bc.genesisBlock == nil {
		return nil, ErrNoGenesis
	}
	var nilBlock *types.Block
	bc.currentBlock.Store(nilBlock)
	bc.currentFastBlock.Store(nilBlock)
	if err := bc.loadLastState(); err != nil {
		return nil, err
	}
	// Take ownership of this particular state
	go bc.update()
	return bc, nil
}


// ValidateNewBlock validates new block.
func (bc *BlockChain) ValidateNewBlock(block *types.Block) error {
	state, err := state.New(bc.CurrentBlock().Root(), bc.stateCache)

	if err != nil {
		return err
	}

	// NOTE Order of mutating state here matters.
	// Process block using the parent state as reference point.
	_, err = bc.processor.Process(
		block, state,
	)
	if err != nil {
		bc.reportBlock(block, err)
		return err
	}

	// Verify all the hash roots (state, txns, receipts, cross-shard)
	if err := bc.Validator().ValidateState(
		block, state,
	); err != nil {
		bc.reportBlock(block, err)
		return err
	}
	return nil
}

func (bc *BlockChain) getProcInterrupt() bool {
	return atomic.LoadInt32(&bc.procInterrupt) == 1
}

// loadLastState loads the last known chain state from the database. This method
// assumes that the chain manager mutex is held.
func (bc *BlockChain) loadLastState() error {
	// Restore the last known head block
	head := rawdb.ReadHeadBlockHash(bc.db)
	if head == (common.Hash{}) {
		// Corrupt or empty database, init from scratch
		utils.Logger().Warn().Msg("Empty database, resetting chain")
		return bc.Reset()
	}
	// Make sure the entire head block is available
	currentBlock := bc.GetBlockByHash(head)
	if currentBlock == nil {
		// Corrupt or empty database, init from scratch
		utils.Logger().Warn().Str("hash", head.Hex()).Msg("Head block missing, resetting chain")
		return bc.Reset()
	}
	// TODO: Figure out why (hongzicong)
	// Make sure the state associated with the block is available
	//if _, err := state.New(currentBlock.Root(), bc.stateCache); err != nil {
	//	// Dangling block without a state associated, init from scratch
	//	utils.Logger().Warn().
	//		Str("number", currentBlock.Number().String()).
	//		Str("hash", currentBlock.Hash().Hex()).
	//		Msg("Head state missing, repairing chain")
	//	if err := bc.repair(&currentBlock); err != nil {
	//		return err
	//	}
	//}
	// Everything seems to be fine, set as the head block
	bc.currentBlock.Store(currentBlock)

	// Restore the last known head header
	currentHeader := currentBlock.Header()
	// TODO: Figure out why dont need (hongzicong)
	//if head := rawdb.ReadHeadHeaderHash(bc.db); head != (common.Hash{}) {
	//	if header := bc.GetHeaderByHash(head); header != nil {
	//		currentHeader = header
	//	}
	//}
	if err := bc.hc.SetCurrentHeader(currentHeader); err != nil {
		return errors.Wrap(err, "headerChain SetCurrentHeader")
	}

	// Restore the last known head fast block
	bc.currentFastBlock.Store(currentBlock)
	if head := rawdb.ReadHeadFastBlockHash(bc.db); head != (common.Hash{}) {
		if block := bc.GetBlockByHash(head); block != nil {
			bc.currentFastBlock.Store(block)
		}
	}

	// Issue a status log for the user
	currentFastBlock := bc.CurrentFastBlock()

	utils.Logger().Info().
		Str("number", currentHeader.Number.String()).
		Str("hash", currentHeader.Hash().Hex()).
		Str("age", common.PrettyAge(time.Unix(currentHeader.Time.Int64(), 0)).String()).
		Msg("Loaded most recent local header")
	utils.Logger().Info().
		Str("number", currentBlock.Number().String()).
		Str("hash", currentBlock.Hash().Hex()).
		Str("age", common.PrettyAge(time.Unix(currentBlock.Time().Int64(), 0)).String()).
		Msg("Loaded most recent local full block")
	utils.Logger().Info().
		Str("number", currentFastBlock.Number().String()).
		Str("hash", currentFastBlock.Hash().Hex()).
		Str("age", common.PrettyAge(time.Unix(currentFastBlock.Time().Int64(), 0)).String()).
		Msg("Loaded most recent local fast block")

	return nil
}

// SetHead rewinds the local chain to a new head. In the case of headers, everything
// above the new head will be deleted and the new one set. In the case of blocks
// though, the head may be further rewound if block bodies are missing (non-archive
// nodes after a fast sync).
func (bc *BlockChain) SetHead(head uint64) error {
	utils.Logger().Warn().Uint64("target", head).Msg("Rewinding blockchain")

	bc.mu.Lock()
	defer bc.mu.Unlock()

	// Rewind the header chain, deleting all block bodies until then
	delFn := func(db rawdb.DatabaseDeleter, hash common.Hash, num uint64) error {
		return rawdb.DeleteBody(db, hash, num)
	}
	if err := bc.hc.SetHead(head, delFn); err != nil {
		return errors.Wrap(err, "headerChain SetHeader")
	}
	currentHeader := bc.hc.CurrentHeader()

	// Clear out any stale content from the caches
	bc.bodyCache.Purge()
	bc.bodyRLPCache.Purge()
	bc.blockCache.Purge()
	bc.futureBlocks.Purge()

	// Rewind the block chain, ensuring we don't end up with a stateless head block
	if currentBlock := bc.CurrentBlock(); currentBlock != nil && currentHeader.Number.Uint64() < currentBlock.NumberU64() {
		bc.currentBlock.Store(bc.GetBlock(currentHeader.Hash(), currentHeader.Number.Uint64()))
	}
	if currentBlock := bc.CurrentBlock(); currentBlock != nil {
		if _, err := state.New(currentBlock.Root(), bc.stateCache); err != nil {
			// Rewound state missing, rolled back to before pivot, reset to genesis
			bc.currentBlock.Store(bc.genesisBlock)
		}
	}
	// Rewind the fast block in a simpleton way to the target head
	if currentFastBlock := bc.CurrentFastBlock(); currentFastBlock != nil && currentHeader.Number.Uint64() < currentFastBlock.NumberU64() {
		bc.currentFastBlock.Store(bc.GetBlock(currentHeader.Hash(), currentHeader.Number.Uint64()))
	}
	// If either blocks reached nil, reset to the genesis state
	if currentBlock := bc.CurrentBlock(); currentBlock == nil {
		bc.currentBlock.Store(bc.genesisBlock)
	}
	if currentFastBlock := bc.CurrentFastBlock(); currentFastBlock == nil {
		bc.currentFastBlock.Store(bc.genesisBlock)
	}
	currentBlock := bc.CurrentBlock()
	currentFastBlock := bc.CurrentFastBlock()

	if err := rawdb.WriteHeadBlockHash(bc.db, currentBlock.Hash()); err != nil {
		return err
	}
	if err := rawdb.WriteHeadFastBlockHash(bc.db, currentFastBlock.Hash()); err != nil {
		return err
	}

	return bc.loadLastState()
}

// ShardID returns the shard Id of the blockchain.
// TODO: use a better solution before resharding shuffle nodes to different shards
func (bc *BlockChain) ShardID() uint32 {
	return bc.CurrentBlock().ShardID()
}

// CurrentBlock retrieves the current head block of the canonical chain. The
// block is retrieved from the blockchain's internal cache.
func (bc *BlockChain) CurrentBlock() *types.Block {
	return bc.currentBlock.Load().(*types.Block)
}

// CurrentFastBlock retrieves the current fast-sync head block of the canonical
// chain. The block is retrieved from the blockchain's internal cache.
func (bc *BlockChain) CurrentFastBlock() *types.Block {
	return bc.currentFastBlock.Load().(*types.Block)
}

// SetProcessor sets the processor required for making state modifications.
func (bc *BlockChain) SetProcessor(processor Processor) {
	bc.procmu.Lock()
	defer bc.procmu.Unlock()
	bc.processor = processor
}

// SetValidator sets the validator which is used to validate incoming blocks.
func (bc *BlockChain) SetValidator(validator Validator) {
	bc.procmu.Lock()
	defer bc.procmu.Unlock()
	bc.validator = validator
}

// Validator returns the current validator.
func (bc *BlockChain) Validator() Validator {
	bc.procmu.RLock()
	defer bc.procmu.RUnlock()
	return bc.validator
}

// Processor returns the current processor.
func (bc *BlockChain) Processor() Processor {
	bc.procmu.RLock()
	defer bc.procmu.RUnlock()
	return bc.processor
}

// State returns a new mutable state based on the current HEAD block.
func (bc *BlockChain) State() (*state.DB, error) {
	return bc.StateAt(bc.CurrentBlock().Root())
}

// StateAt returns a new mutable state based on a particular point in time.
func (bc *BlockChain) StateAt(root common.Hash) (*state.DB, error) {
	return state.New(root, bc.stateCache)
}

// Reset purges the entire blockchain, restoring it to its genesis state.
func (bc *BlockChain) Reset() error {
	return bc.ResetWithGenesisBlock(bc.genesisBlock)
}

// ResetWithGenesisBlock purges the entire blockchain, restoring it to the
// specified genesis state.
func (bc *BlockChain) ResetWithGenesisBlock(genesis *types.Block) error {
	// Dump the entire block chain and purge the caches
	if err := bc.SetHead(0); err != nil {
		return err
	}
	bc.mu.Lock()
	defer bc.mu.Unlock()

	// Prepare the genesis block and reinitialise the chain
	if err := rawdb.WriteBlock(bc.db, genesis); err != nil {
		return err
	}

	bc.genesisBlock = genesis
	if err := bc.insert(bc.genesisBlock); err != nil {
		return err
	}
	bc.hc.SetGenesis(bc.genesisBlock.Header())
	if err := bc.hc.SetCurrentHeader(bc.genesisBlock.Header()); err != nil {
		return err
	}
	bc.currentBlock.Store(bc.genesisBlock)
	bc.currentFastBlock.Store(bc.genesisBlock)

	return nil
}

// writeHeadBlock writes a new head block
func (bc *BlockChain) writeHeadBlock(block *types.Block) error {
	// If the block is on a side chain or an unknown one, force other heads onto it too
	updateHeads := rawdb.ReadCanonicalHash(bc.db, block.NumberU64()) != block.Hash()

	// Add the block to the canonical chain number scheme and mark as the head
	batch := bc.ChainDb().NewBatch()
	if err := rawdb.WriteCanonicalHash(batch, block.Hash(), block.NumberU64()); err != nil {
		return err
	}
	if err := rawdb.WriteHeadBlockHash(batch, block.Hash()); err != nil {
		return err
	}
	if err := batch.Write(); err != nil {
		return err
	}

	bc.currentBlock.Store(block)

	// If the block is better than our head or is on a different chain, force update heads
	if updateHeads {
		if err := bc.hc.SetCurrentHeader(block.Header()); err != nil {
			return errors.Wrap(err, "HeaderChain SetCurrentHeader")
		}
		if err := rawdb.WriteHeadFastBlockHash(bc.db, block.Hash()); err != nil {
			return err
		}

		bc.currentFastBlock.Store(block)
	}
	return nil
}

// insert injects a new head block into the current block chain. This method
// assumes that the block is indeed a true head. It will also reset the head
// header and the head fast sync block to this very same block if they are older
// or if they are on a different side chain.
//
// Note, this function assumes that the `mu` mutex is held!
func (bc *BlockChain) insert(block *types.Block) error {
	return bc.writeHeadBlock(block)
}

// Genesis retrieves the chain's genesis block.
func (bc *BlockChain) Genesis() *types.Block {
	return bc.genesisBlock
}

// GetBody retrieves a block body (transactions and uncles) from the database by
// hash, caching it if found.
func (bc *BlockChain) GetBody(hash common.Hash) *types.Body {
	// Short circuit if the body's already in the cache, retrieve otherwise
	if cached, ok := bc.bodyCache.Get(hash); ok {
		body := cached.(*types.Body)
		return body
	}
	number := bc.hc.GetBlockNumber(hash)
	if number == nil {
		return nil
	}
	body := rawdb.ReadBody(bc.db, hash, *number)
	if body == nil {
		return nil
	}
	// Cache the found body for next time and return
	bc.bodyCache.Add(hash, body)
	return body
}

// GetBodyRLP retrieves a block body in RLP encoding from the database by hash,
// caching it if found.
func (bc *BlockChain) GetBodyRLP(hash common.Hash) rlp.RawValue {
	// Short circuit if the body's already in the cache, retrieve otherwise
	if cached, ok := bc.bodyRLPCache.Get(hash); ok {
		return cached.(rlp.RawValue)
	}
	number := bc.hc.GetBlockNumber(hash)
	if number == nil {
		return nil
	}
	body := rawdb.ReadBodyRLP(bc.db, hash, *number)
	if len(body) == 0 {
		return nil
	}
	// Cache the found body for next time and return
	bc.bodyRLPCache.Add(hash, body)
	return body
}

// HasBlock checks if a block is fully present in the database or not.
func (bc *BlockChain) HasBlock(hash common.Hash, number uint64) bool {
	if bc.blockCache.Contains(hash) {
		return true
	}
	return rawdb.HasBody(bc.db, hash, number)
}

// HasState checks if state trie is fully present in the database or not.
func (bc *BlockChain) HasState(hash common.Hash) bool {
	_, err := bc.stateCache.OpenTrie(hash)
	return err == nil
}

// HasBlockAndState checks if a block and associated state trie is fully present
// in the database or not, caching it if present.
func (bc *BlockChain) HasBlockAndState(hash common.Hash, number uint64) bool {
	// Check first that the block itself is known
	block := bc.GetBlock(hash, number)
	if block == nil {
		return false
	}
	return bc.HasState(block.Root())
}

// GetBlock retrieves a block from the database by hash and number,
// caching it if found.
func (bc *BlockChain) GetBlock(hash common.Hash, number uint64) *types.Block {
	// Short circuit if the block's already in the cache, retrieve otherwise
	if block, ok := bc.blockCache.Get(hash); ok {
		return block.(*types.Block)
	}
	block := rawdb.ReadBlock(bc.db, hash, number)
	if block == nil {
		return nil
	}
	// Cache the found block for next time and return
	bc.blockCache.Add(block.Hash(), block)
	return block
}

// GetBlockByHash retrieves a block from the database by hash, caching it if found.
func (bc *BlockChain) GetBlockByHash(hash common.Hash) *types.Block {
	number := bc.hc.GetBlockNumber(hash)
	if number == nil {
		return nil
	}
	return bc.GetBlock(hash, *number)
}

// GetBlockByNumber retrieves a block from the database by number, caching it
// (associated with its hash) if found.
func (bc *BlockChain) GetBlockByNumber(number uint64) *types.Block {
	hash := rawdb.ReadCanonicalHash(bc.db, number)
	if hash == (common.Hash{}) {
		return nil
	}
	return bc.GetBlock(hash, number)
}

// Stop stops the blockchain service. If any imports are currently in progress
// it will abort them using the procInterrupt.
func (bc *BlockChain) Stop() {
	if !atomic.CompareAndSwapInt32(&bc.running, 0, 1) {
		return
	}
	// Unsubscribe all subscriptions registered from blockchain
	bc.scope.Close()
	close(bc.quit)
	bc.StopInsert()
	bc.wg.Wait()

	// Ensure the state of a recent block is also stored to disk before exiting.
	// We're writing three different states to catch different restart scenarios:
	//  - HEAD:     So we don't need to reprocess any blocks in the general case
	//  - HEAD-1:   So we don't do large reorgs if our HEAD becomes an uncle
	//  - HEAD-127: So we have a hard limit on the number of blocks reexecuted
	if !bc.cacheConfig.Disabled {
		triedb := bc.stateCache.TrieDB()

		for _, offset := range []uint64{0, 1, triesInMemory - 1} {
			if number := bc.CurrentBlock().NumberU64(); number > offset {
				recent := bc.GetHeaderByNumber(number - offset)
				if recent != nil {
					utils.Logger().Info().
						Str("block", recent.Number.String()).
						Str("hash", recent.Hash().Hex()).
						Str("root", recent.Root.Hex()).
						Msg("Writing cached state to disk")
					if err := triedb.Commit(recent.Root, true, nil); err != nil {
						utils.Logger().Error().Err(err).Msg("Failed to commit recent state trie")
					}
				}
			}
		}
		for !bc.triegc.Empty() {
			triedb.Dereference(bc.triegc.PopItem().(common.Hash))
		}
		if size, _ := triedb.Size(); size != 0 {
			utils.Logger().Error().Msg("Dangling trie nodes after full cleanup")
		}
	}
	utils.Logger().Info().Msg("Blockchain manager stopped")
}

// StopInsert interrupts all insertion methods, causing them to return
// errInsertionInterrupted as soon as possible. Insertion is permanently disabled after
// calling this method.
func (bc *BlockChain) StopInsert() {
	atomic.StoreInt32(&bc.procInterrupt, 1)
}

// insertStopped returns true after StopInsert has been called.
func (bc *BlockChain) insertStopped() bool {
	return atomic.LoadInt32(&bc.procInterrupt) == 1
}

func (bc *BlockChain) procFutureBlocks() {
	blocks := make([]*types.Block, 0, bc.futureBlocks.Len())
	for _, hash := range bc.futureBlocks.Keys() {
		if block, exist := bc.futureBlocks.Peek(hash); exist {
			blocks = append(blocks, block.(*types.Block))
		}
	}
	if len(blocks) > 0 {
		sort.Slice(blocks, func(i, j int) bool {
			return blocks[i].NumberU64() < blocks[j].NumberU64()
		})

		// Insert one by one as chain insertion needs contiguous ancestry between blocks
		for i := range blocks {
			bc.InsertChain(blocks[i:i+1], true /* verifyHeaders */)
		}
	}
}

var lastWrite uint64

// WriteBlockWithoutState writes only the block and its metadata to the database,
// but does not write any state. This is used to construct competing side forks
// up to the point where they exceed the canonical total difficulty.
func (bc *BlockChain) WriteBlockWithoutState(block *types.Block) (err error) {
	bc.wg.Add(1)
	defer bc.wg.Done()

	if err := rawdb.WriteBlock(bc.db, block); err != nil {
		return err
	}

	return nil
}

// WriteBlockWithState writes the block and all associated state to the database.
func (bc *BlockChain) WriteBlockWithState(block *types.Block, state *state.DB) (status WriteStatus, err error) {
	bc.wg.Add(1)
	defer bc.wg.Done()

	// Make sure no inconsistent state is leaked during insertion
	bc.mu.Lock()
	defer bc.mu.Unlock()

	currentBlock := bc.CurrentBlock()
	if currentBlock == nil || block.ParentHash() != currentBlock.Hash() {
		return NonStatTy, errors.New("hash of parent block doesn't match the current block hash")
	}

	// Commit state object changes to in-memory trie
	root, err := state.Commit()
	if err != nil {
		return NonStatTy, err
	}

	// If we're running an archive node, always flush
	triedb := bc.stateCache.TrieDB()
	if bc.cacheConfig.Disabled {
		if err := triedb.Commit(root, false, nil); err != nil {
			if isUnrecoverableErr(err) {
				fmt.Printf("Unrecoverable error when committing triedb: %v\nExitting\n", err)
				os.Exit(1)
			}
			return NonStatTy, err
		}
	} else {
		// Full but not archive node, do proper garbage collection
		triedb.Reference(root, common.Hash{}) // metadata reference to keep trie alive
		bc.triegc.Push(root, -int64(block.NumberU64()))

		if current := block.NumberU64(); current > triesInMemory {
			// If we exceeded our memory allowance, flush matured singleton nodes to disk
			var (
				nodes, imgs = triedb.Size()
				limit       = common.StorageSize(bc.cacheConfig.TrieNodeLimit) * 1024 * 1024
			)
			if nodes > limit || imgs > 4*1024*1024 {
				triedb.Cap(limit - ethdb.IdealBatchSize)
			}
			// Find the next state trie we need to commit
			header := bc.GetHeaderByNumber(current - triesInMemory)
			if header != nil {
				chosen := header.Number.Uint64()

				// If we exceeded out time allowance, flush an entire trie to disk
				if bc.gcproc > bc.cacheConfig.TrieTimeLimit {
					// If we're exceeding limits but haven't reached a large enough memory gap,
					// warn the user that the system is becoming unstable.
					if chosen < lastWrite+triesInMemory && bc.gcproc >= 2*bc.cacheConfig.TrieTimeLimit {
						utils.Logger().Info().
							Dur("time", bc.gcproc).
							Dur("allowance", bc.cacheConfig.TrieTimeLimit).
							Float64("optimum", float64(chosen-lastWrite)/triesInMemory).
							Msg("State in memory for too long, committing")
					}
					// Flush an entire trie and restart the counters
					triedb.Commit(header.Root, true, nil)
					lastWrite = chosen
					bc.gcproc = 0
				}
				// Garbage collect anything below our required write retention
				for !bc.triegc.Empty() {
					root, number := bc.triegc.Pop()
					if uint64(-number) > chosen {
						bc.triegc.Push(root, number)
						break
					}
					if -number > bc.maxGarbCollectedBlkNum {
						bc.maxGarbCollectedBlkNum = -number
					}
					triedb.Dereference(root.(common.Hash))
				}
			}
		}
	}

	batch := bc.db.NewBatch()
	// Write the raw block
	if err := rawdb.WriteBlock(batch, block); err != nil {
		return NonStatTy, err
	}

	// Write the positional metadata for transaction/receipt lookups and preimages
	if err := rawdb.WriteBlockTxLookUpEntries(batch, block); err != nil {
		return NonStatTy, err
	}

	if err := batch.Write(); err != nil {
		if isUnrecoverableErr(err) {
			fmt.Printf("Unrecoverable error when writing leveldb: %v\nExitting\n", err)
			os.Exit(1)
		}
		return NonStatTy, err
	}

	// Update current block
	if err := bc.writeHeadBlock(block); err != nil {
		return NonStatTy, errors.Wrap(err, "writeHeadBlock")
	}

	bc.futureBlocks.Remove(block.Hash())
	return CanonStatTy, nil
}

// InsertChain attempts to insert the given batch of blocks in to the canonical
// chain or, otherwise, create a fork. If an error is returned it will return
// the index number of the failing block as well an error describing what went
// wrong.
//
// After insertion is done, all accumulated events will be fired.
func (bc *BlockChain) InsertChain(chain types.Blocks, verifyHeaders bool) (int, error) {
	n, events, logs, err := bc.insertChain(chain, verifyHeaders)
	bc.PostChainEvents(events, logs)
	return n, err
}

// insertChain will execute the actual chain insertion and event aggregation. The
// only reason this method exists as a separate one is to make locking cleaner
// with deferred statements.
func (bc *BlockChain) insertChain(chain types.Blocks, verifyHeaders bool) (int, []interface{}, []*types.Log, error) {
	// Sanity check that we have something meaningful to import
	if len(chain) == 0 {
		return 0, nil, nil, nil
	}

	// Do a sanity check that the provided chain is actually ordered and linked
	for i := 1; i < len(chain); i++ {
		if chain[i].NumberU64() != chain[i-1].NumberU64()+1 || chain[i].ParentHash() != chain[i-1].Hash() {
			// Chain broke ancestry, log a message (programming error) and skip insertion
			utils.Logger().Error().
				Str("number", chain[i].Number().String()).
				Str("hash", chain[i].Hash().Hex()).
				Str("parent", chain[i].ParentHash().Hex()).
				Str("prevnumber", chain[i-1].Number().String()).
				Str("prevhash", chain[i-1].Hash().Hex()).
				Msg("insertChain: non contiguous block insert")

			return 0, nil, nil, fmt.Errorf("non contiguous insert: item %d is #%d [%x…], item %d is #%d [%x…] (parent [%x…])", i-1, chain[i-1].NumberU64(),
				chain[i-1].Hash().Bytes()[:4], i, chain[i].NumberU64(), chain[i].Hash().Bytes()[:4], chain[i].ParentHash().Bytes()[:4])
		}
	}

	// Pre-checks passed, start the full block imports
	bc.wg.Add(1)
	defer bc.wg.Done()

	bc.chainmu.Lock()
	defer bc.chainmu.Unlock()

	// A queued approach to delivering events. This is generally
	// faster than direct delivery and requires much less mutex
	// acquiring.
	var (
		stats         = insertStats{startTime: mclock.Now()}
		events        = make([]interface{}, 0, len(chain))
		lastCanon     *types.Block
		coalescedLogs []*types.Log
	)

	var verifyHeadersResults <-chan error

	// If the block header chain has not been verified, conduct header verification here.
	if verifyHeaders {
		headers := make([]*types.Header, len(chain))
		seals := make([]bool, len(chain))

		for i, block := range chain {
			headers[i] = block.Header()
			seals[i] = true
		}
		// Note that VerifyHeaders verifies headers in the chain in parallel
		abort, results := bc.Engine().VerifyHeaders(bc, headers, seals)
		verifyHeadersResults = results
		defer close(abort)
	}

	// Start a parallel signature recovery (signer will fluke on fork transition, minimal perf loss)
	//senderCacher.recoverFromBlocks(types.MakeSigner(bc.chainConfig, chain[0].Number()), chain)

	// Iterate over the blocks and insert when the verifier permits
	for i, block := range chain {
		// If the chain is terminating, stop processing blocks
		if atomic.LoadInt32(&bc.procInterrupt) == 1 {
			utils.Logger().Debug().Msg("Premature abort during blocks processing")
			break
		}
		// Wait for the block's verification to complete
		bstart := time.Now()

		var err error
		if verifyHeaders {
			err = <-verifyHeadersResults
		}
		if err == nil {
			err = bc.Validator().ValidateBody(block)
		}

		switch {
		case err == ErrKnownBlock:
			// Block and state both already known. However if the current block is below
			// this number we did a rollback and we should reimport it nonetheless.
			if bc.CurrentBlock().NumberU64() >= block.NumberU64() {
				stats.ignored++
				continue
			}

		case err == consensus_engine.ErrFutureBlock:
			// Allow up to MaxFuture second in the future blocks. If this limit is exceeded
			// the chain is discarded and processed at a later time if given.
			max := big.NewInt(time.Now().Unix() + maxTimeFutureBlocks)
			if block.Time().Cmp(max) > 0 {
				return i, events, coalescedLogs, fmt.Errorf("future block: %v > %v", block.Time(), max)
			}
			bc.futureBlocks.Add(block.Hash(), block)
			stats.queued++
			continue

		case err == consensus_engine.ErrUnknownAncestor && bc.futureBlocks.Contains(block.ParentHash()):
			bc.futureBlocks.Add(block.Hash(), block)
			stats.queued++
			continue

		case err == consensus_engine.ErrPrunedAncestor:
			// TODO: add fork choice mechanism
			// Block competing with the canonical chain, store in the db, but don't process
			// until the competitor TD goes above the canonical TD
			//currentBlock := bc.CurrentBlock()
			//localTd := bc.GetTd(currentBlock.Hash(), currentBlock.NumberU64())
			//externTd := new(big.Int).Add(bc.GetTd(block.ParentHash(), block.NumberU64()-1), block.Difficulty())
			//if localTd.Cmp(externTd) > 0 {
			//	if err = bc.WriteBlockWithoutState(block, externTd); err != nil {
			//		return i, events, coalescedLogs, err
			//	}
			//	continue
			//}
			// Competitor chain beat canonical, gather all blocks from the common ancestor
			var winner []*types.Block

			parent := bc.GetBlock(block.ParentHash(), block.NumberU64()-1)
			for parent != nil && !bc.HasState(parent.Root()) {
				winner = append(winner, parent)
				parent = bc.GetBlock(parent.ParentHash(), parent.NumberU64()-1)
			}
			for j := 0; j < len(winner)/2; j++ {
				winner[j], winner[len(winner)-1-j] = winner[len(winner)-1-j], winner[j]
			}
			// Prune in case non-empty winner chain
			if len(winner) > 0 {
				// Import all the pruned blocks to make the state available
				bc.chainmu.Unlock()
				_, evs, logs, err := bc.insertChain(winner, true /* verifyHeaders */)
				bc.chainmu.Lock()
				events, coalescedLogs = evs, logs

				if err != nil {
					return i, events, coalescedLogs, err
				}
			}

		case err != nil:
			bc.reportBlock(block, err)
			return i, events, coalescedLogs, err
		}

		// Create a new statedb using the parent block and report an
		// error if it fails.
		var parent *types.Block
		if i == 0 {
			parent = bc.GetBlock(block.ParentHash(), block.NumberU64()-1)
		} else {
			parent = chain[i-1]
		}
		state, err := state.New(parent.Root(), bc.stateCache)
		if err != nil {
			return i, events, coalescedLogs, err
		}

		// Process block using the parent state as reference point.
		logs, err := bc.processor.Process(
			block, state,
		)
		if err != nil {
			bc.reportBlock(block, err)
			return i, events, coalescedLogs, err
		}

		// Validate the state using the default validator
		if err := bc.Validator().ValidateState(
			block, state,
		); err != nil {
			bc.reportBlock(block, err)
			return i, events, coalescedLogs, err
		}
		proctime := time.Since(bstart)

		// Write the block to the chain and get the status.
		status, err := bc.WriteBlockWithState(
			block, state,
		)
		if err != nil {
			return i, events, coalescedLogs, err
		}
		logger := utils.Logger().With().
			Str("number", block.Number().String()).
			Str("hash", block.Hash().Hex()).
			Int("txs", len(block.Transactions())).
			Str("elapsed", common.PrettyDuration(time.Since(bstart)).String()).
			Logger()

		switch status {
		case CanonStatTy:
			logger.Info().Msg("Inserted new block")
			coalescedLogs = append(coalescedLogs, logs...)
			blockInsertTimer.UpdateSince(bstart)
			events = append(events, ChainEvent{block, block.Hash(), logs})
			lastCanon = block

			// Only count canonical blocks for GC processing time
			bc.gcproc += proctime
		}

		stats.processed++
		cache, _ := bc.stateCache.TrieDB().Size()
		stats.report(chain, i, cache)
	}

	// Append a single chain head event if we've progressed the chain
	if lastCanon != nil && bc.CurrentBlock().Hash() == lastCanon.Hash() {
		events = append(events, ChainHeadEvent{lastCanon})
	}

	return 0, events, coalescedLogs, nil
}

func countTransactions(chain []*types.Block) (c int) {
	for _, b := range chain {
		c += len(b.Transactions())
	}
	return c
}

// PostChainEvents iterates over the events generated by a chain insertion and
// posts them into the event feed.
// TODO: Should not expose PostChainEvents. The chain events should be posted in WriteBlock.
func (bc *BlockChain) PostChainEvents(events []interface{}, logs []*types.Log) {
	// post event logs for further processing
	if logs != nil {
		bc.logsFeed.Send(logs)
	}
	for _, event := range events {
		switch ev := event.(type) {
		case ChainEvent:
			bc.chainFeed.Send(ev)

		case ChainHeadEvent:
			bc.chainHeadFeed.Send(ev)

		case ChainSideEvent:
			bc.chainSideFeed.Send(ev)
		}
	}
}

func (bc *BlockChain) update() {
	futureTimer := time.NewTicker(5 * time.Second)
	defer futureTimer.Stop()
	for {
		select {
		case <-futureTimer.C:
			bc.procFutureBlocks()
		case <-bc.quit:
			return
		}
	}
}

// BadBlock ..
type BadBlock struct {
	Block  *types.Block
	Reason error
}

// MarshalJSON ..
func (b BadBlock) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Block  *types.Header `json:"header"`
		Reason string        `json:"error-cause"`
	}{
		b.Block.Header(),
		b.Reason.Error(),
	})
}

// BadBlocks returns a list of the last 'bad blocks' that
// the client has seen on the network
func (bc *BlockChain) BadBlocks() []BadBlock {
	blocks := make([]BadBlock, bc.badBlocks.Len())
	for _, hash := range bc.badBlocks.Keys() {
		if blk, exist := bc.badBlocks.Peek(hash); exist {
			blocks = append(blocks, blk.(BadBlock))
		}
	}
	return blocks
}

// addBadBlock adds a bad block to the bad-block LRU cache
func (bc *BlockChain) addBadBlock(block *types.Block, reason error) {
	bc.badBlocks.Add(block.Hash(), BadBlock{block, reason})
}

// reportBlock logs a bad block error.
func (bc *BlockChain) reportBlock(block *types.Block, err error) {
	bc.addBadBlock(block, err)

	utils.Logger().Error().Msgf(`
########## BAD BLOCK #########
Chain config: %v
Number: %v
NumTxn: %v
Hash: 0x%x
Error: %v
##############################
`, bc.chainConfig,
		block.Number(),
		len(block.Transactions()),
		block.Hash(),
		err,
	)
}

// CurrentHeader retrieves the current head header of the canonical chain. The
// header is retrieved from the HeaderChain's internal cache.
func (bc *BlockChain) CurrentHeader() *types.Header {
	return bc.hc.CurrentHeader()
}

// GetHeader retrieves a block header from the database by hash and number,
// caching it if found.
func (bc *BlockChain) GetHeader(hash common.Hash, number uint64) *types.Header {
	return bc.hc.GetHeader(hash, number)
}

// GetHeaderByHash retrieves a block header from the database by hash, caching it if
// found.
func (bc *BlockChain) GetHeaderByHash(hash common.Hash) *types.Header {
	return bc.hc.GetHeaderByHash(hash)
}

// GetHeaderByNumber retrieves a block header from the database by number,
// caching it (associated with its hash) if found.
func (bc *BlockChain) GetHeaderByNumber(number uint64) *types.Header {
	return bc.hc.GetHeaderByNumber(number)
}

// Config retrieves the chain's fork configuration.
func (bc *BlockChain) Config() *params.ChainConfig { return bc.chainConfig }

// Engine retrieves the blockchain's consensus engine.
func (bc *BlockChain) Engine() consensus_engine.Engine { return bc.engine }

// ChainDb returns the database
func (bc *BlockChain) ChainDb() ethdb.Database { return bc.db }

// WriteStatus status of write
type WriteStatus byte

// Constants for WriteStatus
const (
	NonStatTy WriteStatus = iota
	CanonStatTy
	SideStatTy
)

var (
	leveldbErrSpec         = "leveldb"
	tooManyOpenFilesErrStr = "Too many open files"
)

// isUnrecoverableErr check whether the input error is not recoverable.
// When writing db, there could be some possible errors from storage level (leveldb).
// Known possible leveldb errors are:
//  1. Leveldb is already closed. (leveldb.ErrClosed)
//  2. ldb file missing from disk. (leveldb.ErrNotFound)
//  3. Corrupted db data. (leveldb.errors.ErrCorrupted)
//  4. OS error when open file (too many open files, ...)
//  5. OS error when write file (read-only, not enough disk space, ...)
// Among all the above leveldb errors, only `too many open files` error is known to be recoverable,
// thus the unrecoverable errors refers to error that is
//  1. The error is from the lower storage level (from module leveldb)
//  2. The error is not too many files error.
func isUnrecoverableErr(err error) bool {
	isLeveldbErr := strings.Contains(err.Error(), leveldbErrSpec)
	isTooManyOpenFiles := strings.Contains(err.Error(), tooManyOpenFilesErrStr)
	return isLeveldbErr && !isTooManyOpenFiles
}
