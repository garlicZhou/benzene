// Code generated by github.com/fjl/gencodec. DO NOT EDIT.

package core

import (
	"benzene/params"
	"encoding/json"
	"errors"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/math"
)

var _ = (*genesisSpecMarshaling)(nil)

// MarshalJSON marshals as JSON.
func (g Genesis) MarshalJSON() ([]byte, error) {
	type Genesis struct {
		Config     *params.ChainConfig                         `json:"config"`
		ShardID    uint64                                      `json:"shardID"`
		Timestamp  math.HexOrDecimal64                         `json:"timestamp"`
		Alloc      map[common.UnprefixedAddress]GenesisAccount `json:"alloc"          gencodec:"required"`
		Number     math.HexOrDecimal64                         `json:"number"`
		GasUsed    math.HexOrDecimal64                         `json:"gasUsed"`
		ParentHash common.Hash                                 `json:"parentHash"`
	}
	var enc Genesis
	enc.Config = g.Config
	enc.ShardID = g.ShardID
	enc.Timestamp = math.HexOrDecimal64(g.Timestamp)
	if g.Alloc != nil {
		enc.Alloc = make(map[common.UnprefixedAddress]GenesisAccount, len(g.Alloc))
		for k, v := range g.Alloc {
			enc.Alloc[common.UnprefixedAddress(k)] = v
		}
	}
	enc.Number = math.HexOrDecimal64(g.Number)
	enc.GasUsed = math.HexOrDecimal64(g.GasUsed)
	enc.ParentHash = g.ParentHash
	return json.Marshal(&enc)
}

// UnmarshalJSON unmarshals from JSON.
func (g *Genesis) UnmarshalJSON(input []byte) error {
	type Genesis struct {
		Config     *params.ChainConfig                         `json:"config"`
		ShardID    *uint64                                     `json:"shardID"`
		Timestamp  *math.HexOrDecimal64                        `json:"timestamp"`
		Alloc      map[common.UnprefixedAddress]GenesisAccount `json:"alloc"          gencodec:"required"`
		Number     *math.HexOrDecimal64                        `json:"number"`
		GasUsed    *math.HexOrDecimal64                        `json:"gasUsed"`
		ParentHash *common.Hash                                `json:"parentHash"`
	}
	var dec Genesis
	if err := json.Unmarshal(input, &dec); err != nil {
		return err
	}
	if dec.Config != nil {
		g.Config = dec.Config
	}
	if dec.ShardID != nil {
		g.ShardID = *dec.ShardID
	}
	if dec.Timestamp != nil {
		g.Timestamp = uint64(*dec.Timestamp)
	}
	if dec.Alloc == nil {
		return errors.New("missing required field 'alloc' for Genesis")
	}
	g.Alloc = make(GenesisAlloc, len(dec.Alloc))
	for k, v := range dec.Alloc {
		g.Alloc[common.Address(k)] = v
	}
	if dec.Number != nil {
		g.Number = uint64(*dec.Number)
	}
	if dec.GasUsed != nil {
		g.GasUsed = uint64(*dec.GasUsed)
	}
	if dec.ParentHash != nil {
		g.ParentHash = *dec.ParentHash
	}
	return nil
}
