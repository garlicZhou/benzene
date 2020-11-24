package node

import (
	p2p_crypto "github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/peer"
	"sync"
)

// Global is the index of the global node configuration
const (
	Global    = 0
	MaxShards = 32 // maximum number of shards. It is also the maxium number of configs.
)

var peerID peer.ID // PeerID of the node

// ConfigType is the structure of all node related configuration variables
type ConfigType struct {
	ShardID   uint32 // ShardID of this node; TODO ek – revisit when resharding
	Port      string // Port of the node.
	IP        string // IP of the node.
	P2PPriKey p2p_crypto.PrivKey
	DBDir string // Database directory
}

// configs is a list of node configuration.
// It has at least one configuration.
// The first one is the default, global node configuration
var shardConfigs []ConfigType
var defaultConfig ConfigType
var onceForConfigs sync.Once

func ensureShardConfigs() {
	onceForConfigs.Do(func() {
		shardConfigs = make([]ConfigType, MaxShards)
		for i := range shardConfigs {
			shardConfigs[i].ShardID = uint32(i)
		}
	})
}

// GetShardConfig return the shard's ConfigType variable
func GetShardConfig(shardID uint32) *ConfigType {
	ensureShardConfigs()
	if int(shardID) >= cap(shardConfigs) {
		return nil
	}
	return &shardConfigs[shardID]
}

// GetDefaultConfig returns default config.
func GetDefaultConfig() *ConfigType {
	return &defaultConfig
}

// SetPeerID set the peer ID of the node
func SetPeerID(pid peer.ID) {
	peerID = pid
}

// GetPeerID returns the peer ID of the node
func GetPeerID() peer.ID {
	return peerID
}