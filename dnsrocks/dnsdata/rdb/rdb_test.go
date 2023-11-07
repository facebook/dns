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
	"fmt"
	"io"
	"os"
	"sync"
	"testing"

	rocksdb "github.com/facebook/dns/dnsrocks/cgo-rocksdb"

	"github.com/stretchr/testify/require"
)

type mockedDB struct {
	get      func(key []byte) ([]byte, error)
	getMulti func(readOptions *rocksdb.ReadOptions, keys [][]byte) ([][]byte, []error)
	put      func(key, value []byte) error
	delete   func(key []byte) error
}

func (mock *mockedDB) Put(_ *rocksdb.WriteOptions, key, value []byte) error {
	return mock.put(key, value)
}

func (mock *mockedDB) Get(_ *rocksdb.ReadOptions, key []byte) ([]byte, error) {
	return mock.get(key)
}

func (mock *mockedDB) Delete(_ *rocksdb.WriteOptions, key []byte) error {
	return mock.delete(key)
}

func (mock *mockedDB) NewBatch() *rocksdb.Batch {
	return &rocksdb.Batch{}
}

func (mock *mockedDB) GetMulti(readOptions *rocksdb.ReadOptions, keys [][]byte) ([][]byte, []error) {
	return mock.getMulti(readOptions, keys)
}

func (mock *mockedDB) ExecuteBatch(_ *rocksdb.Batch, _ *rocksdb.WriteOptions) error {
	return nil
}
func (mock *mockedDB) CatchWithPrimary() error {
	return nil
}

func (mock *mockedDB) Flush() error {
	return nil
}

func (mock *mockedDB) CompactRangeAll() {}

func (mock *mockedDB) IngestSSTFiles(_ []string, _ bool) error {
	return nil
}

func (mock *mockedDB) CloseDatabase() {}

func (mock *mockedDB) CreateIterator(*rocksdb.ReadOptions) *rocksdb.Iterator {
	return nil
}

func (mock *mockedDB) GetOptions() *rocksdb.Options {
	return nil
}

func (mock *mockedDB) GetProperty(string) string {
	return ""
}

func (mock *mockedDB) WaitForCompact(*rocksdb.WaitForCompactOptions) error {
	return nil
}

func TestRDBAddErrorGettingValue(t *testing.T) {
	// check that returns error
	errorMsg := "I CAN'T GET NO VALUE"
	rdb := &RDB{
		db: &mockedDB{
			get: func(key []byte) ([]byte, error) {
				return nil, errors.New(errorMsg)
			},
			put: func(key, value []byte) error {
				return nil
			},
		},
		writeMutex: &sync.Mutex{},
	}
	err := rdb.Add([]byte{}, []byte{})
	if err != nil {
		require.Equal(t, err.Error(), errorMsg)
	}
}

func TestRDBAddToNewKey(t *testing.T) {
	// check creation of a new value for non-existing key
	newKey := []byte{0, 1, 255, 3, 4}
	newValue := []byte{1, 2, 255, 3, 0}
	rdb := &RDB{
		db: &mockedDB{
			get: func(key []byte) ([]byte, error) {
				return nil, nil
			},
			put: func(key, value []byte) error {
				require.Equal(t, key, newKey)
				require.Equal(t, value, []byte{5, 0, 0, 0, 1, 2, 255, 3, 0}) // length(newValue) in unit32 + value itself
				return nil
			},
		},
		writeMutex: &sync.Mutex{},
	}
	require.Nil(t, rdb.Add(newKey, newValue))
}

func TestRDBAddErrorAddToExistingKey(t *testing.T) {
	// check addition of a new value to existing key
	oldValue := []byte{5, 0, 0, 0, 7, 17, 32, 0, 15}
	newKey := []byte{9, 3, 255, 3, 4, 5}
	newValue := []byte{8, 2, 255, 3, 9, 8, 9}
	rdb := &RDB{
		db: &mockedDB{
			get: func(key []byte) ([]byte, error) {
				return oldValue, nil
			},
			put: func(key, value []byte) error {
				require.Equal(t, key, newKey)
				require.Equal(
					t,
					value,
					[]byte{
						// oldValue
						5, 0, 0, 0, 7, 17, 32, 0, 15,
						// length(newValue) in unit32 + value itself
						7, 0, 0, 0, 8, 2, 255, 3, 9, 8, 9,
					},
				)
				return nil
			},
		},
		writeMutex: &sync.Mutex{},
	}
	require.Nil(t, rdb.Add(newKey, newValue))
}

func TestRDBDelErrorGettingKey(t *testing.T) {
	// check that returns error if cannot get key
	errorMsg := "I CAN'T GET NO VALUE"
	rdb := &RDB{
		db: &mockedDB{
			get: func(key []byte) ([]byte, error) {
				return nil, errors.New(errorMsg)
			},
		},
		writeMutex: &sync.Mutex{},
	}
	err := rdb.Del([]byte{}, []byte{})
	if err != nil {
		require.Equal(t, err.Error(), errorMsg)
	}
}

func TestRDBDelErrorMissingKey(t *testing.T) {
	// check that returns error if key does not exist
	rdb := &RDB{
		db: &mockedDB{
			get: func(key []byte) ([]byte, error) {
				return nil, nil
			},
		},
		writeMutex: &sync.Mutex{},
	}
	err := rdb.Del([]byte{}, []byte{})
	require.Equal(t, err, ErrNXKey)
}

func TestRDBDelErrorNonExistingValue(t *testing.T) {
	// check that deleting non-existing value will cause error
	simpleKey := []byte{5, 6, 7, 8}
	simpleVal := []byte{91, 92, 93}
	simpleValStored := []byte{
		3, 0, 0, 0, 1, 2, 3,
		8, 0, 0, 0, 4, 5, 6, 7, 8, 9, 10, 11,
	}
	deleteCalled := false
	rdb := &RDB{
		db: &mockedDB{
			get: func(key []byte) ([]byte, error) {
				require.Equal(t, key, simpleKey)
				return simpleValStored, nil
			},
			put: func(key, value []byte) error {
				t.Error("Should not be called")
				return nil
			},
			delete: func(key []byte) error {
				deleteCalled = true
				return nil
			},
		},
		writeMutex: &sync.Mutex{},
	}
	err := rdb.Del(simpleKey, simpleVal)
	require.Equal(t, err, ErrNXVal)
	require.Equal(t, deleteCalled, false)
}

func TestRDBDelErrorTruncatedValue1(t *testing.T) {
	// check that deleting non-existing value will cause error
	simpleKey := []byte{5, 6, 7, 8}
	simpleVal := []byte{91, 92, 93}
	simpleValStored := []byte{
		3, 0, 0, 0, 1, 2, 3,
		8, 0, 0, 0, 4, 5, 6, 7, 8, 9, 10,
	}
	deleteCalled := false
	rdb := &RDB{
		db: &mockedDB{
			get: func(key []byte) ([]byte, error) {
				require.Equal(t, key, simpleKey)
				return simpleValStored, nil
			},
			put: func(key, value []byte) error {
				t.Error("Should not be called")
				return nil
			},
			delete: func(key []byte) error {
				deleteCalled = true
				return nil
			},
		},
		writeMutex: &sync.Mutex{},
	}
	err := rdb.Del(simpleKey, simpleVal)
	require.Equal(t, err, io.ErrUnexpectedEOF)
	require.Equal(t, deleteCalled, false)
}

func TestRDBDelErrorTruncatedValue2(t *testing.T) {
	// check that deleting non-existing value will cause error
	simpleKey := []byte{5, 6, 7, 8}
	simpleVal := []byte{91, 92, 93}
	simpleValStored := []byte{
		3, 0, 0, 0, 1, 2, 3,
		8, 0, 0,
	}
	deleteCalled := false
	rdb := &RDB{
		db: &mockedDB{
			get: func(key []byte) ([]byte, error) {
				require.Equal(t, key, simpleKey)
				return simpleValStored, nil
			},
			put: func(key, value []byte) error {
				t.Error("Should not be called")
				return nil
			},
			delete: func(key []byte) error {
				deleteCalled = true
				return nil
			},
		},
		writeMutex: &sync.Mutex{},
	}
	err := rdb.Del(simpleKey, simpleVal)
	if err != nil {
		require.Equal(t, err.Error(), "unexpected EOF")
	}
	require.Equal(t, deleteCalled, false)
}

func TestRDBDelOnlyValue(t *testing.T) {
	// check that deleting the only value will delete the key as well
	simpleKey := []byte{5, 6, 7, 8}
	simpleVal := []byte{1, 2, 3}
	simpleValStored := []byte{3, 0, 0, 0, 1, 2, 3}
	deleteCalled := false
	rdb := &RDB{
		db: &mockedDB{
			get: func(key []byte) ([]byte, error) {
				require.Equal(t, key, simpleKey)
				return simpleValStored, nil
			},
			put: func(key, value []byte) error {
				t.Error("Should not be called")
				return nil
			},
			delete: func(key []byte) error {
				deleteCalled = true
				require.Equal(t, key, simpleKey)
				return nil
			},
		},
		writeMutex: &sync.Mutex{},
	}
	err := rdb.Del(simpleKey, simpleVal)
	require.Nil(t, err)
	require.Equal(t, deleteCalled, true)
}

func TestRDBDelWithOffset(t *testing.T) {
	// check that values are deleted correctly
	simpleKey := []byte{5, 6, 7, 8}
	simpleValBefore := []byte{
		3, 0, 0, 0, 1, 2, 3,
		8, 0, 0, 0, 4, 5, 6, 7, 8, 9, 10, 11,
		2, 0, 0, 0, 12, 13,
	}
	type testCase struct {
		deletedValue []byte
		after        []byte
	}
	testCases := []testCase{
		{
			// deleting in the beginning
			deletedValue: []byte{1, 2, 3},
			after: []byte{
				8, 0, 0, 0, 4, 5, 6, 7, 8, 9, 10, 11,
				2, 0, 0, 0, 12, 13,
			},
		},
		{
			// deleting in the middle
			deletedValue: []byte{4, 5, 6, 7, 8, 9, 10, 11},
			after: []byte{
				3, 0, 0, 0, 1, 2, 3,
				2, 0, 0, 0, 12, 13,
			},
		},
		{
			// deleting in the end
			deletedValue: []byte{12, 13},
			after: []byte{
				3, 0, 0, 0, 1, 2, 3,
				8, 0, 0, 0, 4, 5, 6, 7, 8, 9, 10, 11,
			},
		},
	}
	for _, test := range testCases {
		rdb := &RDB{
			db: &mockedDB{
				get: func(key []byte) ([]byte, error) {
					require.Equal(t, simpleKey, key)
					valBefore := make([]byte, len(simpleValBefore))
					copy(valBefore, simpleValBefore)
					return valBefore, nil
				},
				put: func(key, value []byte) error {
					require.Equal(t, simpleKey, key)
					require.Equal(t, test.after, value)
					return nil
				},
				delete: func(key []byte) error {
					t.Error("Should not be called")
					return nil
				},
			},
			writeMutex: &sync.Mutex{},
		}
		require.Nil(t, rdb.Del(simpleKey, test.deletedValue))
	}
}

func TestRDBBatchGetAffectedKeys(t *testing.T) {
	type testCase struct {
		added   kvList
		deleted kvList
		output  [][]byte
	}
	testCases := []testCase{
		{
			// both are empty
			added:   kvList{},
			deleted: kvList{},
			output:  [][]byte{},
		},
		{
			// no deleted
			added: kvList{
				keyValues{
					key: []byte{0, 1, 3},
				},
				keyValues{
					key: []byte{0, 47},
				},
				keyValues{
					key: []byte{0, 52},
				},
				keyValues{
					key: []byte{0, 52},
				},
				keyValues{
					key: []byte{0, 54},
				},
			},
			deleted: kvList{},
			output:  [][]byte{{0, 1, 3}, {0, 47}, {0, 52}, {0, 54}},
		},
		{
			// no added
			added: kvList{},
			deleted: kvList{
				keyValues{
					key: []byte{0, 1, 3},
				},
				keyValues{
					key: []byte{0, 47},
				},
				keyValues{
					key: []byte{0, 47},
				},
				keyValues{
					key: []byte{0, 52},
				},
				keyValues{
					key: []byte{0, 54},
				},
			},
			output: [][]byte{{0, 1, 3}, {0, 47}, {0, 52}, {0, 54}},
		},
		{
			added: kvList{
				keyValues{
					key: []byte{0, 1, 2},
				},
				keyValues{
					key: []byte{0, 1, 4},
				},
				keyValues{
					key: []byte{0, 47},
				},
				keyValues{
					key: []byte{0, 47},
				},
				keyValues{
					key: []byte{0, 52},
				},
				keyValues{
					key: []byte{0, 54},
				},
			},
			deleted: kvList{
				keyValues{
					key: []byte{0, 1, 2},
				},
				keyValues{
					key: []byte{0, 1, 3},
				},
				keyValues{
					key: []byte{0, 1, 3},
				},
				keyValues{
					key: []byte{0, 47},
				},
				keyValues{
					key: []byte{0, 47},
				},
				keyValues{
					key: []byte{0, 52},
				},
				keyValues{
					key: []byte{0, 54},
				},
				keyValues{
					key: []byte{0, 55},
				},
			},
			output: [][]byte{{0, 1, 2}, {0, 1, 3}, {0, 1, 4}, {0, 47}, {0, 52}, {0, 54}, {0, 55}},
		},
	}
	for nt, tc := range testCases {
		t.Run(fmt.Sprintf("%d", nt), func(t *testing.T) {
			testBatch := &Batch{
				addedPairs:   tc.added,
				deletedPairs: tc.deleted,
			}
			require.Equal(t, tc.output, testBatch.getAffectedKeys())
		})
	}
}

func TestRDBContext(t *testing.T) {
	simpleKey := []byte{5, 6, 7, 8}
	type testCase struct {
		data           []byte
		unpackedValues [][]byte
		expectedError  error
	}
	testCases := []testCase{
		{
			// one packed value
			data: []byte{
				3, 0, 0, 0, 1, 2, 3,
			},
			unpackedValues: [][]byte{
				{1, 2, 3},
			},
			expectedError: nil,
		},
		{
			// three packed values
			data: []byte{
				3, 0, 0, 0, 1, 2, 3,
				8, 0, 0, 0, 4, 5, 6, 7, 8, 9, 10, 11,
				2, 0, 0, 0, 12, 13,
			},
			unpackedValues: [][]byte{
				{1, 2, 3},
				{4, 5, 6, 7, 8, 9, 10, 11},
				{12, 13},
			},
			expectedError: nil,
		},
		{
			// two values, last with bad length
			data: []byte{
				2, 0, 0, 0, 1, 2,
				2, 0, 0, 0, 1,
			},
			unpackedValues: [][]byte{
				{1, 2},
			},
			expectedError: io.ErrUnexpectedEOF,
		},
		{
			// incomplete length
			data: []byte{
				2, 0, 0,
			},
			unpackedValues: [][]byte{},
			expectedError:  io.ErrUnexpectedEOF,
		},
		{
			// missing key
			data:           nil,
			unpackedValues: [][]byte{},
			expectedError:  nil,
		},
	}
	for _, test := range testCases {
		rdb := &RDB{
			db: &mockedDB{
				get: func(key []byte) ([]byte, error) {
					require.Equal(t, simpleKey, key)
					return test.data, nil
				},
				put: func(key, value []byte) error {
					t.Error("Should not be called")
					return nil
				},
				delete: func(key []byte) error {
					t.Error("Should not be called")
					return nil
				},
			},
			writeMutex: &sync.Mutex{},
		}
		context := NewContext()
		require.NotNil(t, context)
		i := 0
		err := rdb.ForEach(
			simpleKey,
			func(value []byte) error {
				require.Equal(t, value, test.unpackedValues[i])
				i++
				return nil
			},
			context)

		require.Equal(t, err, test.expectedError)
		require.Equal(t, len(test.unpackedValues), i)
	}
}

func TestBatchGetAffectedKeys(t *testing.T) {
	type testCase struct {
		added        kvList
		deleted      kvList
		expectedKeys [][]byte
	}
	testCases := []testCase{
		{
			added:        kvList{},
			deleted:      kvList{},
			expectedKeys: [][]byte{},
		},
		{
			added: kvList{
				{
					key: []byte{1},
				},
				{
					key: []byte{2},
				},
			},
			deleted: kvList{},
			expectedKeys: [][]byte{
				{1},
				{2},
			},
		},
		{
			added: kvList{},
			deleted: kvList{
				{
					key: []byte{3},
				},
				{
					key: []byte{4},
				},
			},
			expectedKeys: [][]byte{
				{3},
				{4},
			},
		},
		{
			added: kvList{
				{
					key: []byte{1},
				},
				{
					key: []byte{2},
				},
			},
			deleted: kvList{
				{
					key: []byte{3},
				},
				{
					key: []byte{4},
				},
				{
					key: []byte{5},
				},
			},
			expectedKeys: [][]byte{
				{1},
				{2},
				{3},
				{4},
				{5},
			},
		},
	}
	rdb := &RDB{}
	for _, test := range testCases {
		batch := rdb.CreateBatch()
		batch.addedPairs = test.added
		batch.deletedPairs = test.deleted
		keys := batch.getAffectedKeys()
		require.Equal(t, keys, test.expectedKeys)
	}
}

func TestRDBFindFirst(t *testing.T) {
	type testCase struct {
		requestedKeys  [][]byte
		getMultiValues [][]byte
		getMultiErrors []error
		expectedValue  []byte
		expectedError  error
		expectedOffset int
	}
	someStrangeError := errors.New("you wouldn't believe what just happened")
	testCases := []testCase{
		{
			// key not found
			requestedKeys: [][]byte{
				[]byte("key_A"),
			},
			getMultiValues: [][]byte{
				nil,
			},
			getMultiErrors: []error{
				nil,
			},
			expectedValue:  nil,
			expectedError:  nil,
			expectedOffset: -1,
		},
		{
			// two keys requested, both exist, first returned
			requestedKeys: [][]byte{
				[]byte("key_B"),
				[]byte("key_C"),
			},
			getMultiValues: [][]byte{
				{8, 0, 0, 0, 4, 5, 6, 7, 8, 9, 10, 11},
				{3, 0, 0, 0, 1, 2, 3},
			},
			getMultiErrors: []error{
				nil, nil,
			},
			expectedValue:  []byte{4, 5, 6, 7, 8, 9, 10, 11},
			expectedError:  nil,
			expectedOffset: 0,
		},
		{
			// three keys requested, second returned
			requestedKeys: [][]byte{
				[]byte("key_A"),
				[]byte("key_B"),
				[]byte("key_C"),
			},
			getMultiValues: [][]byte{
				nil,
				{3, 0, 0, 0, 1, 2, 3},
				nil,
			},
			getMultiErrors: []error{
				nil, nil, nil,
			},
			expectedValue:  []byte{1, 2, 3},
			expectedError:  nil,
			expectedOffset: 1,
		},
		{
			// three keys requested, third one exists, second returns error
			requestedKeys: [][]byte{
				[]byte("key_A"),
				[]byte("key_B"),
				[]byte("key_C"),
			},
			getMultiValues: [][]byte{
				nil,
				nil,
				{2, 0, 0, 0, 1, 2},
			},
			getMultiErrors: []error{
				nil, someStrangeError, nil,
			},
			expectedValue:  nil,
			expectedError:  someStrangeError,
			expectedOffset: -1,
		},
		{
			// three keys requested, third one has malformed value header
			requestedKeys: [][]byte{
				[]byte("key_A"),
				[]byte("key_B"),
				[]byte("key_C"),
			},
			getMultiValues: [][]byte{
				nil,
				nil,
				{2, 0, 0},
			},
			getMultiErrors: []error{
				nil, nil, nil,
			},
			expectedValue:  nil,
			expectedError:  io.ErrUnexpectedEOF,
			expectedOffset: -1,
		},
		{
			// three keys requested, third one has mismatched value length
			requestedKeys: [][]byte{
				[]byte("key_C"),
				[]byte("key_A"),
				[]byte("key_B"),
			},
			getMultiValues: [][]byte{
				nil,
				nil,
				{2, 0, 0, 0, 5},
			},
			getMultiErrors: []error{
				nil, nil, nil,
			},
			expectedValue:  nil,
			expectedError:  io.ErrUnexpectedEOF,
			expectedOffset: -1,
		},
		{
			// three keys requested, third key one exists and has two values,
			// only the first value is returned
			requestedKeys: [][]byte{
				[]byte("key_D"),
				[]byte("key_E"),
				[]byte("key_F"),
			},
			getMultiValues: [][]byte{
				nil,
				nil,
				{
					5, 0, 0, 0, 9, 8, 7, 6, 5,
					3, 0, 0, 0, 4, 3, 2,
				},
			},
			getMultiErrors: []error{
				nil, nil, nil,
			},
			expectedValue:  []byte{9, 8, 7, 6, 5},
			expectedError:  nil,
			expectedOffset: 2,
		},
	}
	for nt, test := range testCases {
		t.Run(fmt.Sprintf("%d", nt), func(t *testing.T) {
			rdb := &RDB{
				db: &mockedDB{
					getMulti: func(readOptions *rocksdb.ReadOptions, keys [][]byte) ([][]byte, []error) {
						require.Equal(t, keys, test.requestedKeys)
						return test.getMultiValues, test.getMultiErrors
					},
					get: func(key []byte) ([]byte, error) {
						t.Error("Should not be called")
						return nil, nil
					},
					put: func(key, value []byte) error {
						t.Error("Should not be called")
						return nil
					},
					delete: func(key []byte) error {
						t.Error("Should not be called")
						return nil
					},
				},
			}
			val, offset, err := rdb.FindFirst(test.requestedKeys)
			require.Equal(t, val, test.expectedValue)
			require.Equal(t, offset, test.expectedOffset)
			require.Equal(t, err, test.expectedError)
		})
	}
}

func TestRDBBatchIntegrate(t *testing.T) {
	type testCase struct {
		added         kvList
		deleted       kvList
		uniqueKeys    [][]byte
		valuesBefore  [][]byte
		valuesAfter   [][]byte
		expectedError error
	}
	testCases := []testCase{
		{
			// testCase #1 - values should be merged and added
			valuesBefore: [][]byte{
				nil, nil,
			},
			added: kvList{
				keyValues{
					key:    []byte{0, 47},
					values: []byte{1},
				},
				keyValues{
					key:    []byte{0, 52},
					values: []byte{2, 3},
				},
				keyValues{
					key:    []byte{0, 52},
					values: []byte{4, 5, 6},
				},
			},
			deleted: kvList{},
			uniqueKeys: [][]byte{
				{0, 47}, {0, 52},
			},
			valuesAfter: [][]byte{
				{1, 0, 0, 0, 1},
				{2, 0, 0, 0, 2, 3, 3, 0, 0, 0, 4, 5, 6},
			},
			expectedError: nil,
		},
		{
			// testCase #2 - values should be deleted
			valuesBefore: [][]byte{
				{1, 0, 0, 0, 1},
				{2, 0, 0, 0, 2, 3, 3, 0, 0, 0, 4, 5, 6},
			},
			added: kvList{},
			deleted: kvList{
				keyValues{
					key:    []byte{0, 47},
					values: []byte{1},
				},
				keyValues{
					key:    []byte{0, 52},
					values: []byte{2, 3},
				},
			},
			uniqueKeys: [][]byte{
				{0, 47}, {0, 52},
			},
			valuesAfter: [][]byte{
				{},
				{3, 0, 0, 0, 4, 5, 6},
			},
			expectedError: nil,
		},
		{
			// testCase #3 - values should be added and deleted
			valuesBefore: [][]byte{
				{1, 0, 0, 0, 1},
				{2, 0, 0, 0, 2, 3, 3, 0, 0, 0, 4, 5, 6},
				{2, 0, 0, 0, 9, 3},
				{},
			},
			added: kvList{
				keyValues{
					key:    []byte{0, 46},
					values: []byte{3, 2, 1},
				},
				keyValues{
					key:    []byte{0, 47},
					values: []byte{9, 10},
				},
				keyValues{
					key:    []byte{0, 53},
					values: []byte{2, 3},
				},
				keyValues{
					key:    []byte{0, 53},
					values: []byte{4, 5, 6},
				},
			},
			deleted: kvList{
				keyValues{
					key:    []byte{0, 47},
					values: []byte{2, 3},
				},
				keyValues{
					key:    []byte{0, 52},
					values: []byte{9, 3},
				},
			},
			uniqueKeys: [][]byte{
				{0, 46}, {0, 47}, {0, 52}, {0, 53},
			},
			valuesAfter: [][]byte{
				{1, 0, 0, 0, 1, 3, 0, 0, 0, 3, 2, 1},     // key {0, 46}: {3, 2, 1} added
				{3, 0, 0, 0, 4, 5, 6, 2, 0, 0, 0, 9, 10}, // key {0, 47}: {9, 10} added and {2, 3} deleted
				{},                                       // key {0, 52}: {9, 3} deleted
				{2, 0, 0, 0, 2, 3, 3, 0, 0, 0, 4, 5, 6},  // key {0, 53}: {2, 3} and {4, 5, 6} added
			},
			expectedError: nil,
		},
		{
			// testCase #3 - internal error on value {0, 47}
			valuesBefore: [][]byte{
				{1, 0, 0, 0, 1},
				{2, 0, 0, 0, 2, 3, 3, 0, 0, 0, 4, 5, 6},
				{2, 0, 0, 0, 9, 3},
				{},
			},
			added: kvList{
				keyValues{
					key:    []byte{0, 46},
					values: []byte{3, 2, 1},
				},
				keyValues{
					key:    []byte{0, 47},
					values: []byte{9, 10},
				},
				keyValues{
					key:    []byte{0, 53},
					values: []byte{2, 3},
				},
				keyValues{
					key:    []byte{0, 53},
					values: []byte{4, 5, 6},
				},
			},
			deleted: kvList{
				keyValues{
					key:    []byte{0, 47},
					values: []byte{2, 3},
				},
				keyValues{
					key:    []byte{0, 52},
					values: []byte{9, 3},
				},
			},
			uniqueKeys: [][]byte{
				{0, 46}, {0, 52}, {0, 53},
			},
			valuesAfter:   nil,
			expectedError: fmt.Errorf("internal error: batch integration is incorrect 1 != 4 || 0 != 2"),
		},
	}
	for nt, tc := range testCases {
		t.Run(fmt.Sprintf("%d", nt), func(t *testing.T) {
			testBatch := &Batch{
				addedPairs:   tc.added,
				deletedPairs: tc.deleted,
			}
			err := testBatch.integrate(tc.uniqueKeys, &tc.valuesBefore)
			require.Equal(t, tc.expectedError, err)
			require.Equal(t, tc.valuesAfter, tc.valuesBefore)
		})
	}
}

func TestBatchAdd(t *testing.T) {
	type kv struct {
		key []byte
		val []byte
	}
	tc := kv{
		key: []byte{1, 2, 3},
		val: []byte{8},
	}
	targ := kv{
		key: []byte{1, 2, 3},
		val: []byte{8},
	}

	b := &Batch{}
	b.Add(tc.key, tc.val)

	// mutate to exercise ownership
	tc.key[0]++
	tc.val[0]++

	if len(b.addedPairs) != 1 {
		t.Fatalf("expected 1 added pair, got %d", len(b.addedPairs))
	}
	key := b.addedPairs[0].key
	if !bytes.Equal(key, targ.key) {
		t.Errorf("expected key %v, got %v", targ.key, key)
	}
	val := b.addedPairs[0].values
	if !bytes.Equal(val, targ.val) {
		t.Errorf("expected value %v, got %v", targ.val, val)
	}
}

func TestBatchDel(t *testing.T) {
	type kv struct {
		key []byte
		val []byte
	}
	tc := kv{
		key: []byte{1, 2, 3},
		val: []byte{8},
	}
	targ := kv{
		key: []byte{1, 2, 3},
		val: []byte{8},
	}

	b := &Batch{}
	b.Del(tc.key, tc.val)

	// mutate to exercise ownership
	tc.key[0]++
	tc.val[0]++

	if len(b.deletedPairs) != 1 {
		t.Fatalf("expected 1 added pair, got %d", len(b.addedPairs))
	}
	key := b.deletedPairs[0].key
	if !bytes.Equal(key, targ.key) {
		t.Errorf("expected key %v, got %v", targ.key, key)
	}
	val := b.deletedPairs[0].values
	if !bytes.Equal(val, targ.val) {
		t.Errorf("expected value %v, got %v", targ.val, val)
	}
}

func TestExecuteBatch(t *testing.T) {
	// same logic as in rdb_util
	makeFullVal := func(val []byte) []byte {
		b := make([]byte, 4)
		binary.LittleEndian.PutUint32(b[:], uint32(len(val)))
		return append(b, val...)
	}

	type kv struct {
		key []byte
		val []byte
	}

	toAdd := kv{
		key: []byte{1, 2, 3},
		val: []byte{2, 0, 3},
	}
	toDel := kv{
		key: []byte{3, 2, 1},
		val: []byte{1, 2, 6},
	}

	dir, err := os.MkdirTemp("", "rdb_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir) // clean up

	// can't mock rocksdb.Batch, so use real db
	testdb, err := NewRDB(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer testdb.Close()
	// first, add stuff
	if err := testdb.Add(toDel.key, toDel.val); err != nil {
		t.Fatal(err)
	}

	b := &Batch{}
	b.Add(toAdd.key, toAdd.val)
	b.Del(toDel.key, toDel.val)

	// now, execute batch
	if err := testdb.ExecuteBatch(b); err != nil {
		t.Fatal(err)
	}

	value1, err := testdb.db.Get(testdb.readOptions, toAdd.key)
	if err != nil {
		t.Fatal(err)
	}
	require.Equalf(t, makeFullVal(toAdd.val), value1, "added value should be present in DB")

	value2, err := testdb.db.Get(testdb.readOptions, toDel.key)
	require.Nil(t, err)
	require.Nilf(t, value2, "key should be removed from DB, not just value set to []byte{}")
}
