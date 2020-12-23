package bnz

import (
	"benzene/core"
	"benzene/core/types"
	"github.com/libp2p/go-libp2p-core/peer"
)

// NodeAPI is the list of functions from node used to call rpc apis.
type NodeAPI interface {
	AddPendingTransaction(newTx *types.Transaction) error
	Blockchain() *core.BlockChain

	ListPeer(topic string) []peer.ID
	ListTopic() []string
}

