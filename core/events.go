package core

import (
	"benzene/core/types"
	"github.com/ethereum/go-ethereum/common"
)

// NewTxsEvent is posted when a batch of transactions enter the transaction pool.
type NewTxsEvent struct{ Txs []*types.Transaction }

// NewMinedBlockEvent is posted when a block has been imported.
type NewMinedBlockEvent struct{ Block *types.Block }

// ChainEvent is the struct of chain event.
type ChainEvent struct {
	Block *types.Block
	Hash  common.Hash
}

// ChainSideEvent is chain side event.
type ChainSideEvent struct {
	Block *types.Block
}

// ChainHeadEvent is the struct of chain head event.
type ChainHeadEvent struct{ Block *types.Block }
