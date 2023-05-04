// Copyright 2018 The go-ethereum Authors
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

//go:build !js
// +build !js

// Package badger implements the key-value database layer based on LevelDB.
package badger

import (
	"runtime"
	"sync"

	"github.com/dgraph-io/badger/v4"
	"github.com/ethereum/go-ethereum/ethdb"
)

// Database is a persistent key-value store. Apart from basic data storage
// functionality it also supports batch writes and iterating over the keyspace in
// binary-alphabetical order.
type Database struct {
	fn string     // filename for reporting
	db *badger.DB // LevelDB instance

	quitLock sync.Mutex      // Mutex protecting the quit channel access
	quitChan chan chan error // Quit channel to stop the metrics collection before closing the database
}

// New returns a wrapped LevelDB object. The namespace is the prefix that the
// metrics reporting should use for surfacing internal stats.
func New(file string, cache int, handles int, namespace string, readonly bool) (*Database, error) { // Set default options
	// Open the db and recover any potential corruptions
	option := badger.DefaultOptions(file)
	option.Logger = nil
	db, err := badger.Open(option)
	if err != nil {
		return nil, err
	}
	// Assemble the wrapper with all the registered metrics
	bdb := &Database{
		fn:       file,
		db:       db,
		quitChan: make(chan chan error),
	}
	return bdb, nil
}

// Close stops the metrics collection, flushes any pending data to disk and closes
// all io accesses to the underlying key-value store.
func (db *Database) Close() error {
	db.quitLock.Lock()
	defer db.quitLock.Unlock()
	return db.db.Close()
}

// Has retrieves if a key is present in the key-value store.
func (db *Database) Has(key []byte) (bool, error) {
	txn := db.db.NewTransaction(false)
	defer txn.Discard()
	_, err := txn.Get(key[:])
	if err == badger.ErrKeyNotFound {
		return false, nil
	} else if err != nil {
		return false, err
	}
	return true, nil
}

// Get retrieves the given key if it's present in the key-value store.
func (db *Database) Get(key []byte) ([]byte, error) {
	txn := db.db.NewTransaction(false)
	defer txn.Discard()
	item, err := txn.Get(key)
	if err != nil {
		return nil, err
	}
	return item.ValueCopy(nil)
}

// Put inserts the given value into the key-value store.
func (db *Database) Put(key []byte, value []byte) error {
	txn := db.db.NewTransaction(true)
	if err := txn.Set(key, value); err != nil {
		return err
	}
	return txn.Commit()
}

// Delete removes the key from the key-value store.
func (db *Database) Delete(key []byte) error {
	txn := db.db.NewTransaction(true)
	if err := txn.Delete(key); err != nil {
		return err
	}
	return txn.Commit()
}

// NewBatch creates a write-only key-value store that buffers changes to its host
// database until a final write is called.
func (db *Database) NewBatch() ethdb.Batch {
	return &batch{
		db:  db.db,
		b:   db.db.NewWriteBatch(),
		ops: make([]*operation, 0),
	}
}

// NewBatchWithSize creates a write-only database batch with pre-allocated buffer.
func (db *Database) NewBatchWithSize(size int) ethdb.Batch {
	return &batch{
		db:  db.db,
		b:   db.db.NewWriteBatch(),
		ops: make([]*operation, 0),
	}
}

type badgerIterator struct {
	iter  *badger.Iterator
	moved bool
}

// NewIterator creates a binary-alphabetical iterator over a subset
// of database content with a particular key prefix, starting at a particular
// initial key (or after, if it does not exist).
func (db *Database) NewIterator(prefix []byte, start []byte) ethdb.Iterator {
	txn := db.db.NewTransaction(false)
	iter := txn.NewIterator(badger.IteratorOptions{
		Prefix: prefix,
	})
	lowerKey := make([]byte, len(prefix)+len(start))
	copy(lowerKey[:len(prefix)], prefix)
	copy(lowerKey[len(prefix):], start)
	iter.Seek(lowerKey)
	return &badgerIterator{iter: iter, moved: true}
}

func (iter *badgerIterator) Next() bool {
	if iter.moved {
		iter.moved = false
		return iter.iter.Valid()
	}
	if !iter.iter.Valid() {
		return false
	}
	iter.iter.Next()
	return iter.iter.Valid()
}

func (iter *badgerIterator) Error() error {
	return nil
}

func (iter *badgerIterator) Key() []byte {
	return iter.iter.Item().Key()
}

func (iter *badgerIterator) Value() []byte {
	value, err := iter.iter.Item().ValueCopy(nil)
	if err != nil {
		return nil
	}
	return value
}

func (iter *badgerIterator) Release() {
	iter.iter.Close()
}

type snapshot struct {
	snap *badger.Txn
}

// NewSnapshot creates a database snapshot based on the current state.
// The created snapshot will not be affected by all following mutations
// happened on the database.
// Note don't forget to release the snapshot once it's used up, otherwise
// the stale data will never be cleaned up by the underlying compactor.
func (db *Database) NewSnapshot() (ethdb.Snapshot, error) {
	return &snapshot{snap: db.db.NewTransaction(false)}, nil
}

// Stat returns a particular internal stat of the database.
func (db *Database) Stat(property string) (string, error) {
	return "", nil
}

// Compact flattens the underlying data store for the given key range. In essence,
// deleted and overwritten versions are discarded, and the data is rearranged to
// reduce the cost of operations needed to access them.
//
// A nil start is treated as a key before all keys in the data store; a nil limit
// is treated as a key after all keys in the data store. If both is nil then it
// will compact entire data store.
func (db *Database) Compact(start []byte, limit []byte) error {
	return db.db.Flatten(runtime.NumCPU())
}

// Path returns the path to the database directory.
func (db *Database) Path() string {
	return db.fn
}

type operationType int

const (
	putOperation operationType = iota
	deleteOepration
)

type operation struct {
	op    operationType
	key   []byte
	value []byte
}

// batch is a write-only leveldb batch that commits changes to its host database
// when Write is called. A batch cannot be used concurrently.
type batch struct {
	db   *badger.DB
	b    *badger.WriteBatch
	ops  []*operation
	size int
}

// Put inserts the given value into the batch for later committing.
func (b *batch) Put(key, value []byte) error {
	if err := b.b.Set(key, value); err != nil {
		return err
	}
	b.ops = append(b.ops, &operation{
		op:    putOperation,
		key:   key,
		value: value,
	})
	b.size += len(key) + len(value)
	return nil
}

// Delete inserts the a key removal into the batch for later committing.
func (b *batch) Delete(key []byte) error {
	if err := b.b.Delete(key); err != nil {
		return err
	}
	b.ops = append(b.ops, &operation{
		op:  deleteOepration,
		key: key,
	})
	b.size += len(key)
	return nil
}

// ValueSize retrieves the amount of data queued up for writing.
func (b *batch) ValueSize() int {
	return b.size
}

// Write flushes any accumulated data to disk.
func (b *batch) Write() error {
	return b.b.Flush()
}

// Reset resets the batch for reuse.
func (b *batch) Reset() {
	b.b.Cancel()
	b.b = b.db.NewWriteBatch()
	b.ops = make([]*operation, 0)
	b.size = 0
}

// Replay replays the batch contents.
func (b *batch) Replay(w ethdb.KeyValueWriter) error {
	for _, op := range b.ops {
		switch op.op {
		case putOperation:
			if err := w.Put(op.key, op.value); err != nil {
				return err
			}
		case deleteOepration:
			if err := w.Delete(op.key); err != nil {
				return err
			}
		}
	}
	return nil
}

// Has retrieves if a key is present in the snapshot backing by a key-value
// data store.
func (snap *snapshot) Has(key []byte) (bool, error) {
	_, err := snap.snap.Get(key)
	if err == badger.ErrKeyNotFound {
		return false, nil
	} else if err != nil {
		return false, err
	}
	return true, nil
}

// Get retrieves the given key if it's present in the snapshot backing by
// key-value data store.
func (snap *snapshot) Get(key []byte) ([]byte, error) {
	item, err := snap.snap.Get(key)
	if err != nil {
		return nil, err
	}
	return item.ValueCopy(nil)
}

// Release releases associated resources. Release should always succeed and can
// be called multiple times without causing error.
func (snap *snapshot) Release() {
	snap.snap.Discard()
}
