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
	"benzene/internal/utils"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
	"golang.org/x/crypto/sha3"
	"math/big"
	"sync"
	"sync/atomic"

	"github.com/ethereum/go-ethereum/common"
	"github.com/rs/zerolog"
)

// Constants for block.
var (
	EmptyRootHash = DeriveSha(Transactions{})
)

// hasherPool holds LegacyKeccak hashers.
var hasherPool = sync.Pool{
	New: func() interface{} {
		return sha3.NewLegacyKeccak256()
	},
}

func rlpHash(x interface{}) (h common.Hash) {
	sha := hasherPool.Get().(crypto.KeccakState)
	defer hasherPool.Put(sha)
	sha.Reset()
	rlp.Encode(sha, x)
	sha.Read(h[:])
	return h
}

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
		b.header.TxHash = EmptyRootHash
	} else {
		b.header.TxHash = DeriveSha(Transactions(txs))
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
	if cpy.Number = new(big.Int); h.Number != nil {
		cpy.Number.Set(h.Number)
	}
	if cpy.Time = new(big.Int); h.Time != nil {
		cpy.Time.Set(h.Time)
	}
	return &cpy
}

// Number checks if block b1 is less than block b2.
func Number(b1, b2 *Block) bool {
	return b1.header.Number.Cmp(b2.header.Number) < 0
}

// Logger returns a sub-logger with block contexts added.
func (b *Block) Logger(logger *zerolog.Logger) *zerolog.Logger {
	return b.header.Logger(logger)
}

// Transactions returns transactions.
func (b *Block) Transactions() Transactions {
	return b.transactions
}

// Number returns header number.
func (b *Block) Number() *big.Int { return b.header.Number }

// Time is header time.
func (b *Block) Time() *big.Int { return b.header.Time }

// NumberU64 is the header number in uint64.
func (b *Block) NumberU64() uint64 { return b.header.Number.Uint64() }

// ShardID is the header ShardID.
func (b *Block) ShardID() uint32 { return b.header.ShardID }

// Root is the header root.
func (b *Block) Root() common.Hash { return b.header.Root }

// ParentHash return header parent hash.
func (b *Block) ParentHash() common.Hash { return b.header.ParentHash }

// TxHash returns header tx hash.
func (b *Block) TxHash() common.Hash { return b.header.TxHash }

// Header returns header.
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
