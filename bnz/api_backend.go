// Copyright 2015 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package bnz

import (
	"benzene/accounts"
	"benzene/core"
	"benzene/core/rawdb"
	"benzene/core/types"
	"benzene/params"
	"context"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/event"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/pkg/errors"
	"math/big"
)

// BnzAPIBackend implements bnzapi.Backend for full nodes
type BnzAPIBackend struct {
	extRPCEnabled bool
	bnz           *Benzene
}

// ChainConfig returns the active chain configuration.
func (b *BnzAPIBackend) ChainConfig(shardid uint64) *params.ChainConfig {
	return b.bnz.Blockchain(shardid).Config()
}

func (b *BnzAPIBackend) CurrentBlock(shardid uint64) *types.Block {
	return b.bnz.Blockchain(shardid).CurrentBlock()
}

func (b *BnzAPIBackend) SetHead(shardid uint64, number uint64) {
	//b.bnz.protocolManager.downloader.Cancel()
	b.bnz.Blockchain(shardid).SetHead(number)
}

func (b *BnzAPIBackend) HeaderByNumber(ctx context.Context, shardid uint64, number rpc.BlockNumber) (*types.Header, error) {
	// Pending block is only known by the miner
	if number == rpc.PendingBlockNumber {
		//block := b.bnz.miner.PendingBlock()
		//return block, nil
		// TODO: support pending block (hongzicong)
		return nil, errors.New("Not support pending block yet")
	}
	// Otherwise resolve and return the block
	if number == rpc.LatestBlockNumber {
		return b.bnz.Blockchain(shardid).CurrentBlock().Header(), nil
	}
	return b.bnz.Blockchain(shardid).GetHeaderByNumber(uint64(number)), nil
}

func (b *BnzAPIBackend) HeaderByNumberOrHash(ctx context.Context, shardid uint64, blockNrOrHash rpc.BlockNumberOrHash) (*types.Header, error) {
	if blockNr, ok := blockNrOrHash.Number(); ok {
		return b.HeaderByNumber(ctx, shardid, blockNr)
	}
	if hash, ok := blockNrOrHash.Hash(); ok {
		header := b.bnz.Blockchain(shardid).GetHeaderByHash(hash)
		if header == nil {
			return nil, errors.New("header for hash not found")
		}
		if blockNrOrHash.RequireCanonical && b.bnz.Blockchain(shardid).GetCanonicalHash(header.Number.Uint64()) != hash {
			return nil, errors.New("hash is not currently canonical")
		}
		return header, nil
	}
	return nil, errors.New("invalid arguments; neither block nor hash specified")
}

func (b *BnzAPIBackend) HeaderByHash(ctx context.Context, shardid uint64, hash common.Hash) (*types.Header, error) {
	return b.bnz.Blockchain(shardid).GetHeaderByHash(hash), nil
}

func (b *BnzAPIBackend) BlockByNumber(ctx context.Context, shardid uint64, number rpc.BlockNumber) (*types.Block, error) {
	// Pending block is only known by the miner
	if number == rpc.PendingBlockNumber {
		//block := b.bnz.miner.PendingBlock()
		//return block, nil
		// TODO: support pending block (hongzicong)
		return nil, errors.New("Not support pending block yet")
	}
	// Otherwise resolve and return the block
	if number == rpc.LatestBlockNumber {
		return b.bnz.Blockchain(shardid).CurrentBlock(), nil
	}
	return b.bnz.Blockchain(shardid).GetBlockByNumber(uint64(number)), nil
}

func (b *BnzAPIBackend) BlockByHash(ctx context.Context, shardid uint64, hash common.Hash) (*types.Block, error) {
	return b.bnz.Blockchain(shardid).GetBlockByHash(hash), nil
}

func (b *BnzAPIBackend) BlockByNumberOrHash(ctx context.Context, shardid uint64, blockNrOrHash rpc.BlockNumberOrHash) (*types.Block, error) {
	if blockNr, ok := blockNrOrHash.Number(); ok {
		return b.BlockByNumber(ctx, shardid, blockNr)
	}
	if hash, ok := blockNrOrHash.Hash(); ok {
		header := b.bnz.Blockchain(shardid).GetHeaderByHash(hash)
		if header == nil {
			return nil, errors.New("header for hash not found")
		}
		if blockNrOrHash.RequireCanonical && b.bnz.Blockchain(shardid).GetCanonicalHash(header.Number.Uint64()) != hash {
			return nil, errors.New("hash is not currently canonical")
		}
		block := b.bnz.Blockchain(shardid).GetBlock(hash, header.Number.Uint64())
		if block == nil {
			return nil, errors.New("header found, but block body is missing")
		}
		return block, nil
	}
	return nil, errors.New("invalid arguments; neither block nor hash specified")
}

func (b *BnzAPIBackend) StateAndHeaderByNumber(ctx context.Context, shardid uint64, number rpc.BlockNumber) (*state.StateDB, *types.Header, error) {
	// Pending state is only known by the miner
	if number == rpc.PendingBlockNumber {
		//block := b.bnz.miner.PendingBlock()
		//return block, nil
		// TODO: support pending block (hongzicong)
		return nil, nil, errors.New("Not support pending block yet")
	}
	// Otherwise resolve the block number and return its state
	header, err := b.HeaderByNumber(ctx, shardid, number)
	if err != nil {
		return nil, nil, err
	}
	if header == nil {
		return nil, nil, errors.New("header not found")
	}
	stateDb, err := b.bnz.Blockchain(shardid).StateAt(header.Root)
	return stateDb, header, err
}

func (b *BnzAPIBackend) StateAndHeaderByNumberOrHash(ctx context.Context, shardid uint64, blockNrOrHash rpc.BlockNumberOrHash) (*state.StateDB, *types.Header, error) {
	if blockNr, ok := blockNrOrHash.Number(); ok {
		return b.StateAndHeaderByNumber(ctx, shardid, blockNr)
	}
	if hash, ok := blockNrOrHash.Hash(); ok {
		header, err := b.HeaderByHash(ctx, shardid, hash)
		if err != nil {
			return nil, nil, err
		}
		if header == nil {
			return nil, nil, errors.New("header for hash not found")
		}
		if blockNrOrHash.RequireCanonical && b.bnz.Blockchain(shardid).GetCanonicalHash(header.Number.Uint64()) != hash {
			return nil, nil, errors.New("hash is not currently canonical")
		}
		stateDb, err := b.bnz.Blockchain(shardid).StateAt(header.Root)
		return stateDb, header, err
	}
	return nil, nil, errors.New("invalid arguments; neither block nor hash specified")
}

func (b *BnzAPIBackend) SubscribeRemovedLogsEvent(shardid uint64, ch chan<- core.RemovedLogsEvent) event.Subscription {
	return b.bnz.Blockchain(shardid).SubscribeRemovedLogsEvent(ch)
}

func (b *BnzAPIBackend) SubscribeChainEvent(shardid uint64, ch chan<- core.ChainEvent) event.Subscription {
	return b.bnz.Blockchain(shardid).SubscribeChainEvent(ch)
}

func (b *BnzAPIBackend) SubscribeChainHeadEvent(shardid uint64, ch chan<- core.ChainHeadEvent) event.Subscription {
	return b.bnz.Blockchain(shardid).SubscribeChainHeadEvent(ch)
}

func (b *BnzAPIBackend) SubscribeChainSideEvent(shardid uint64, ch chan<- core.ChainSideEvent) event.Subscription {
	return b.bnz.Blockchain(shardid).SubscribeChainSideEvent(ch)
}

func (b *BnzAPIBackend) SubscribeLogsEvent(shardid uint64, ch chan<- []*types.Log) event.Subscription {
	return b.bnz.Blockchain(shardid).SubscribeLogsEvent(ch)
}

func (b *BnzAPIBackend) SendTx(ctx context.Context, signedTx *types.Transaction) error {
	return b.bnz.AddPendingTransaction(signedTx)
}

func (b *BnzAPIBackend) GetPoolTransactions(shardid uint64) (types.Transactions, error) {
	pending, err := b.bnz.TxPool(shardid).Pending()
	if err != nil {
		return nil, err
	}
	var txs types.Transactions
	for _, batch := range pending {
		txs = append(txs, batch...)
	}
	return txs, nil
}

func (b *BnzAPIBackend) GetPoolTransaction(shardid uint64, hash common.Hash) *types.Transaction {
	return b.bnz.TxPool(shardid).Get(hash)
}

func (b *BnzAPIBackend) GetTransaction(ctx context.Context, shardid uint64, txHash common.Hash) (*types.Transaction, common.Hash, uint64, uint64, error) {
	tx, blockHash, blockNumber, index := rawdb.ReadTransaction(b.bnz.ChainDb(shardid), txHash)
	return tx, blockHash, blockNumber, index, nil
}

func (b *BnzAPIBackend) GetPoolNonce(ctx context.Context, shardid uint64, addr common.Address) (uint64, error) {
	return b.bnz.TxPool(shardid).Nonce(addr), nil
}

func (b *BnzAPIBackend) Stats(shardid uint64) (pending int, queued int) {
	return b.bnz.TxPool(shardid).Stats()
}

func (b *BnzAPIBackend) TxPoolContent(shardid uint64) (map[common.Address]types.Transactions, map[common.Address]types.Transactions) {
	return b.bnz.TxPool(shardid).Content()
}

func (b *BnzAPIBackend) TxPool(shardid uint64) *core.TxPool {
	return b.bnz.TxPool(shardid)
}

func (b *BnzAPIBackend) SubscribeNewTxsEvent(shardid uint64, ch chan<- core.NewTxsEvent) event.Subscription {
	return b.bnz.TxPool(shardid).SubscribeNewTxsEvent(ch)
}

func (b *BnzAPIBackend) SuggestPrice(ctx context.Context) (*big.Int, error) {
	//return b.gpo.SuggestPrice(ctx)
	return big.NewInt(0), nil
}

func (b *BnzAPIBackend) ChainDb(shardid uint64) ethdb.Database {
	return b.bnz.ChainDb(shardid)
}

func (b *BnzAPIBackend) AccountManager() *accounts.Manager {
	return b.bnz.AccountManager()
}

func (b *BnzAPIBackend) ExtRPCEnabled() bool {
	return b.extRPCEnabled
}

func (b *BnzAPIBackend) ShardID() []uint64 {
	return b.bnz.config.ShardID
}

func (b *BnzAPIBackend) EventMux() *event.TypeMux {
	return b.bnz.EventMux()
}

func (b *BnzAPIBackend) CurrentHeader(shardid uint64) *types.Header {
	return b.bnz.Blockchain(shardid).CurrentHeader()
}
