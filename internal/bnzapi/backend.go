package bnzapi

import (
	"benzene/accounts"
	"benzene/core"
	"benzene/core/types"
	"benzene/params"
	"context"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/event"
	"github.com/ethereum/go-ethereum/rpc"
	"math/big"
)

// Backend interface provides the common API services (that are provided by
// both full and light clients) with access to necessary functions.
type Backend interface {
	// General Benzene API
	SuggestPrice(ctx context.Context) (*big.Int, error)
	ChainDb(shardid uint64) ethdb.Database
	AccountManager() *accounts.Manager
	ExtRPCEnabled() bool
	ShardID() []uint64

	// Blockchain API
	SetHead(shardid uint64, number uint64)
	HeaderByNumber(ctx context.Context, shardid uint64, number rpc.BlockNumber) (*types.Header, error)
	HeaderByHash(ctx context.Context, shardid uint64, hash common.Hash) (*types.Header, error)
	HeaderByNumberOrHash(ctx context.Context, shardid uint64, blockNrOrHash rpc.BlockNumberOrHash) (*types.Header, error)
	CurrentHeader(shardid uint64) *types.Header
	CurrentBlock(shardid uint64) *types.Block
	BlockByNumber(ctx context.Context, shardid uint64, number rpc.BlockNumber) (*types.Block, error)
	BlockByHash(ctx context.Context, shardid uint64, hash common.Hash) (*types.Block, error)
	BlockByNumberOrHash(ctx context.Context, shardid uint64, blockNrOrHash rpc.BlockNumberOrHash) (*types.Block, error)
	StateAndHeaderByNumber(ctx context.Context, shardid uint64, number rpc.BlockNumber) (*state.StateDB, *types.Header, error)
	StateAndHeaderByNumberOrHash(ctx context.Context, shardid uint64, blockNrOrHash rpc.BlockNumberOrHash) (*state.StateDB, *types.Header, error)
	SubscribeChainEvent(shardid uint64, ch chan<- core.ChainEvent) event.Subscription
	SubscribeChainHeadEvent(shardid uint64, ch chan<- core.ChainHeadEvent) event.Subscription
	SubscribeChainSideEvent(shardid uint64, ch chan<- core.ChainSideEvent) event.Subscription

	// Transaction pool API
	SendTx(ctx context.Context, signedTx *types.Transaction) error
	GetTransaction(ctx context.Context, shardid uint64, txHash common.Hash) (*types.Transaction, common.Hash, uint64, uint64, error)
	GetPoolTransactions(shardid uint64) (types.Transactions, error)
	GetPoolTransaction(shardid uint64, txHash common.Hash) *types.Transaction
	GetPoolNonce(ctx context.Context, shardid uint64, addr common.Address) (uint64, error)
	Stats(shardid uint64) (pending int, queued int)
	TxPoolContent(shardid uint64) (map[common.Address]types.Transactions, map[common.Address]types.Transactions)
	SubscribeNewTxsEvent(shardid uint64, ch chan<- core.NewTxsEvent) event.Subscription

	ChainConfig(shardid uint64) *params.ChainConfig
}

func GetAPIs(apiBackend Backend) []rpc.API {
	nonceLock := new(AddrLocker)
	return []rpc.API{
		{
			Namespace: "bnz",
			Version:   "1.0",
			Service:   NewPublicBlockChainAPI(apiBackend),
			Public:    true,
		}, {
			Namespace: "bnz",
			Version:   "1.0",
			Service:   NewPublicTransactionPoolAPI(apiBackend, nonceLock),
			Public:    true,
		},
	}
}
