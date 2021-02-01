package rawdb

import (
	"benzene/params"
	"encoding/binary"
	"encoding/json"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/log"
)

// ReadChainConfig retrieves the consensus settings based on the given genesis hash.
func ReadChainConfig(db ethdb.KeyValueReader, hash common.Hash) *params.ChainConfig {
	data, _ := db.Get(configKey(hash))
	if len(data) == 0 {
		return nil
	}
	var config params.ChainConfig
	if err := json.Unmarshal(data, &config); err != nil {
		log.Error("Invalid chain config JSON", "hash", hash, "err", err)
		return nil
	}
	return &config
}

// WriteChainConfig writes the chain config settings to the database.
func WriteChainConfig(db ethdb.KeyValueWriter, hash common.Hash, cfg *params.ChainConfig) {
	if cfg == nil {
		return
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		log.Crit("Failed to JSON encode chain config", "err", err)
	}
	if err := db.Put(configKey(hash), data); err != nil {
		log.Crit("Failed to store chain config", "err", err)
	}
}

// WriteShardID writes the shard id to the database
func WriteShardID(db ethdb.KeyValueWriter, shardID uint64) {
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], shardID)
	if err := db.Put(shardIDKey, buf[:]); err != nil {
		log.Crit("Failed to store shard id of the blockchain", "err", err)
	}
}

// ReadShardID writes the shard id to the database
func ReadShardID(db ethdb.KeyValueReader) *uint64 {
	data, _ := db.Get(shardIDKey)
	if len(data) == 0 {
		return nil
	}
	if len(data) != 8 {
		return nil
	}
	shardID := binary.BigEndian.Uint64(data)
	return &shardID
}
