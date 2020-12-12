package node

import (
	"fmt"
	"strconv"
)

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

func getNetworkPrefix(shardID ShardID) (netPre string) {
	return "benzene"
}

// NewGroupIDByShardID returns a new groupID for a shard
func NewGroupIDByShardID(shardID ShardID) GroupID {
	return GroupID(fmt.Sprintf(GroupIDShardPrefix.String(), getNetworkPrefix(shardID), strconv.Itoa(int(shardID))))
}

// NewClientGroupIDByShardID returns a new groupID for a shard's client
func NewClientGroupIDByShardID(shardID ShardID) GroupID {
	return GroupID(fmt.Sprintf(GroupIDShardClientPrefix.String(), getNetworkPrefix(shardID), strconv.Itoa(int(shardID))))
}

// ActionType lists action on group
type ActionType uint

// Const of different Action type
const (
	ActionStart ActionType = iota
	ActionPause
	ActionResume
	ActionStop
	ActionUnknown
)

func (a ActionType) String() string {
	switch a {
	case ActionStart:
		return "ActionStart"
	case ActionPause:
		return "ActionPause"
	case ActionResume:
		return "ActionResume"
	case ActionStop:
		return "ActionStop"
	}
	return "ActionUnknown"
}

// GroupAction specify action on corresponding group
type GroupAction struct {
	Name   GroupID
	Action ActionType
}

func (g GroupAction) String() string {
	return fmt.Sprintf("%s/%s", g.Name, g.Action)
}