package types

import (
	"github.com/ethereum/go-ethereum/common"
	"math/big"
	"sync/atomic"

	"github.com/ethereum/go-ethereum/rlp"
)

// Transaction struct.
type Transaction struct {
	data txdata
	// caches
	hash atomic.Value
	size atomic.Value
	from atomic.Value
}

type txdata struct {
	AccountNonce uint64          `json:"nonce"      gencodec:"required"`

	Sender    [20]byte
	Recipient [20]byte

	ShardID   uint32
	ToShardID uint32

	Amount    float64
	Timestamp float64

	// 签名实现方法二选一：
	// 1. 使用go-ethereum的包
	// 2. 使用go自带的ecdsa包，但是没有SECP256k1曲线
	V *big.Int // 公钥
	R *big.Int // 签名的R值
	S *big.Int // 签名的S值

	// 在数据存储中，go的签名与python的签名有不同之处：
	// go的签名需要存R和S
	// python的签名已经将R和S整合到了一起

	// This is only used when marshaling to JSON.
	Hash *common.Hash `json:"hash" rlp:"-"`
}

// Nonce returns account nonce from Transaction.
func (tx *Transaction) Nonce() uint64 {
	return tx.data.AccountNonce
}

// Sender returns the sender address of the transaction.
func (tx *Transaction) Sender() [20]byte {
	from := tx.data.Sender
	return from
}

// Recipient returns the recipient address of the transaction.
func (tx *Transaction) Recipient() [20]byte {
	to := tx.data.Recipient
	return to
}


// Hash hashes the RLP encoding of tx.
// It uniquely identifies the transaction.
func (tx *Transaction) Hash() common.Hash {
	if hash := tx.hash.Load(); hash != nil {
		return hash.(common.Hash)
	}
	v := rlpHash(tx)
	tx.hash.Store(v)
	return v
}

// ShardID returns which shard id this transaction was signed for (if at all)
func (tx *Transaction) ShardID() uint32 {
	return tx.data.ShardID
}

// ToShardID returns the destination shard id this transaction is going to
func (tx *Transaction) ToShardID() uint32 {
	return tx.data.ToShardID
}

// Amount returns the payment of Transaction.
func (tx *Transaction) Amount() float64 {
	return tx.data.Amount
}

// Transactions is a Transaction slice type for basic sorting.
type Transactions []*Transaction

// Len returns the length of s.
func (s Transactions) Len() int { return len(s) }

// Swap swaps the i'th and the j'th element in s.
func (s Transactions) Swap(i, j int) { s[i], s[j] = s[j], s[i] }

// GetRlp implements Rlpable and returns the i'th element of s in rlp.
func (s Transactions) GetRlp(i int) []byte {
	enc, _ := rlp.EncodeToBytes(s[i])
	return enc
}
