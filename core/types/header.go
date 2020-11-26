package types

import (
	"encoding/json"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"math/big"
	"unsafe"

	"github.com/ethereum/go-ethereum/common"
	"github.com/rs/zerolog"
)

//go:generate gencodec -type Header -field-override headerMarshaling -out gen_header_json.go

type Header struct {
	ParentHash common.Hash `json:"parentHash"       gencodec:"required"`
	Root       common.Hash `json:"stateRoot"        gencodec:"required"`
	TxHash     common.Hash `json:"transactionsRoot" gencodec:"required"`
	Number     *big.Int    `json:"number"           gencodec:"required"`
	Time       *big.Int    `json:"timestamp"        gencodec:"required"`
	ShardID    uint32      `json:"shardID"          gencodec:"required"`
}

// field type overrides for gencodec
type headerMarshaling struct {
	Number     *hexutil.Big
	Time       *hexutil.Big
	Hash       common.Hash `json:"hash"` // adds call to Hash() in MarshalJSON
}

func (h Header) String() string {
	s, _ := json.Marshal(h)
	return string(s)
}

// Hash returns the block hash of the header, which is simply the keccak256 hash of its
// RLP encoding.
func (h *Header) Hash() common.Hash {
	return rlpHash(h)
}

// Size returns the approximate memory used by all internal contents.
func (h *Header) Size() common.StorageSize {
	return common.StorageSize(unsafe.Sizeof(*h)) +
		common.StorageSize((h.Number.BitLen()+
			h.Time.BitLen())/8,
		)
}

// Logger returns a sub-logger with block contexts added.
func (h *Header) Logger(logger *zerolog.Logger) *zerolog.Logger {
	nlogger := logger.
		With().
		Str("blockHash", h.Hash().Hex()).
		Uint32("blockShard", h.ShardID).
		Uint64("blockNumber", h.Number.Uint64()).
		Logger()
	return &nlogger
}