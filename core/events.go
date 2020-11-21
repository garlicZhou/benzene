package core

import (
	"benzene/core/types"
	"github.com/ethereum/go-ethereum/common"
)

// NewTxsEvent is posted when a batch of transactions enter the transaction pool.
type NewTxsEvent struct{ Txs []*types.Transaction }

// PendingLogsEvent is posted pre mining and notifies of pending logs.
type PendingLogsEvent struct {
	Logs []*types.Log
}

// NewMinedBlockEvent is posted when a block has been imported.
type NewMinedBlockEvent struct{ Block *types.Block }

// RemovedLogsEvent is posted when a reorg happens
type RemovedLogsEvent struct{ Logs []*types.Log }

// ChainEvent is the struct of chain event.
type ChainEvent struct {
	Block *types.Block
	Hash  common.Hash
	Logs  []*types.Log
}

// ChainSideEvent is chain side event.
type ChainSideEvent struct {
	Block *types.Block
}

// ChainHeadEvent is the struct of chain head event.
type ChainHeadEvent struct{ Block *types.Block }
