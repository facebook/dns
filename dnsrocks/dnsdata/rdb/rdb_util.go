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

package rdb

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"log"
	"sort"
)

var (
	// ErrNXVal is a non-existing value error
	ErrNXVal = errors.New("attempt to delete non-existing value")
	// ErrNXKey is a non-existing key error
	ErrNXKey = errors.New("attempt to delete non-existing key")
)

type keyValues struct {
	key    []byte
	values [][]byte
}

type kvList []keyValues

// Sort sorts by key
func (kv *kvList) Sort() {
	sort.Slice(
		*kv,
		func(i, j int) bool {
			return bytes.Compare((*kv)[i].key, (*kv)[j].key) < 0
		},
	)
}

// appendValues will append 'newVals' values to a multi-value 'data', and return the result;
// used for add operations. It is basically a form of serialization.
// It does so by prefixing each value with 32-bit length and concatenating the result.
// The motivation behind choosing 32 bits for the legth is following:
// RFC-1035 RDLENGTH (uint16) + header requires more than 16 bytes; rounding
// this up to uint32. Potentially that wastes about 1 byte, so there is a room
// for trading off between space and computation.
func appendValues(data []byte, newVals [][]byte) []byte {
	var b [4]byte

	for _, v := range newVals {
		vlen := len(v)
		binary.LittleEndian.PutUint32(b[:], uint32(vlen))
		data = append(data, b[:]...)
		data = append(data, v...)
	}
	return data
}

// delValue will delete the 'value' from a multi-value 'data', returns error
// if the data is malformed of the value does not exist
func delValue(data []byte, value []byte) ([]byte, error) {
	var i, chunkLen int
	l := len(data)
	for i < l {
		if i+4 > l {
			return nil, io.ErrUnexpectedEOF
		}
		chunkLen = int(binary.LittleEndian.Uint32(data[i:])) + 4
		if i+chunkLen > l {
			return nil, io.ErrUnexpectedEOF
		}
		v := data[i+4 : i+chunkLen]
		if len(v) == len(value) && bytes.Equal(v, value) {
			if i+chunkLen < len(data) {
				copy(data[i:], data[i+chunkLen:])
			}
			return data[:l-chunkLen], nil
		}
		i += chunkLen
	}
	log.Printf("Cannot remove non-existent value %v (%s)", value, string(value))
	return nil, ErrNXVal // value not found in the multi-value data
}

func copyBytes(b []byte) []byte {
	cc := make([]byte, len(b))
	copy(cc, b)
	return cc
}
