package node

import (
	"benzene/api/proto"
	"benzene/core/types"
	"bytes"
	"github.com/ethereum/go-ethereum/rlp"
	"log"
)

// MessageType is to indicate the specific type of message under Node category
type MessageType byte

// Constant of the top level Message Type exchanged among nodes
const (
	Transaction MessageType = iota
	Block
)

// TransactionMessageType represents the types of messages used for Node/Transaction
type TransactionMessageType int

// Constant of transaction message subtype
const (
	Send TransactionMessageType = iota
)

// BlockMessageType represents the type of messages used for Node/Block
type BlockMessageType int

// Block sync message subtype
const (
	Sync BlockMessageType = iota
)

var (
	// B suffix means Byte
	nodeB  = byte(proto.Node)
	blockB = byte(Block)
	txnB   = byte(Transaction)
	sendB  = byte(Send)
	syncB  = byte(Sync)

	// H suffix means header
	transactionListH = []byte{nodeB, txnB, sendB}
	syncH            = []byte{nodeB, blockB, syncB}
)

// ConstructTransactionListMessageAccount constructs serialized transactions in account model
func ConstructTransactionListMessageAccount(transactions types.Transactions) []byte {
	byteBuffer := bytes.NewBuffer(transactionListH)
	txs, err := rlp.EncodeToBytes(transactions)
	if err != nil {
		log.Fatal(err)
		return []byte{} // TODO(RJ): better handle of the error
	}
	byteBuffer.Write(txs)
	return byteBuffer.Bytes()
}

// ConstructBlocksSyncMessage constructs blocks sync message to send blocks to other nodes
func ConstructBlocksSyncMessage(blocks []*types.Block) []byte {
	byteBuffer := bytes.NewBuffer(syncH)
	blocksData, _ := rlp.EncodeToBytes(blocks)
	byteBuffer.Write(blocksData)
	return byteBuffer.Bytes()
}
