package node


// GroupID is a multicast group ID.
//
// It is a binary string,
// conducive to layering and scoped generation using cryptographic hash.
//
// Applications define their own group ID, without central allocation.
// A cryptographically secure random string of enough length – 32 bytes for
// example – may be used.
type GroupID string

func (id GroupID) String() string {
	return string(id)
}

// Const of group ID
const (
	GroupIDShardPrefix       GroupID = "%s/0.0.1/node/shard/%s"
	GroupIDShardClientPrefix GroupID = "%s/0.0.1/client/shard/%s"
)

// ShardID defines the ID of a shard
type ShardID uint32
