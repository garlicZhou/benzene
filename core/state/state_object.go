package state

import (
	"bytes"
	"fmt"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/metrics"
	"github.com/ethereum/go-ethereum/rlp"
	"io"
	"math/big"
	"time"
)

var emptyCodeHash = crypto.Keccak256(nil)

// Code ...
type Code []byte

func (c Code) String() string {
	return string(c) //strings.Join(Disassemble(c), " ")
}

// Storage ...
type Storage map[common.Hash]common.Hash

func (s Storage) String() (str string) {
	for key, value := range s {
		str += fmt.Sprintf("%X : %X\n", key, value)
	}

	return
}

// Copy ...
func (s Storage) Copy() Storage {
	cpy := make(Storage)
	for key, value := range s {
		cpy[key] = value
	}

	return cpy
}

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

	// Write caches.
	trie Trie // storage trie, which becomes non-nil on first access
	code Code // contract bytecode, which gets set when code is loaded

	originStorage  Storage // Storage cache of original entries to dedup rewrites, reset for every transaction
	pendingStorage Storage // Storage entries that need to be flushed to disk, at the end of an entire block
	dirtyStorage   Storage // Storage entries that have been modified in the current transaction execution
	fakeStorage    Storage // Fake storage which constructed by caller for debugging purpose.

	// Cache flags.
	// When an object is marked suicided it will be delete from the trie
	// during the "update" phase of the state transition.
	dirtyCode bool // true if the code was updated
	suicided  bool
	deleted   bool
}

// empty returns whether the account is considered empty.
func (s *Object) empty() bool {
	return s.data.Nonce == 0 && s.data.Balance.Sign() == 0 && bytes.Equal(s.data.CodeHash, emptyCodeHash)
}

// Account is the Ethereum consensus representation of accounts.
// These objects are stored in the main account trie.
type Account struct {
	Nonce    uint64
	Balance  *big.Int
	Root     common.Hash // merkle root of the storage trie
	CodeHash []byte
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
		originStorage:  make(Storage),
		pendingStorage: make(Storage),
		dirtyStorage:   make(Storage),
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

func (s *Object) getTrie(db Database) Trie {
	if s.trie == nil {
		var err error
		s.trie, err = db.OpenStorageTrie(s.addrHash, s.data.Root)
		if err != nil {
			s.trie, _ = db.OpenStorageTrie(s.addrHash, common.Hash{})
			s.setError(fmt.Errorf("can't create storage trie: %v", err))
		}
	}
	return s.trie
}

// finalise moves all dirty storage slots into the pending area to be hashed or
// committed later. It is invoked at the end of every transaction.
func (s *Object) finalise() {

}

// updateTrie writes cached storage modifications into the object's storage trie.
func (s *Object) updateTrie(db Database) Trie {
	// Make sure all dirty slots are finalized into the pending storage area
	s.finalise()

	// Track the amount of time wasted on updating the storge trie
	if metrics.EnabledExpensive {
		defer func(start time.Time) { s.db.StorageUpdates += time.Since(start) }(time.Now())
	}
	// Insert all the pending updates into the trie
	tr := s.getTrie(db)
	return tr
}

// UpdateRoot sets the trie root to the current root hash of
func (s *Object) updateRoot(db Database) {
	s.updateTrie(db)

	// Track the amount of time wasted on hashing the storge trie
	if metrics.EnabledExpensive {
		defer func(start time.Time) { s.db.StorageHashes += time.Since(start) }(time.Now())
	}
	s.data.Root = s.trie.Hash()
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
