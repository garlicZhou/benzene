package rawdb

import (
	"benzene/core/types"
	"benzene/internal/utils"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/rlp"
)

// ReadTxLookupEntry retrieves the positional metadata associated with a transaction
// hash to allow retrieving the transaction or receipt by hash.
func ReadTxLookupEntry(db DatabaseReader, hash common.Hash) (common.Hash, uint64, uint64) {
	data, _ := db.Get(txLookupKey(hash))
	if len(data) == 0 {
		return common.Hash{}, 0, 0
	}
	var entry TxLookupEntry
	if err := rlp.DecodeBytes(data, &entry); err != nil {
		utils.Logger().Error().Err(err).Str("hash", hash.Hex()).Msg("Invalid transaction lookup entry RLP")
		return common.Hash{}, 0, 0
	}
	return entry.BlockHash, entry.BlockIndex, entry.Index
}

// WriteBlockTxLookUpEntries writes all look up entries of block's transactions
func WriteBlockTxLookUpEntries(db DatabaseWriter, block *types.Block) error {
	for i, tx := range block.Transactions() {
		entry := TxLookupEntry{
			BlockHash:  block.Hash(),
			BlockIndex: block.NumberU64(),
			Index:      uint64(i),
		}
		val, err := rlp.EncodeToBytes(entry)
		if err != nil {
			return err
		}
		key := txLookupKey(tx.Hash())
		if err := db.Put(key, val); err != nil {
			return err
		}
	}
	return nil
}
