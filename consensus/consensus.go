package consensus

import (
	"benzene/core"
	"benzene/p2p"
)

// Consensus is the main struct with all states and data related to consensus process.
type Consensus struct {
	ShardID uint32   // Shard Id which this node belongs to
	host    p2p.Host // The p2p host used to send/receive p2p messages

	// The blockchain this consensus is working on
	Blockchain *core.BlockChain
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
