// Copyright 2014 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

// Package state provides a caching layer atop the Ethereum state trie.
package state

import (
	"fmt"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/metrics"
	"github.com/ethereum/go-ethereum/rlp"
	"math/big"
	"time"
)

// DB within the ethereum protocol are used to store anything
// within the merkle trie. StateDBs take care of caching and storing
// nested states. It's the general query interface to retrieve:
// * Contracts
// * Accounts
type DB struct {
	db   Database
	trie Trie

	// This map holds 'live' objects, which will get modified while processing a state transition.
	stateObjects        map[common.Address]*Object
	stateObjectsPending map[common.Address]struct{} // State objects finalized but not yet written to the trie

	// DB error.
	// State objects are used by the consensus core and VM which are
	// unable to deal with database-level errors. Any error that occurs
	// during a database read is memoized here and will eventually be returned
	// by DB.Commit.
	dbErr error

	// Measurements gathered during execution for debugging purposes
	AccountReads   time.Duration
	AccountHashes  time.Duration
	AccountUpdates time.Duration
	AccountCommits time.Duration
	StorageReads   time.Duration
	StorageHashes  time.Duration
	StorageUpdates time.Duration
	StorageCommits time.Duration
}

// New creates a new state from a given trie.
func New(root common.Hash, db Database) (*DB, error) {
	tr, err := db.OpenTrie(root)
	if err != nil {
		return nil, err
	}
	return &DB{
		db:                  db,
		trie:                tr,
		stateObjects:        make(map[common.Address]*Object),
		stateObjectsPending: make(map[common.Address]struct{}),
	}, nil
}

// setError remembers the first non-nil error it is called with.
func (db *DB) setError(err error) {
	if db.dbErr == nil {
		db.dbErr = err
	}
}

func (db *DB) Error() error {
	return db.dbErr
}

// Database retrieves the low level database supporting the lower level trie ops.
func (db *DB) Database() Database {
	return db.db
}

/*
 * SETTERS
 */

// AddBalance adds amount to the account associated with addr.
func (db *DB) AddBalance(addr common.Address, amount *big.Int) {
	stateObject := db.GetOrNewStateObject(addr)
	if stateObject != nil {
		stateObject.AddBalance(amount)
	}
}

// SubBalance subtracts amount from the account associated with addr.
func (db *DB) SubBalance(addr common.Address, amount *big.Int) {
	stateObject := db.GetOrNewStateObject(addr)
	if stateObject != nil {
		stateObject.SubBalance(amount)
	}
}

// SetBalance ...
func (db *DB) SetBalance(addr common.Address, amount *big.Int) {
	stateObject := db.GetOrNewStateObject(addr)
	if stateObject != nil {
		stateObject.SetBalance(amount)
	}
}

// SetNonce ...
func (db *DB) SetNonce(addr common.Address, nonce uint64) {
	stateObject := db.GetOrNewStateObject(addr)
	if stateObject != nil {
		stateObject.SetNonce(nonce)
	}
}

//
// Setting, updating & deleting state object methods.
//

// updateStateObject writes the given object to the trie.
func (db *DB) updateStateObject(obj *Object) {
	// Track the amount of time wasted on updating the account from the trie
	if metrics.EnabledExpensive {
		defer func(start time.Time) { db.AccountUpdates += time.Since(start) }(time.Now())
	}
	// Encode the account and update the account trie
	addr := obj.Address()

	data, err := rlp.EncodeToBytes(obj)
	if err != nil {
		panic(fmt.Errorf("can't encode object at %x: %v", addr[:], err))
	}
	db.setError(db.trie.TryUpdate(addr[:], data))
}

// deleteStateObject removes the given object from the state trie.
func (db *DB) deleteStateObject(obj *Object) {
	// Track the amount of time wasted on deleting the account from the trie
	if metrics.EnabledExpensive {
		defer func(start time.Time) { db.AccountUpdates += time.Since(start) }(time.Now())
	}
	// Delete the account from the trie
	addr := obj.Address()
	db.setError(db.trie.TryDelete(addr[:]))
}

// getStateObject retrieves a state object given by the address, returning nil if
// the object is not found or was deleted in this execution context. If you need
// to differentiate between non-existent/just-deleted, use getDeletedStateObject.
func (db *DB) getStateObject(addr common.Address) *Object {
	if obj := db.getDeletedStateObject(addr); obj != nil && !obj.deleted {
		return obj
	}
	return nil
}

// getDeletedStateObject is similar to getStateObject, but instead of returning
// nil for a deleted state object, it returns the actual object with the deleted
// flag set. This is needed by the state journal to revert to the correct s-
// destructed object instead of wiping all knowledge about the state object.
func (db *DB) getDeletedStateObject(addr common.Address) *Object {
	// Prefer live objects if any is available
	if obj := db.stateObjects[addr]; obj != nil {
		return obj
	}
	// Track the amount of time wasted on loading the object from the database
	if metrics.EnabledExpensive {
		defer func(start time.Time) { db.AccountReads += time.Since(start) }(time.Now())
	}
	// Load the object from the database
	enc, err := db.trie.TryGet(addr[:])
	if len(enc) == 0 {
		db.setError(err)
		return nil
	}
	var data Account
	if err := rlp.DecodeBytes(enc, &data); err != nil {
		log.Error("Failed to decode state object", "addr", addr, "err", err)
		return nil
	}
	// Insert into the live set
	obj := newObject(db, addr, data)
	db.setStateObject(obj)
	return obj
}

func (db *DB) setStateObject(object *Object) {
	db.stateObjects[object.Address()] = object
}

// GetOrNewStateObject retrieves a state object or create a new state object if nil.
func (db *DB) GetOrNewStateObject(addr common.Address) *Object {
	stateObject := db.getStateObject(addr)
	if stateObject == nil {
		stateObject, _ = db.createObject(addr)
	}
	return stateObject
}

// createObject creates a new state object. If there is an existing account with
// the given address, it is overwritten and returned as the second return value.
func (db *DB) createObject(addr common.Address) (newobj, prev *Object) {
	prev = db.getDeletedStateObject(addr) // Note, prev might have been deleted, we need that!

	newobj = newObject(db, addr, Account{})
	newobj.setNonce(0) // sets the object to dirty
	db.setStateObject(newobj)
	return newobj, prev
}

// Finalise finalises the state by removing the db destructed objects
// and clears the journal as well as the refunds.
func (db *DB) Finalise(deleteEmptyObjects bool) {
	// Invalidate journal because reverting across transactions is not allowed.
	db.clearJournalAndRefund()
}

// IntermediateRoot computes the current root hash of the state trie.
// It is called in between transactions to get the root hash that
// goes into transaction receipts.
func (db *DB) IntermediateRoot(deleteEmptyObjects bool) common.Hash {
	// Finalise all the dirty storage states and write them into the tries
	db.Finalise(deleteEmptyObjects)

	for addr := range db.stateObjectsPending {
		obj := db.stateObjects[addr]
		if obj.deleted {
			db.deleteStateObject(obj)
		} else {
			obj.updateRoot(db.db)
			db.updateStateObject(obj)
		}
	}
	if len(db.stateObjectsPending) > 0 {
		db.stateObjectsPending = make(map[common.Address]struct{})
	}
	// Track the amount of time wasted on hashing the account trie
	if metrics.EnabledExpensive {
		defer func(start time.Time) { db.AccountHashes += time.Since(start) }(time.Now())
	}
	return db.trie.Hash()
}

func (db *DB) clearJournalAndRefund() {
}

// Commit writes the state to the underlying in-memory trie database.
func (db *DB) Commit() (root common.Hash, err error) {
	if db.dbErr != nil {
		return common.Hash{}, fmt.Errorf("commit aborted due to earlier error: %v", db.dbErr)
	}
	// Write the account trie changes, measuing the amount of wasted time
	if metrics.EnabledExpensive {
		defer func(start time.Time) { db.AccountCommits += time.Since(start) }(time.Now())
	}
	return db.trie.Commit(func(path []byte, leaf []byte, parent common.Hash) error {
		var account Account
		if err := rlp.DecodeBytes(leaf, &account); err != nil {
			return nil
		}
		return nil
	})
}
