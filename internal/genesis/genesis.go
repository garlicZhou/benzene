package genesis

// DeployAccount is the account used in genesis
type DeployAccount struct {
	Address      string // account address
	ShardID      uint32 // shardID of the account
}
