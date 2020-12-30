package consensus

import (
	msg_pb "benzene/api/proto/message"
	"benzene/core"
	"benzene/crypto/bls"
	"benzene/multibls"
	"benzene/p2p"
	"context"
)

// Consensus is the main struct with all states and data related to consensus process.
type Consensus struct {

	// The blockchain this consensus is working on
	Blockchain *core.BlockChain

	// Minimal number of peers in the shard
	// If the number of validators is less than minPeers, the consensus won't start
	MinPeers int

	// Shard Id which this node belongs to
	ShardID []uint64

	// The p2p host used to send/receive p2p messages
	host p2p.Host
}

// New create a new Consensus record
func New(
	host p2p.Host, shard []uint64, multiBLSPriKey multibls.PrivateKeys,
) (*Consensus, error) {
	consensus := Consensus{}
	consensus.host = host

	consensus.ShardID = shard

	return &consensus, nil
}

// HandleMessageUpdate will update the consensus state according to received message
func (consensus *Consensus) HandleConsensusMessage(ctx context.Context, msg *msg_pb.Message, senderKey *bls.SerializedPublicKey) error {
	return nil
}
