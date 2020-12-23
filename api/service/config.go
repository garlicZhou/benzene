package service

import (
	"benzene/internal/configs"
)

// NodeConfig defines a structure of node configuration
// that can be used in services.
// This is to pass node configuration to services and prevent
// cyclic imports
type NodeConfig struct {
	// The three groupID design, please refer to https://github.com/harmony-one/harmony/blob/master/node/node.md#libp2p-integration
	ShardGroupID []configs.GroupID                      // the group ID of the shard
	Client       []configs.GroupID                      // the client group ID of the shard
	ShardID      []uint64                               // shardID of this node
	Actions      map[configs.GroupID]configs.ActionType // actions on the groups
}

// GroupIDShards is a map of ShardGroupID ID
// key is the shard ID
// value is the corresponding group ID
var (
	GroupIDShards       = map[uint64]configs.GroupID{}
	GroupIDShardClients = map[uint64]configs.GroupID{}
)

func init() {
	// init beacon chain group IDs
	GroupIDShards[0] = configs.NewGroupIDByShardID(0)
	GroupIDShardClients[0] = configs.NewClientGroupIDByShardID(0)

	for sid := uint64(1); sid <= configs.MaxShards; sid++ {
		GroupIDShards[sid] = configs.NewGroupIDByShardID(sid)
		GroupIDShardClients[sid] = configs.NewClientGroupIDByShardID(sid)
	}
}
