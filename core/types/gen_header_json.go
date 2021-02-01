// Code generated by github.com/fjl/gencodec. DO NOT EDIT.

package types

import (
	"encoding/json"
	"errors"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

var _ = (*headerMarshaling)(nil)

// MarshalJSON marshals as JSON.
func (h Header) MarshalJSON() ([]byte, error) {
	type Header struct {
		ParentHash common.Hash    `json:"parentHash"       gencodec:"required"`
		Root       common.Hash    `json:"stateRoot"        gencodec:"required"`
		TxHash     common.Hash    `json:"transactionsRoot" gencodec:"required"`
		Number     *hexutil.Big   `json:"number"           gencodec:"required"`
		GasLimit   hexutil.Uint64 `json:"gasLimit"         gencodec:"required"`
		GasUsed    hexutil.Uint64 `json:"gasUsed"          gencodec:"required"`
		Time       *hexutil.Big   `json:"timestamp"        gencodec:"required"`
		ShardID    uint64         `json:"shardID"          gencodec:"required"`
		Hash       common.Hash    `json:"hash"`
	}
	var enc Header
	enc.ParentHash = h.ParentHash
	enc.Root = h.Root
	enc.TxHash = h.TxHash
	enc.Number = (*hexutil.Big)(h.Number)
	enc.GasLimit = hexutil.Uint64(h.GasLimit)
	enc.GasUsed = hexutil.Uint64(h.GasUsed)
	enc.Time = (*hexutil.Big)(h.Time)
	enc.ShardID = h.ShardID
	enc.Hash = h.Hash()
	return json.Marshal(&enc)
}

// UnmarshalJSON unmarshals from JSON.
func (h *Header) UnmarshalJSON(input []byte) error {
	type Header struct {
		ParentHash *common.Hash    `json:"parentHash"       gencodec:"required"`
		Root       *common.Hash    `json:"stateRoot"        gencodec:"required"`
		TxHash     *common.Hash    `json:"transactionsRoot" gencodec:"required"`
		Number     *hexutil.Big    `json:"number"           gencodec:"required"`
		GasLimit   *hexutil.Uint64 `json:"gasLimit"         gencodec:"required"`
		GasUsed    *hexutil.Uint64 `json:"gasUsed"          gencodec:"required"`
		Time       *hexutil.Big    `json:"timestamp"        gencodec:"required"`
		ShardID    *uint64         `json:"shardID"          gencodec:"required"`
	}
	var dec Header
	if err := json.Unmarshal(input, &dec); err != nil {
		return err
	}
	if dec.ParentHash == nil {
		return errors.New("missing required field 'parentHash' for Header")
	}
	h.ParentHash = *dec.ParentHash
	if dec.Root == nil {
		return errors.New("missing required field 'stateRoot' for Header")
	}
	h.Root = *dec.Root
	if dec.TxHash == nil {
		return errors.New("missing required field 'transactionsRoot' for Header")
	}
	h.TxHash = *dec.TxHash
	if dec.Number == nil {
		return errors.New("missing required field 'number' for Header")
	}
	h.Number = (*big.Int)(dec.Number)
	if dec.GasLimit == nil {
		return errors.New("missing required field 'gasLimit' for Header")
	}
	h.GasLimit = uint64(*dec.GasLimit)
	if dec.GasUsed == nil {
		return errors.New("missing required field 'gasUsed' for Header")
	}
	h.GasUsed = uint64(*dec.GasUsed)
	if dec.Time == nil {
		return errors.New("missing required field 'timestamp' for Header")
	}
	h.Time = (*big.Int)(dec.Time)
	if dec.ShardID == nil {
		return errors.New("missing required field 'shardID' for Header")
	}
	h.ShardID = *dec.ShardID
	return nil
}
