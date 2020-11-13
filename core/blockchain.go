package core

import (
	"github.com/ethereum/go-ethereum/ethdb"
)

const (
	TX_PER_BLOCK = 1000
	TX_MAX_PAY = 1000
	DIFFICULTY = 4
	TX_DIFFICULTY = 3
	VOTE_DIFFICULTY = 5
	NODES_PER_SHARD = 4
	SHARD_NUM = 1
	TOTAL_NUMBER_OF_NODES = NODES_PER_SHARD * SHARD_NUM
	BLOCKS_NUMBER = 40
)

type BlockChain struct {
	db     ethdb.Database // Low level persistent database to store final content in
}

// NewBlockChain returns a fully initialised block chain using information
// available in the database. It initialises the default Ethereum validator and
// Processor.
func NewBlockChain(db ethdb.Database) (*BlockChain, error) {
	bc := &BlockChain{
		db:  db,
	}
	
	if err := bc.loadLastState(); err != nil {
		return nil, err
	}
	// Take ownership of this particular state
	go bc.update()

	return bc, nil
}