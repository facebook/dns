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

// #cgo pkg-config: rocksdb
// #include "rocksdb/c.h"
// #include <stdlib.h> // for free()
import "C"

import (
	"errors"
	"unsafe"
)

// SSTFileWriter allows batch creation of SST files, https://github.com/facebook/rocksdb/wiki/Creating-and-Ingesting-SST-files
type SSTFileWriter struct {
	cFileWriter *C.rocksdb_sstfilewriter_t
	cEnvOptions *C.rocksdb_envoptions_t
	dbOptions   *Options
}

// CreateSSTFileWriter creates SSTFileWriter object using provided file path
func CreateSSTFileWriter(path string) (*SSTFileWriter, error) {
	options := NewOptions()
	options.EnableCreateIfMissing()

	sstPath := C.CString(path)
	defer C.free(unsafe.Pointer(sstPath))

	var envOptions *C.rocksdb_envoptions_t = C.rocksdb_envoptions_create()

	var fileWriter *C.rocksdb_sstfilewriter_t = C.rocksdb_sstfilewriter_create(envOptions, options.cOptions)
	var cError *C.char
	C.rocksdb_sstfilewriter_open(fileWriter, sstPath, &cError)

	if cError != nil {
		defer C.rocksdb_free(unsafe.Pointer(cError))
		return nil, errors.New(C.GoString(cError))
	}
	return &SSTFileWriter{
		cFileWriter: fileWriter,
		cEnvOptions: envOptions,
		dbOptions:   options,
	}, nil
}

// Put stores a binary key-value
func (w *SSTFileWriter) Put(key, value []byte) error {
	var cError *C.char
	cKeyPtr, cKeyLen := bytesToPtr(key)
	cValPtr, cValLen := bytesToPtr(value)
	C.rocksdb_sstfilewriter_put(
		w.cFileWriter,
		cKeyPtr, cKeyLen, cValPtr, cValLen,
		&cError,
	)
	if cError != nil {
		defer C.rocksdb_free(unsafe.Pointer(cError))
		return errors.New(C.GoString(cError))
	}
	return nil
}

// Finish finishes writing to SST, and returns error (if any)
func (w *SSTFileWriter) Finish() error {
	var cError *C.char
	C.rocksdb_sstfilewriter_finish(w.cFileWriter, &cError)
	if cError != nil {
		defer C.rocksdb_free(unsafe.Pointer(cError))
		return errors.New(C.GoString(cError))
	}
	return nil
}

// GetFileSize returns the size of SST file
func (w *SSTFileWriter) GetFileSize() uint64 {
	var size uint64
	C.rocksdb_sstfilewriter_file_size(w.cFileWriter, (*C.uint64_t)(unsafe.Pointer(&size)))
	return size
}

// CloseWriter frees memory structures associated with the object
func (w *SSTFileWriter) CloseWriter() {
	C.rocksdb_sstfilewriter_destroy(w.cFileWriter)
	w.dbOptions.FreeOptions()
	C.rocksdb_envoptions_destroy(w.cEnvOptions)
}
