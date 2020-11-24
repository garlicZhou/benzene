package core

import (
	nodeconfig "benzene/internal/configs/node"
	"benzene/params"
	"github.com/ethereum/go-ethereum/common"
	"math/big"
)

// Genesis specifies the header fields, state of a genesis block. It also defines hard
// fork switch-over blocks through the chain configuration.
type Genesis struct {
	Config         *params.ChainConfig  `json:"config"`
	Factory        blockfactory.Factory `json:"-"`
	Nonce          uint64               `json:"nonce"`
	ShardID        uint32               `json:"shardID"`
	Timestamp      uint64               `json:"timestamp"`
	Alloc          GenesisAlloc         `json:"alloc"          gencodec:"required"`
	ShardStateHash common.Hash          `json:"shardStateHash" gencodec:"required"`
	ShardState     shard.State          `json:"shardState"     gencodec:"required"`
}

// NewGenesisSpec creates a new genesis spec for the given network type and shard ID.
// Note that the shard state is NOT initialized.
func NewGenesisSpec(netType nodeconfig.NetworkType, shardID uint32) *Genesis {
	genesisAlloc := make(GenesisAlloc)
	chainConfig := params.ChainConfig{}

	return &Genesis{
		Config:    &chainConfig,
		Factory:   blockfactory.NewFactory(&chainConfig),
		Alloc:     genesisAlloc,
		ShardID:   shardID,
		Timestamp: 1561734000, // GMT: Friday, June 28, 2019 3:00:00 PM. PST: Friday, June 28, 2019 8:00:00 AM
	}
}

// GenesisAlloc specifies the initial state that is part of the genesis block.
type GenesisAlloc map[common.Address]GenesisAccount

// GenesisAccount is an account in the state of the genesis block.
type GenesisAccount struct {
	Code       []byte                      `json:"code,omitempty"`
	Storage    map[common.Hash]common.Hash `json:"storage,omitempty"`
	Balance    *big.Int                    `json:"balance" gencodec:"required"`
	Nonce      uint64                      `json:"nonce,omitempty"`
	PrivateKey []byte                      `json:"secretKey,omitempty"` // for tests
}