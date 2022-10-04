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
#include <stdlib.h> // for free()
*/
import "C"

import (
	"reflect"
	"unsafe"
)

type charsSlice []*C.char
type sizeTSlice []C.size_t

func (s charsSlice) c() **C.char {
	sH := (*reflect.SliceHeader)(unsafe.Pointer(&s))
	return (**C.char)(unsafe.Pointer(sH.Data))
}

func (s sizeTSlice) c() *C.size_t {
	sH := (*reflect.SliceHeader)(unsafe.Pointer(&s))
	return (*C.size_t)(unsafe.Pointer(sH.Data))
}

type ptrList struct {
	cChars   charsSlice
	cLengths sizeTSlice
}

func bytesToPtr(bytes []byte) (*C.char, C.size_t) {
	var result *C.char
	length := len(bytes)
	if length > 0 {
		result = (*C.char)(unsafe.Pointer(&bytes[0]))
	}
	return result, C.size_t(length)
}

func strToPtr(s string) (*C.char, C.size_t) {
	var result *C.char
	length := len(s)
	if length > 0 {
		result = C.CString(s)
	}
	return result, C.size_t(length)
}

// bytesListToPtrList converts slice of byte slices into an array of char arrays and sizes,
// it explicitly allocates new arrays and copies data; the caller is responsible for
// calling freePtrList
func bytesListToPtrList(bytes [][]byte) *ptrList {
	bytesLen := len(bytes)
	// NOTE: this array's content is allocated with malloc, and needs to be freed outside
	cBytes := make(charsSlice, bytesLen)
	cLengths := make(sizeTSlice, bytesLen)
	for i, token := range bytes {
		length := len(token)
		if length > 0 {
			// this needs to be C.freed
			cBytes[i] = (*C.char)(C.CBytes(token))
		} else {
			cBytes[i] = nil
		}
		cLengths[i] = C.size_t(length)
	}
	return &ptrList{
		cChars:   cBytes,
		cLengths: cLengths,
	}
}

// freePtrList frees up the memory allocated for ptrList
func (l *ptrList) freePtrList() {
	for _, chars := range l.cChars {
		C.free(unsafe.Pointer(chars))
	}
}
