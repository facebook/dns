//go:build ignore

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

import (
	"encoding/binary"
	"fmt"
	"testing"
)

const (
	keyFmt = "bench_key%06d"
	valFmt = "bench_val%06d"
)

func seqFillStr(upperLimit uint64) error {
	var bKey [8]byte
	sValue := "5678"
	for i := uint64(0); i < upperLimit; i++ {
		binary.LittleEndian.PutUint64(bKey[:], i)
		if err := db.PutStr(writeOptions, string(bKey[:]), sValue); err != nil {
			return fmt.Errorf("error writing string: %s", err.Error())
		}
	}
	return nil
}

func seqFillBytes(upperLimit uint64) error {
	var bKey [8]byte
	bValue := []byte{5, 6, 7, 8}
	for i := uint64(0); i < upperLimit; i++ {
		binary.LittleEndian.PutUint64(bKey[:], i)
		if err := db.Put(writeOptions, bKey[:], bValue); err != nil {
			return fmt.Errorf("error writing bytes: %v - %s", bKey, err.Error())
		}
	}
	return nil
}

// BenchmarkPutStr benchmarks PutStr() function
func BenchmarkPutStr(b *testing.B) {
	if err := seqFillStr(uint64(b.N)); err != nil {
		b.Error(err)
	}
}

// BenchmarkGetStr benchmarks GetStr() function
func BenchmarkGetStr(b *testing.B) {
	seqFillStr(uint64(b.N))

	b.ResetTimer()
	var bKey [8]byte
	upperLimit := uint64(b.N)
	for i := uint64(0); i < upperLimit; i++ {
		binary.LittleEndian.PutUint64(bKey[:], i)
		if _, err := db.GetStr(readOptions, string(bKey[:])); err != nil {
			b.Errorf("Error reading string: %s", err.Error())
		}
	}
}

// BenchmarkPut benchmarks Put() function
func BenchmarkPut(b *testing.B) {
	if err := seqFillBytes(uint64(b.N)); err != nil {
		b.Error(err)
	}
}

// BenchmarkGet benchmarks Get() function for existing values
func BenchmarkGet(b *testing.B) {
	if err := seqFillBytes(uint64(b.N)); err != nil {
		b.Error(err)
	}

	b.ResetTimer()

	var bKey [8]byte
	upperLimit := uint64(b.N)
	for i := uint64(0); i < upperLimit; i++ {
		binary.LittleEndian.PutUint64(bKey[:], i)
		if _, err := db.Get(readOptions, bKey[:]); err != nil {
			b.Errorf("Error reading string: %s", err.Error())
		}
	}
}

// BenchmarkGetNonExistent benchmarks Get() function for non-existing values
func BenchmarkGetNonExistent(b *testing.B) {
	bKey := [16]byte{
		'd', 'o', 'e', 's',
		'n', 'o', 't',
		'e', 'x', 'i', 's', 't',
		'0', '0', '0', '0',
	}
	upperLimit := uint64(b.N)
	for i := uint64(0); i < upperLimit; i++ {
		binary.LittleEndian.PutUint64(bKey[8:], i)
		if _, err := db.Get(readOptions, bKey[:]); err != nil {
			b.Errorf("Error reading string: %s", err.Error())
		}
	}
}

// BenchmarkBatchPut benchmarks ExecuteBatch() on Put() operations
func BenchmarkBatchPut(b *testing.B) {
	testSizes := []int{10, 100, 1000, 10000}
	for _, size := range testSizes {
		b.Run(fmt.Sprintf("BenchmarkBatchPut%d", size), func(b *testing.B) {
			batch := db.NewBatch()
			defer batch.Destroy()
			for i := 0; i < size; i++ {
				batch.Put([]byte(fmt.Sprintf(keyFmt, i)), []byte(fmt.Sprintf(valFmt, i)))
			}

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if err := db.ExecuteBatch(batch, writeOptions); err != nil {
					b.Errorf("Error executing write batch: %s", err.Error())
				}
			}
		})
	}
}

// BenchmarkGetMulti benchmarks GetMulti
func BenchmarkGetMulti(b *testing.B) {
	testSizes := []int{10, 100, 1000, 10000, 100000, 1000000}
	for _, size := range testSizes {
		b.Run(fmt.Sprintf("BenchmarkGetMulti%d", size), func(b *testing.B) {
			// write values and prepare key batch
			keys := make([][]byte, size)
			batch := db.NewBatch()
			defer batch.Destroy()
			for i := 0; i < size; i++ {
				key, val := []byte(fmt.Sprintf(keyFmt, i)), []byte(fmt.Sprintf(valFmt, i))
				batch.Put(key, val)
				keys[i] = key
			}
			if err := db.ExecuteBatch(batch, writeOptions); err != nil {
				b.Errorf("Error executing write batch: %s", err.Error())
			}

			b.ResetTimer()

			// read values
			for i := 0; i < b.N; i++ {
				res, errs := db.GetMulti(readOptions, keys)
				for _, err := range errs {
					if err != nil {
						b.Error(err)
					}
				}
				if len(res) != size {
					b.Errorf("GetMulti mismatch: expected %d, got %d", size, len(res))
				}
			}
		})
	}
}

// BenchmarkIteratorGet benchmarks Iterator
func BenchmarkIteratorGet(b *testing.B) {
	testSizes := []int{10, 100, 1000, 100000, 1000000}
	for _, size := range testSizes {
		b.Run(fmt.Sprintf("BenchmarkIteratorGet%d", size), func(b *testing.B) {
			keys, vals := make([][]byte, size), make([][]byte, size)
			batch := db.NewBatch()
			defer batch.Destroy()
			for i := 0; i < size; i++ {
				bKey, bValue := []byte(fmt.Sprintf(keyFmt, i)), []byte(fmt.Sprintf(valFmt, i))
				batch.Put(bKey, bValue)
				keys[i] = bKey
				vals[i] = bValue
			}

			if err := db.ExecuteBatch(batch, writeOptions); err != nil {
				b.Errorf("Error executing write batch: %s", err.Error())
			}

			b.ResetTimer()

			iter := db.CreateIterator(readOptions)
			defer iter.FreeIterator()
			for i := 0; i < b.N; i++ {
				iter.Seek(keys[0])
				for j := 0; j < size; j++ {
					if !iter.IsValid() {
						b.Errorf("Invalid iterator on key %s, error %s", keys[j], iter.GetError())
					}
					rVal := iter.Value()
					if len(rVal) != len(vals[j]) {
						b.Errorf("Value mismatch: expected %s, got %s", vals[j], rVal)
					}
					iter.Next()
				}
				if err := iter.GetError(); err != nil {
					b.Error(err)
				}
			}
		})
	}
}
