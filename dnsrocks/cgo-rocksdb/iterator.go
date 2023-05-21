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

const unsigned char ITER_BOOL_CHAR_TRUE = 1;
*/
import "C"

import (
	"errors"
	"unsafe"
)

// Iterator is a box for RocksDB iterator
type Iterator struct {
	cIter *C.rocksdb_iterator_t
}

// CreateIterator creates an instance of Iterator
func (db *RocksDB) CreateIterator(readOptions *ReadOptions) *Iterator {
	return &Iterator{
		cIter: C.rocksdb_create_iterator(db.cDB, readOptions.cReadOptions),
	}
}

// IsValid checks iterator validity. For invalid iterators the caller should check
// the error by calling GetError().
func (i *Iterator) IsValid() bool {
	return C.rocksdb_iter_valid(i.cIter) == C.ITER_BOOL_CHAR_TRUE
}

// SeekToFirst positions iterator on the first key.
func (i *Iterator) SeekToFirst() {
	C.rocksdb_iter_seek_to_first(i.cIter)
}

// Seek positions iterator on this key. If this key does not exist - on the next key in order.
func (i *Iterator) Seek(key []byte) {
	cKeyPtr, cKeyLen := bytesToPtr(key)
	C.rocksdb_iter_seek(i.cIter, cKeyPtr, cKeyLen)
}

// SeekForPrev positions iterator on this key. If this key does not exist - on the previous key in order.
func (i *Iterator) SeekForPrev(key []byte) {
	cKeyPtr, cKeyLen := bytesToPtr(key)
	C.rocksdb_iter_seek_for_prev(i.cIter, cKeyPtr, cKeyLen)
}

// Next moves iterator on the next key
func (i *Iterator) Next() {
	C.rocksdb_iter_next(i.cIter)
}

// Prev moves iterator on the previous key
func (i *Iterator) Prev() {
	C.rocksdb_iter_prev(i.cIter)
}

// Key returns the key at the current position. To advance position use Next() and Prev(). To obtain the value use Value().
func (i *Iterator) Key() []byte {
	var klen C.size_t
	ckey := C.rocksdb_iter_key(i.cIter, &klen)
	return C.GoBytes(unsafe.Pointer(ckey), C.int(klen))
}

// Value returns the value at the current position. To obtain the key use Key().
func (i *Iterator) Value() []byte {
	var vlen C.size_t
	cval := C.rocksdb_iter_value(i.cIter, &vlen)
	return C.GoBytes(unsafe.Pointer(cval), C.int(vlen))
}

// GetError returns last error, or nil if there is none.
func (i *Iterator) GetError() error {
	var cError *C.char
	C.rocksdb_iter_get_error(i.cIter, &cError)
	if cError != nil {
		defer C.rocksdb_free(unsafe.Pointer(cError))
		return errors.New(C.GoString(cError))
	}
	return nil
}

// FreeIterator frees up memory for the iterator
func (i *Iterator) FreeIterator() {
	C.rocksdb_iter_destroy(i.cIter)
}
