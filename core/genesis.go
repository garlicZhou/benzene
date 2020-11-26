package core

import (
	"benzene/core/rawdb"
	"benzene/core/state"
	"benzene/core/types"
	"benzene/internal/utils"
	"benzene/params"
	"encoding/json"
	"fmt"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethdb"
	"math/big"
	"os"
)

// Genesis specifies the header fields, state of a genesis block. It also defines hard
// fork switch-over blocks through the chain configuration.
type Genesis struct {
	Config    *params.ChainConfig `json:"config"`
	ShardID   uint32              `json:"shardID"`
	Timestamp uint64              `json:"timestamp"`
	Alloc     GenesisAlloc        `json:"alloc"          gencodec:"required"`

	// These fields are used for consensus tests. Please don't use them
	// in actual genesis blocks.
	Number     uint64      `json:"number"`
	GasUsed    uint64      `json:"gasUsed"`
	ParentHash common.Hash `json:"parentHash"`
}

// NewGenesisSpec creates a new genesis spec for the given network type and shard ID.
// Note that the shard state is NOT initialized.
func NewGenesisSpec(shardID uint32) *Genesis {
	genesisAlloc := make(GenesisAlloc)
	chainConfig := params.ChainConfig{}

	return &Genesis{
		Config:    &chainConfig,
		Alloc:     genesisAlloc,
		ShardID:   shardID,
		Timestamp: 1561734000, // GMT: Friday, June 28, 2019 3:00:00 PM. PST: Friday, June 28, 2019 8:00:00 AM
	}
}

// GenesisAlloc specifies the initial state that is part of the genesis block.
type GenesisAlloc map[common.Address]GenesisAccount

// UnmarshalJSON is to deserialize the data into GenesisAlloc.
func (ga *GenesisAlloc) UnmarshalJSON(data []byte) error {
	m := make(map[common.UnprefixedAddress]GenesisAccount)
	if err := json.Unmarshal(data, &m); err != nil {
		return err
	}
	*ga = make(GenesisAlloc)
	for addr, a := range m {
		(*ga)[common.Address(addr)] = a
	}
	return nil
}

// GenesisAccount is an account in the state of the genesis block.
type GenesisAccount struct {
	Code       []byte                      `json:"code,omitempty"`
	Storage    map[common.Hash]common.Hash `json:"storage,omitempty"`
	Balance    *big.Int                    `json:"balance" gencodec:"required"`
	Nonce      uint64                      `json:"nonce,omitempty"`
	PrivateKey []byte                      `json:"secretKey,omitempty"` // for tests
}

// ToBlock creates the genesis block and writes state of a genesis specification
// to the given database (or discards it if nil).
func (g *Genesis) ToBlock(db ethdb.Database) *types.Block {
	if db == nil {
		utils.Logger().Error().Msg("db should be initialized")
		os.Exit(1)
	}
	statedb, _ := state.New(common.Hash{}, state.NewDatabase(db))
	for addr, account := range g.Alloc {
		statedb.AddBalance(addr, account.Balance)
	}
	root := statedb.IntermediateRoot(false)
	head := &types.Header{
		ParentHash: g.ParentHash,
		Root:       root,
		Number:     new(big.Int).SetUint64(g.Number),
		Time:       new(big.Int).SetUint64(g.Timestamp),
		ShardID:    g.ShardID,
	}
	statedb.Commit()
	statedb.Database().TrieDB().Commit(root, true, nil)

	return types.NewBlock(head, nil)
}

// Commit writes the block and state of a genesis specification to the database.
// The block is committed as the canonical head block.
func (g *Genesis) Commit(db ethdb.Database) (*types.Block, error) {
	block := g.ToBlock(db)
	if block.Number().Sign() != 0 {
		return nil, fmt.Errorf("can't commit genesis block with number > 0")
	}

	if err := rawdb.WriteBlock(db, block); err != nil {
		return nil, err
	}
	if err := rawdb.WriteCanonicalHash(db, block.Hash(), block.NumberU64()); err != nil {
		return nil, err
	}
	if err := rawdb.WriteHeadBlockHash(db, block.Hash()); err != nil {
		return nil, err
	}
	if err := rawdb.WriteHeadHeaderHash(db, block.Hash()); err != nil {
		return nil, err
	}

	return block, nil
}

// MustCommit writes the genesis block and state to db, panicking on error.
// The block is committed as the canonical head block.
func (g *Genesis) MustCommit(db ethdb.Database) *types.Block {
	block, err := g.Commit(db)
	if err != nil {
		panic(err)
	}
	return block
}
