/*
Copyright (c) Meta Platforms, Inc. and affiliates.
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
    http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package rocksdb

/*
// @fb-only: #include "rocksdb/src/include/rocksdb/c.h"
#cgo pkg-config: "rocksdb"
#include "rocksdb/c.h" // @oss-only

#include <stdlib.h> // for free()
*/
import "C"

import (
	"errors"
	"unsafe"
)

// RocksDB is a connection instance
type RocksDB struct {
	cDB           *C.rocksdb_t
	name          string
	secondary     bool
	secondaryPath string
	options       *Options
}

// OpenDatabase opens the database directory with provided options,
// returns RocksDB instance or error.
// Parameters:
//   - name: path to db (directory)
//   - readOnly: open in read-only mode
//   - readOnlyErrorIfLogExists: for read-only mode will throw error if logfile exists
func OpenDatabase(name string, readOnly, readOnlyErrorIfLogExist bool, options *Options) (*RocksDB, error) {
	var (
		cError *C.char
	)
	dbName := C.CString(name)
	defer C.free(unsafe.Pointer(dbName))
	var db *C.rocksdb_t
	if readOnly {
		db = C.rocksdb_open_for_read_only(
			options.cOptions, dbName, BoolToChar(readOnlyErrorIfLogExist), &cError,
		)
	} else {
		db = C.rocksdb_open(options.cOptions, dbName, &cError)
	}
	if cError != nil {
		defer C.rocksdb_free(unsafe.Pointer(cError))
		return nil, errors.New(C.GoString(cError))
	}
	return &RocksDB{
		cDB:     db,
		name:    name,
		options: options,
	}, nil
}

// OpenSecondary opens the database in "secondary" read-only mode,
// returns RocksDB instance or error.
// The secondary_path argument points to a directory where the secondary
// instance stores its info log.
func OpenSecondary(name, secondaryPath string, options *Options) (*RocksDB, error) {
	// secondary instance has to keep all SST files open
	options.SetMaxOpenFiles(-1)

	var cError *C.char

	dbName := C.CString(name)
	defer C.free(unsafe.Pointer(dbName))
	cSecondaryPath := C.CString(secondaryPath)
	defer C.free(unsafe.Pointer(cSecondaryPath))
	var db *C.rocksdb_t = C.rocksdb_open_as_secondary(
		options.cOptions, dbName, cSecondaryPath, &cError,
	)
	if cError != nil {
		defer C.rocksdb_free(unsafe.Pointer(cError))
		return nil, errors.New(C.GoString(cError))
	}
	return &RocksDB{
		cDB:           db,
		name:          name,
		options:       options,
		secondary:     true,
		secondaryPath: secondaryPath,
	}, nil
}

// CatchWithPrimary makes the best effort to catch up with all the latest updates
// from the primary database.
func (db *RocksDB) CatchWithPrimary() error {
	if !db.secondary {
		return errors.New("This database is the primary database")
	}

	var cError *C.char
	C.rocksdb_try_catch_up_with_primary(db.cDB, &cError)
	if cError != nil {
		defer C.rocksdb_free(unsafe.Pointer(cError))
		return errors.New(C.GoString(cError))
	}

	return nil
}

// Put stores a binary key-value
func (db *RocksDB) Put(writeOptions *WriteOptions, key, value []byte) error {
	var cError *C.char
	cKeyPtr, cKeyLen := bytesToPtr(key)
	cValPtr, cValLen := bytesToPtr(value)
	C.rocksdb_put(
		db.cDB, writeOptions.cWriteOptions,
		cKeyPtr, cKeyLen, cValPtr, cValLen,
		&cError,
	)
	if cError != nil {
		defer C.rocksdb_free(unsafe.Pointer(cError))
		return errors.New(C.GoString(cError))
	}
	return nil
}

// Get retrieves the binary value associated with the byte key
func (db *RocksDB) Get(readOptions *ReadOptions, key []byte) ([]byte, error) {
	var (
		cError    *C.char
		cValueLen C.size_t
	)
	cKeyPtr, cKeyLen := bytesToPtr(key)
	cValue := C.rocksdb_get(
		db.cDB, readOptions.cReadOptions,
		cKeyPtr, cKeyLen,
		&cValueLen, &cError,
	)
	if cError != nil {
		err := errors.New(C.GoString(cError))
		C.rocksdb_free(unsafe.Pointer(cError))
		return nil, err
	}
	if cValue == nil {
		return nil, nil
	}
	result := C.GoBytes(unsafe.Pointer(cValue), C.int(cValueLen))
	C.rocksdb_free(unsafe.Pointer(cValue))
	return result, nil
}

// GetMulti retrieves multiple binary values associated with multiple byte keys;
// returns two arrays of corresponding size - one with results, and another
// with errors (or nil's)
func (db *RocksDB) GetMulti(readOptions *ReadOptions, keys [][]byte) ([][]byte, []error) {
	keysCount := len(keys)

	scValues := make(charsSlice, keysCount)
	stValueLengths := make(sizeTSlice, keysCount)
	scErrors := make(charsSlice, keysCount)

	keyList := bytesListToPtrList(keys)
	C.rocksdb_multi_get(
		db.cDB, readOptions.cReadOptions,
		C.size_t(keysCount), keyList.cChars.c(), keyList.cLengths.c(),
		scValues.c(), stValueLengths.c(), scErrors.c(),
	)
	keyList.freePtrList()

	// process errors
	errorList := make([]error, keysCount)
	for i := 0; i < len(scErrors); i++ {
		if scErrors[i] != nil {
			// NOTE: C.GoString() is a copying operation; there is a space for trading
			// safety for speed here
			errorList[i] = errors.New(C.GoString(scErrors[i]))
			C.rocksdb_free(unsafe.Pointer(scErrors[i]))
		}
	}

	// process values
	valueList := make([][]byte, keysCount)
	for i := 0; i < len(scValues); i++ {
		// NOTE: ditto about C.GoBytes(), if it is ever a bottleneck, then
		// a wrapper on top of pure C arrays (with slice-like interface) is
		// the way to go
		valueList[i] = C.GoBytes(unsafe.Pointer(scValues[i]), C.int(stValueLengths[i]))
		if scValues[i] != nil {
			C.rocksdb_free(unsafe.Pointer(scValues[i]))
		}
	}

	return valueList, errorList
}

// Delete removes the binary key from the database
func (db *RocksDB) Delete(writeOptions *WriteOptions, key []byte) error {
	var cError *C.char
	cKeyPtr, cKeyLen := bytesToPtr(key)
	C.rocksdb_delete(db.cDB, writeOptions.cWriteOptions, cKeyPtr, cKeyLen, &cError)
	if cError != nil {
		defer C.rocksdb_free(unsafe.Pointer(cError))
		return errors.New(C.GoString(cError))
	}
	return nil
}

// PutStr stores a string key-value
func (db *RocksDB) PutStr(writeOptions *WriteOptions, key, value string) error {
	var cError *C.char
	cKeyPtr, cKeyLen := strToPtr(key)
	defer C.free(unsafe.Pointer(cKeyPtr))
	cValPtr, cValLen := strToPtr(value)
	defer C.free(unsafe.Pointer(cValPtr))
	C.rocksdb_put(
		db.cDB, writeOptions.cWriteOptions,
		cKeyPtr, cKeyLen, cValPtr, cValLen,
		&cError,
	)
	if cError != nil {
		defer C.rocksdb_free(unsafe.Pointer(cError))
		return errors.New(C.GoString(cError))
	}
	return nil
}

// GetStr retrieves the string value associated with the string key
func (db *RocksDB) GetStr(readOptions *ReadOptions, key string) (string, error) {
	var (
		cError    *C.char
		cValueLen C.size_t
	)
	cKeyPtr, cKeyLen := strToPtr(key)
	defer C.free(unsafe.Pointer(cKeyPtr))
	cValue := C.rocksdb_get(
		db.cDB, readOptions.cReadOptions,
		cKeyPtr, cKeyLen,
		&cValueLen, &cError,
	)
	if cError != nil {
		err := errors.New(C.GoString(cError))
		C.rocksdb_free(unsafe.Pointer(cError))
		return "", err
	}
	if cValue == nil {
		return "", nil
	}
	result := C.GoStringN(cValue, C.int(cValueLen))
	C.rocksdb_free(unsafe.Pointer(cValue))
	return result, nil
}

// DeleteStr removes the string key from the database
func (db *RocksDB) DeleteStr(writeOptions *WriteOptions, key string) error {
	var cError *C.char
	cKeyPtr, cKeyLen := strToPtr(key)
	defer C.free(unsafe.Pointer(cKeyPtr))
	C.rocksdb_delete(db.cDB, writeOptions.cWriteOptions, cKeyPtr, cKeyLen, &cError)
	if cError != nil {
		defer C.rocksdb_free(unsafe.Pointer(cError))
		return errors.New(C.GoString(cError))
	}
	return nil
}

// ExecuteBatch executes operations from the batch with provided options
func (db *RocksDB) ExecuteBatch(batch *Batch, writeOptions *WriteOptions) error {
	var cError *C.char
	C.rocksdb_write(
		db.cDB, writeOptions.cWriteOptions,
		batch.cBatch, &cError,
	)
	if cError != nil {
		defer C.rocksdb_free(unsafe.Pointer(cError))
		return errors.New(C.GoString(cError))
	}
	return nil
}

// IngestSSTFiles ingests provided files into the database.
// More details: https://github.com/facebook/rocksdb/wiki/Creating-and-Ingesting-SST-files
// useHardlinks allows to use 'link' syscall while moving files instead of copying them,
// set it to 'true' if the filesystem supports it.
func (db *RocksDB) IngestSSTFiles(fileNames []string, useHardlinks bool) error {
	cFileNames := make([]*C.char, len(fileNames))
	for i, fileName := range fileNames {
		cFileNames[i] = C.CString(fileName)
	}
	ingestOptions := C.rocksdb_ingestexternalfileoptions_create()
	C.rocksdb_ingestexternalfileoptions_set_move_files(ingestOptions, BoolToChar(useHardlinks))

	defer func() {
		for _, cFileName := range cFileNames {
			C.free(unsafe.Pointer(cFileName))
		}
		C.rocksdb_ingestexternalfileoptions_destroy(ingestOptions)
	}()

	var cError *C.char
	C.rocksdb_ingest_external_file(db.cDB, &cFileNames[0], C.size_t(len(cFileNames)), ingestOptions, &cError)
	if cError != nil {
		defer C.rocksdb_free(unsafe.Pointer(cError))
		return errors.New(C.GoString(cError))
	}
	return nil
}

// Flush flushes in-memory WAL to disk. It's important to know that Flush can trigger background operations like compaction,
// which may be interrupted if Close is called immediately after flush. Such operations can be waited upon by
// checking DB properties like 'rocksdb.num-running-compactions' via GetProperty call.
func (db *RocksDB) Flush() error {
	cFlushOptions := C.rocksdb_flushoptions_create()
	defer func() {
		C.rocksdb_flushoptions_destroy(cFlushOptions)
	}()

	var cError *C.char
	C.rocksdb_flush(db.cDB, cFlushOptions, &cError)
	if cError != nil {
		defer C.rocksdb_free(unsafe.Pointer(cError))
		return errors.New(C.GoString(cError))
	}
	return nil
}

// CompactRangeAll runs compaction on all DB
func (db *RocksDB) CompactRangeAll() {
	C.rocksdb_compact_range(
		db.cDB,
		nil,
		0,
		nil,
		0,
	)
}

// WaitForCompact waits for all currently running compactions to finish, optionally closing the DB afterwards
func (db *RocksDB) WaitForCompact(options *WaitForCompactOptions) error {
	var cError *C.char
	C.rocksdb_wait_for_compact(db.cDB, options.cOptions, &cError)
	if cError != nil {
		defer C.rocksdb_free(unsafe.Pointer(cError))
		return errors.New(C.GoString(cError))
	}
	return nil
}

// GetProperty returns value of db property
func (db *RocksDB) GetProperty(prop string) string {
	cprop := C.CString(prop)
	defer C.free(unsafe.Pointer(cprop))
	cValue := C.rocksdb_property_value(db.cDB, cprop)
	defer C.rocksdb_free(unsafe.Pointer(cValue))
	return C.GoString(cValue)
}

// GetOptions returns options
func (db *RocksDB) GetOptions() *Options {
	return db.options
}

// CloseDatabase frees the memory and closes the connection
func (db *RocksDB) CloseDatabase() {
	db.options.FreeOptions()
	C.rocksdb_close(db.cDB)
}

// Batch is a wrapper for WriteBatch. It allows to implement "transactions"
// in RocksDB sense, by grouping a number of operations together.
// According to https://github.com/facebook/rocksdb/wiki/Transactions :
// "Note that RocksDB provides Atomicity by default when writing multiple keys
// via WriteBatch. Transactions provide a way to guarantee that a batch of
// writes will only be written if there are no conflicts. Similar to a
// WriteBatch, no other threads can see the changes in a transaction until
// it has been written (committed)."
type Batch struct {
	cBatch *C.rocksdb_writebatch_t
}

// NewBatch creates a Batch
func (db *RocksDB) NewBatch() *Batch {
	return &Batch{
		cBatch: C.rocksdb_writebatch_create(),
	}
}

// Clear clears batch content
func (batch *Batch) Clear() {
	C.rocksdb_writebatch_clear(batch.cBatch)
}

// GetCount returns the number of actions in the batch
func (batch *Batch) GetCount() int {
	return int(C.rocksdb_writebatch_count(batch.cBatch))
}

// Put schedules storing a binary key-value pair in the batch
func (batch *Batch) Put(key, value []byte) {
	cKeyPtr, cKeyLen := bytesToPtr(key)
	cValPtr, cValLen := bytesToPtr(value)
	C.rocksdb_writebatch_put(
		batch.cBatch,
		cKeyPtr, cKeyLen, cValPtr, cValLen,
	)
}

// PutVector stores key-value pair composed of multiple "chunks".
// If you just want to store more than one key-value pair, this is NOT
// what you are looking for.
func (batch *Batch) PutVector(key, value [][]byte) {
	keyList := bytesListToPtrList(key)
	valList := bytesListToPtrList(value)

	C.rocksdb_writebatch_putv(
		batch.cBatch,
		C.int(len(key)), keyList.cChars.c(), keyList.cLengths.c(),
		C.int(len(value)), valList.cChars.c(), valList.cLengths.c(),
	)
	keyList.freePtrList()
	valList.freePtrList()
}

// Delete schedules a deletion of the operation in the batch
func (batch *Batch) Delete(key []byte) {
	cKeyPtr, cKeyLen := bytesToPtr(key)
	C.rocksdb_writebatch_delete(batch.cBatch, cKeyPtr, cKeyLen)
}

// Destroy destroys a Batch object
func (batch *Batch) Destroy() {
	C.rocksdb_writebatch_destroy(batch.cBatch)
}
