package node

import (
	"benzene/core/types"
	"benzene/core"
)

// Node represents a protocol-participating node in the network
type Node struct {
	BlockChannel          chan *types.Block                 // The channel to send newly proposed blocks
	ConfirmedBlockChannel chan *types.Block                 // The channel to send confirmed blocks
}

// Blockchain returns the blockchain for the node's current shard.
func (node *Node) Blockchain() *core.BlockChain {
	shardID := node.NodeConfig.ShardID
	bc, err := node.shardChains.ShardChain(shardID)
	if err != nil {
		utils.Logger().Error().
			Uint32("shardID", shardID).
			Err(err).
			Msg("cannot get shard chain")
	}
	return bc
}