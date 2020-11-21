package state

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
	"io"
	"math/big"
)

var emptyCodeHash = crypto.Keccak256(nil)

// Object represents an Ethereum account which is being modified.
//
// The usage pattern is as follows:
// First you need to obtain a state object.
// Account values can be accessed and modified through the object.
// Finally, call CommitTrie to write the modified storage trie into a database.
type Object struct {
	address  common.Address
	addrHash common.Hash // hash of ethereum address of the account
	data     Account
	db       *DB

	// DB error.
	// State objects are used by the consensus core and VM which are
	// unable to deal with database-level errors. Any error that occurs
	// during a database read is memoized here and will eventually be returned
	// by StateDB.Commit.
	dbErr error
}

// empty returns whether the account is considered empty.
func (s *Object) empty() bool {
	return s.data.Nonce == 0 && s.data.Balance.Sign() == 0
}

// Account is the Ethereum consensus representation of accounts.
// These objects are stored in the main account trie.
type Account struct {
	Nonce    uint64
	Balance  *big.Int
}

// newObject creates a state object.
func newObject(db *DB, address common.Address, data Account) *Object {
	if data.Balance == nil {
		data.Balance = new(big.Int)
	}
	return &Object{
		db:             db,
		address:        address,
		addrHash:       crypto.Keccak256Hash(address[:]),
		data:           data,
	}
}

// EncodeRLP implements rlp.Encoder.
func (s *Object) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, s.data)
}

// setError remembers the first non-nil error it is called with.
func (s *Object) setError(err error) {
	if s.dbErr == nil {
		s.dbErr = err
	}
}

// AddBalance removes amount from c's balance.
// It is used to add funds to the destination account of a transfer.
func (s *Object) AddBalance(amount *big.Int) {
	s.SetBalance(new(big.Int).Add(s.Balance(), amount))
}

// SubBalance removes amount from c's balance.
// It is used to remove funds from the origin account of a transfer.
func (s *Object) SubBalance(amount *big.Int) {
	if amount.Sign() == 0 {
		return
	}
	s.SetBalance(new(big.Int).Sub(s.Balance(), amount))
}

// SetBalance ...
func (s *Object) SetBalance(amount *big.Int) {
	s.setBalance(amount)
}

func (s *Object) setBalance(amount *big.Int) {
	s.data.Balance = amount
}

func (s *Object) deepCopy(db *DB) *Object {
	stateObject := newObject(db, s.address, s.data)
	return stateObject
}

//
// Attribute accessors
//

// Address returns the address of the contract/account
func (s *Object) Address() common.Address {
	return s.address
}

// SetNonce ...
func (s *Object) SetNonce(nonce uint64) {
	s.setNonce(nonce)
}

func (s *Object) setNonce(nonce uint64) {
	s.data.Nonce = nonce
}

// Balance ...
func (s *Object) Balance() *big.Int {
	return s.data.Balance
}

// Nonce ...
func (s *Object) Nonce() uint64 {
	return s.data.Nonce
}