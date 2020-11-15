// Copyright 2018 The go-ethereum Authors
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

package types

import (
	"benzene/utils"
	"math/big"
	"sync/atomic"

	"github.com/ethereum/go-ethereum/common"
	"github.com/rs/zerolog"
)

// Constants for block.
var (
	EmptyRootHash = DeriveSha(Transactions{})
)

type Block struct {
	header       *Header
	transactions Transactions

	// caches
	hash atomic.Value
}

// Body is a simple (mutable, non-safe) data container for storing and moving
// a block's data contents (transactions) together.
type Body struct {
	Transactions []*Transaction
}

// NewBlock creates a new block. The input data is copied,
// changes to header and to the field values will not affect the
// block.
//
// The values of TxHash in header are ignored and set to
// values derived from the given txs.
func NewBlock(header *Header, txs []*Transaction) *Block {
	b := &Block{header: CopyHeader(header)}

	// TODO: panic if len(txs) != len(receipts)
	if len(txs) == 0 {
		b.header.SetTxHash(EmptyRootHash)
	} else {
		b.header.SetTxHash(DeriveSha(Transactions(txs)))
		b.transactions = make(Transactions, len(txs))
		copy(b.transactions, txs)
	}

	return b
}

// NewBlockWithHeader creates a block with the given header data. The
// header data is copied, changes to header and to the field values
// will not affect the block.
func NewBlockWithHeader(header *Header) *Block {
	return &Block{header: CopyHeader(header)}
}

// CopyHeader creates a deep copy of a block header to prevent side effects from
// modifying a header variable.
func CopyHeader(h *Header) *Header {
	cpy := *h
	if h.Number() != nil {
		cpy.SetNumber(h.Number())
	}
	return &cpy
}

// Number checks if block b1 is less than block b2.
func Number(b1, b2 *Block) bool {
	return b1.header.Number().Cmp(b2.header.Number()) < 0
}

// Logger returns a sub-logger with block contexts added.
func (b *Block) Logger(logger *zerolog.Logger) *zerolog.Logger {
	return b.header.Logger(logger)
}

// Transactions returns transactions.
func (b *Block) Transactions() Transactions {
	return b.transactions
}

// Time is header time.
func (b *Block) Time() *big.Int { return b.header.Time() }

// NumberU64 is the header number in uint64.
func (b *Block) NumberU64() uint64 { return b.header.Number().Uint64() }

func (b *Block) Header() *Header { return CopyHeader(b.header) }

// Body returns the non-header content of the block.
func (b *Block) Body() *Body { return &Body{b.transactions} }

// WithBody returns a new block with the given transaction and uncle contents.
func (b *Block) WithBody(transactions []*Transaction) *Block {
	block := &Block{
		header:       CopyHeader(b.header),
		transactions: make([]*Transaction, len(transactions)),
	}
	copy(block.transactions, transactions)
	return block
}

// Hash returns the keccak256 hash of b's header.
// The hash is computed on the first call and cached thereafter.
func (b *Block) Hash() common.Hash {
	if hash := b.hash.Load(); hash != nil {
		return hash.(common.Hash)
	}
	b.Logger(utils.Logger()).Debug().Msg("finalizing and caching block hash")
	v := b.header.Hash()
	b.hash.Store(v)
	return v
}

// Blocks is an array of Block.
type Blocks []*Block
