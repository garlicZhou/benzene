package bnz

import (
	consensus_engine "benzene/consensus/engine"
	"benzene/core"
	"benzene/core/types"
	"benzene/internal/bnzapi"
	"benzene/internal/chain"
	"benzene/node"
	"benzene/params"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/event"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/rpc"
)

// Benzene implements the Benzene full node service.
type Benzene struct {
	config *Config

	// Handlers
	txPool map[uint64]*core.TxPool
	// TODO: use Protocol design in go-ethereum (hongzicong)
	//protocolManager *ProtocolManager

	// DB interfaces
	chainDbs map[uint64]ethdb.Database // Blockchain database

	eventMux *event.TypeMux
	engine   consensus_engine.Engine

	shardChains core.Collection // Shard databases

	APIBackend *BnzAPIBackend
}

// New creates a new Benzene object (including the
// initialisation of the common Benzene object)
func New(stack *node.Node, config *Config) (*Benzene, error) {
	if config.NoPruning && config.TrieDirtyCache > 0 {
		if config.SnapshotCache > 0 {
			config.TrieCleanCache += config.TrieDirtyCache * 3 / 5
			config.SnapshotCache += config.TrieDirtyCache * 2 / 5
		} else {
			config.TrieCleanCache += config.TrieDirtyCache
		}
		config.TrieDirtyCache = 0
	}
	log.Info("Allocated trie memory caches", "clean", common.StorageSize(config.TrieCleanCache)*1024*1024, "dirty", common.StorageSize(config.TrieDirtyCache)*1024*1024)

	bnz := &Benzene{
		config:   config,
		txPool:   make(map[uint64]*core.TxPool),
		chainDbs: make(map[uint64]ethdb.Database),
		eventMux: stack.EventMux(),
	}

	var err error
	var chainConfig *params.ChainConfig
	for _, shardid := range stack.Config().ShardID {
		bnz.chainDbs[shardid], err = stack.OpenDatabase(shardid, "chaindata", config.DatabaseCache, config.DatabaseHandles, "bnz/db/chaindata/")
		if err != nil {
			return nil, err
		}
		chainConfig, _, genesisErr := core.SetupGenesisBlock(bnz.chainDbs[shardid], config.Genesis[shardid])
		if genesisErr != nil {
			return nil, genesisErr
		}
		bnz.engine = CreateConsensusEngine(stack, chainConfig, bnz.chainDbs)
		log.Info("Initialised chain configuration", "config", chainConfig)
	}

	var (
		cacheConfig = &core.CacheConfig{
			TrieCleanLimit:      config.TrieCleanCache,
			TrieCleanJournal:    stack.ResolvePath(config.TrieCleanCacheJournal),
			TrieCleanRejournal:  config.TrieCleanCacheRejournal,
			TrieCleanNoPrefetch: config.NoPrefetch,
			TrieDirtyLimit:      config.TrieDirtyCache,
			TrieDirtyDisabled:   config.NoPruning,
			TrieTimeLimit:       config.TrieTimeout,
			SnapshotLimit:       config.SnapshotCache,
		}
	)

	collection := core.NewCollection(bnz.chainDbs, bnz.engine, cacheConfig, chainConfig, bnz.shouldPreserve, &config.TxLookupLimit)
	bnz.shardChains = collection

	for _, shardid := range stack.Config().ShardID {
		if config.TxPool.Journal != "" {
			config.TxPool.Journal = stack.ResolvePath(config.TxPool.Journal)
		}
		bnz.txPool[shardid] = core.NewTxPool(config.TxPool, chainConfig, bnz.Blockchain(shardid))
	}
	bnz.APIBackend = &BnzAPIBackend{stack.Config().ExtRPCEnabled(), bnz}

	// Register the backend on the node
	stack.RegisterAPIs(bnz.APIs())
	stack.RegisterLifecycle(bnz)
	return bnz, nil
}

// CreateConsensusEngine creates the required type of consensus engine instance for an Ethereum service
func CreateConsensusEngine(stack *node.Node, chainConfig *params.ChainConfig, dbs map[uint64]ethdb.Database) consensus_engine.Engine {
	// TODO: return our consensus engine (hongzicong)
	return &chain.EngineImpl{}
}

// APIs return the collection of RPC services the benzene package offers.
// NOTE, some of these services probably need to be moved to somewhere else.
func (bnz *Benzene) APIs() []rpc.API {
	apis := bnzapi.GetAPIs(bnz.APIBackend)

	// TODO: provide more apis (hongzicong)
	return append(apis, []rpc.API{

	}...)
}

// isLocalBlock checks whether the specified block is mined
// by local miner accounts.
func (bnz *Benzene) isLocalBlock(block *types.Block) bool {
	// TODO: to verify whether a block is proposed locally (hongzicong)
	return false
}

// shouldPreserve checks whether we should preserve the given block
// during the chain reorg depending on whether the author of block
// is a local account.
func (bnz *Benzene) shouldPreserve(block *types.Block) bool {
	return bnz.isLocalBlock(block)
}

// Blockchain returns the blockchain for the node's current shard.
func (bnz *Benzene) Blockchain(shardid uint64) *core.BlockChain {
	bc, err := bnz.shardChains.ShardChain(shardid)
	if err != nil {
		log.Error("cannot get shard chain", "shardID", shardid, "err", err)
	}
	return bc
}

func (bnz *Benzene) TxPool(shardid uint64) *core.TxPool    { return bnz.txPool[shardid] }
func (bnz *Benzene) EventMux() *event.TypeMux              { return bnz.eventMux }
func (bnz *Benzene) Engine() consensus_engine.Engine       { return bnz.engine }
func (bnz *Benzene) ChainDb(shardid uint64) ethdb.Database { return bnz.chainDbs[shardid] }

// Start implements node.Lifecycle, starting all internal goroutines needed by the
// Benzene protocol implementation.
func (bnz *Benzene) Start() error {
	return nil
}

// Stop implements node.Lifecycle, terminating all internal goroutines used by the
// Benzene protocol.
func (bnz *Benzene) Stop() error {
	// Then stop everything else.
	for _, shardid := range bnz.config.ShardID {
		bnz.txPool[shardid].Stop()
	}
	bnz.shardChains.Close()
	bnz.eventMux.Stop()
	return nil
}
