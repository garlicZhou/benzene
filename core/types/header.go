package types

import (
	"encoding/json"
	"math/big"
	"unsafe"

	"golang.org/x/crypto/sha3"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/rs/zerolog"
)

type Header struct {
	fields headerFields
}

type headerFields struct {
	ParentHash common.Hash `json:"parentHash"       gencodec:"required"`
	Number     *big.Int    `json:"number"           gencodec:"required"`
	Time       *big.Int    `json:"timestamp"        gencodec:"required"`
	ShardID    uint32      `json:"shardID"          gencodec:"required"`
	Signature  [96]byte    `json:"signature"        gencodec:"required"`
	TxHash     common.Hash `json:"transactionsRoot" gencodec:"required"`
}

func (h Header) String() string {
	s, _ := json.Marshal(h)
	return string(s)
}

// NewHeader creates a new header object.
func NewHeader(parentHash common.Hash, number, time *big.Int, shardID uint32, signature [96]byte) *Header {
	return &Header{headerFields{
		ParentHash: parentHash,
		Number:     new(big.Int).Set(number),
		Time:       new(big.Int).Set(time),
		ShardID:    shardID,
		Signature:  signature,
	}}
}

// Hash returns the block hash of the header, which is simply the keccak256 hash of its RLP encoding.
func (h *Header) Hash() (hash common.Hash) {
	hw := sha3.NewLegacyKeccak256()
	rlp.Encode(hw, h)
	hw.Sum(hash[:0])
	return hash
}

// Size returns the approximate memory used by all internal contents.
func (h *Header) Size() common.StorageSize {
	return common.StorageSize(unsafe.Sizeof(*h)) +
		common.StorageSize((h.Number().BitLen()+
			h.Time().BitLen())/8,
		)
}

// Logger returns a sub-logger with block contexts added.
func (h *Header) Logger(logger *zerolog.Logger) *zerolog.Logger {
	nlogger := logger.
		With().
		Str("blockHash", h.Hash().Hex()).
		Uint32("blockShard", h.ShardID()).
		Uint64("blockNumber", h.Number().Uint64()).
		Logger()
	return &nlogger
}

// Getter & Setter.
func (h *Header) ParentHash() common.Hash {
	return h.fields.ParentHash
}

func (h *Header) SetParentHash(newParentHash common.Hash) {
	h.fields.ParentHash = newParentHash
}

func (h *Header) Number() *big.Int {
	return new(big.Int).Set(h.fields.Number)
}

func (h *Header) SetNumber(newNumber *big.Int) {
	h.fields.Number = new(big.Int).Set(newNumber)
}

func (h *Header) Time() *big.Int {
	return new(big.Int).Set(h.fields.Time)
}

func (h *Header) SetTime(newTime *big.Int) {
	h.fields.Time = new(big.Int).Set(newTime)
}

func (h *Header) ShardID() uint32 {
	return h.fields.ShardID
}

func (h *Header) SetShardID(newShardID uint32) {
	h.fields.ShardID = newShardID
}

func (h *Header) Signature() [96]byte {
	return h.fields.Signature
}

func (h *Header) SetSignature(newSignature [96]byte) {
	h.fields.Signature = newSignature
}

func (h *Header) TxHash() common.Hash {
	return h.fields.TxHash
}

func (h *Header) SetTxHash(newTxHash common.Hash) {
	h.fields.TxHash = newTxHash
}
