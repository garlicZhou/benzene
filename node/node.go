package node

import (
	"benzene/consensus"
	"benzene/core"
	"benzene/core/types"
	nodeconfig "benzene/internal/configs/node"
	"benzene/internal/utils"
	"benzene/p2p"
	"benzene/params"
	"fmt"
	"os"
	"time"
)

// Node represents a protocol-participating node in the network
type Node struct {
	BlockChannel          chan *types.Block // The channel to send newly proposed blocks
	ConfirmedBlockChannel chan *types.Block // The channel to send confirmed blocks

	shardChains core.Collection // Shard databases
	SelfPeer    p2p.Peer

	host p2p.Host // The p2p host used to send/receive p2p messages

	NodeConfig  *nodeconfig.ConfigType // node configuration, including group ID, shard ID, etc
	chainConfig params.ChainConfig     // Chain configuration.

	isFirstTime bool // the node was started with a fresh database

	unixTimeAtNodeStart int64
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

// Start kicks off the node message handling
func (node *Node) Start() error {
	// NOTE never gets here
	return nil
}

// New creates a new node.
func New(
	host p2p.Host,
	consensusObj *consensus.Consensus,
	chainDBFactory core.DBFactory,
) *Node {
	node := Node{}
	node.unixTimeAtNodeStart = time.Now().Unix()
	if consensusObj != nil {
		node.NodeConfig = nodeconfig.GetShardConfig(consensusObj.ShardID)
	} else {
		node.NodeConfig = nodeconfig.GetDefaultConfig()
	}

	if host != nil {
		node.host = host
		node.SelfPeer = host.GetSelfPeer()
	}

	chainConfig := params.ChainConfig{}
	node.chainConfig = chainConfig

	collection := core.NewCollection(
		chainDBFactory, &genesisInitializer{&node}, chain.Engine, &chainConfig,
	)
	node.shardChains = collection

	return &node
}

// ShutDown gracefully shut down the node server and dump the in-memory blockchain state into DB.
func (node *Node) ShutDown() {
	node.Blockchain().Stop()
	const msg = "Successfully shut down!\n"
	utils.Logger().Print(msg)
	fmt.Print(msg)
	os.Exit(0)
}
