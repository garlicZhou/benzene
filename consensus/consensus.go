package consensus

import (
	"benzene/core"
	"benzene/p2p"
)

// Consensus is the main struct with all states and data related to consensus process.
type Consensus struct {

	// The blockchain this consensus is working on
	Blockchain *core.BlockChain

	// Shard Id which this node belongs to
	ShardID uint32

	// The p2p host used to send/receive p2p messages
	host p2p.Host
}

// New create a new Consensus record
func New(
	host p2p.Host, shard uint32,
) (*Consensus, error) {
	consensus := Consensus{}
	consensus.host = host

	consensus.ShardID = shard

	return &consensus, nil
}