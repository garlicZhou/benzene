package bnz

import (
	"benzene/core"
	"benzene/core/types"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/libp2p/go-libp2p-core/peer"
)

// Benzene implements the Benzene full node service.
type Benzene struct {
	TxPool     *core.TxPool
	BlockChain *core.BlockChain
	NodeAPI    NodeAPI
	ShardID    uint32

	// DB interfaces
	chainDb ethdb.Database // Block chain database
}

// NodeAPI is the list of functions from node used to call rpc apis.
type NodeAPI interface {
	AddPendingTransaction(newTx *types.Transaction) error
	Blockchain() *core.BlockChain

	ListPeer(topic string) []peer.ID
	ListTopic() []string
}

// New creates a new Harmony object (including the
// initialisation of the common Harmony object)
func New(
	nodeAPI NodeAPI, txPool *core.TxPool, shardID uint32,
) *Benzene {
	chainDb := nodeAPI.Blockchain().ChainDb()

	bnz := &Benzene{
		BlockChain: nodeAPI.Blockchain(),
		TxPool:     txPool,
		chainDb:    chainDb,
		NodeAPI:    nodeAPI,
		ShardID:    shardID,
	}

	return bnz
}
