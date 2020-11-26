package node

import (
	"benzene/core"
	"benzene/internal/utils"
	"github.com/ethereum/go-ethereum/ethdb"
)

// genesisInitializer is a shardchain.DBInitializer adapter.
type genesisInitializer struct {
	node *Node
}

// InitChainDB sets up a new genesis block in the database for the given shard.
func (gi *genesisInitializer) InitChainDB(db ethdb.Database, shardID uint32) error {
	gi.node.SetupGenesisBlock(db, shardID)
	return nil
}

// SetupGenesisBlock sets up a genesis blockchain.
func (node *Node) SetupGenesisBlock(db ethdb.Database, shardID uint32) {
	utils.Logger().Info().Interface("shardID", shardID).Msg("setting up a brand new chain database")
	if shardID == node.NodeConfig.ShardID {
		node.isFirstTime = true
	}

	gspec := core.NewGenesisSpec(shardID)
	// Store genesis block into db.
	gspec.MustCommit(db)
}