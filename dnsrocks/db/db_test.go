/*
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package db

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/facebook/dns/dnsrocks/testaid"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

var (
	// Expected to exist in TestDB v1
	validationKey1 = []byte{0, 0, 3, 'w', 'w', 'w', 7, 'e', 'x', 'a', 'm', 'p', 'l', 'e', 3, 'c', 'o', 'm', 0}
	// Expected to not exist in TestDB v1
	validationKey2 = []byte{0, 0, 3, 'f', 'o', 'o', 7, 'e', 'x', 'a', 'm', 'p', 'l', 'e', 4, 'c', 'o', 'o', 'm', 0}
	// Expected to exist in TestDB v2
	validationKey1v2 = []byte{0, 111, 3, 'c', 'o', 'm', 7, 'e', 'x', 'a', 'm', 'p', 'l', 'e', 3, 'w', 'w', 'w', 0, 0, 0}
	// Expected to not exist in TestDB v2
	validationKey2v2 = []byte{0, 111, 4, 'c', 'o', 'o', 'm', 7, 'e', 'x', 'a', 'm', 'p', 'l', 'e', 3, 'f', 'o', 'o', 0, 0, 0}
)

// TestDbKeyValidation verify the db key validation function works as expected
func TestDbKeyValidationV1Keys(t *testing.T) {
	testCases := []struct {
		key []byte
		err error
	}{
		{
			key: validationKey1,
			err: nil,
		},
		{
			key: validationKey2,
			err: ErrValidationKeyNotFound,
		},
	}

	testDBs := []testaid.TestDB{testaid.TestCDB, testaid.TestRDB}

	for _, testDb := range testDBs {
		db, err := Open(testDb.Path, testDb.Driver)
		if err != nil {
			t.Fatalf("Failed to initialize DB: %v", err)
		}
		defer db.Destroy()

		for i, test := range testCases {
			t.Run(fmt.Sprintf("Testing %v", test.key), func(t *testing.T) {
				err := db.ValidateDbKey(test.key)
				require.Equalf(t, test.err, err, "test %d expected error", i)
			})
		}
	}
}

// TestDbKeyValidation verify the db key validation function works as expected
func TestDbKeyValidationV2Keys(t *testing.T) {
	testCases := []struct {
		key []byte
		err error
	}{
		{
			key: validationKey1v2,
			err: nil,
		},
		{
			key: validationKey2v2,
			err: ErrValidationKeyNotFound,
		},
	}

	testDBs := []testaid.TestDB{testaid.TestRDBV2}

	for _, testDb := range testDBs {
		db, err := Open(testDb.Path, testDb.Driver)
		if err != nil {
			t.Fatalf("Failed to initialize DB: %v", err)
		}
		defer db.Destroy()

		for i, test := range testCases {
			t.Run(fmt.Sprintf("Testing %v", test.key), func(t *testing.T) {
				err := db.ValidateDbKey(test.key)
				require.Equalf(t, test.err, err, "test %d expected error", i)
			})
		}
	}
}

// getBaseMockDBI returns a basic MockDBI
func getBaseMockDBI(ctrl *gomock.Controller) *MockDBI {
	mockDbi := NewMockDBI(ctrl)
	mockDbi.EXPECT().NewContext().AnyTimes()
	mockDbi.EXPECT().FreeContext(gomock.Any()).AnyTimes()
	mockDbi.EXPECT().Find(gomock.Any(), gomock.Any()).AnyTimes()
	return mockDbi
}

// TestDbReload verifies that the correct DBs are used after reload
func TestDbReload(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockDbi := getBaseMockDBI(ctrl)
	oldDb := &DB{dbi: mockDbi}
	var reloadTimeout = 1 * time.Second

	// Test 1: reload DB with an invalid DB, expect that they are not switched,
	// the new DB is closed, and correct error is returned
	mockDbiInvalid := getBaseMockDBI(ctrl)
	mockDbiInvalid.EXPECT().ForEach(gomock.Any(), gomock.Any(), gomock.Any()).Return(errors.New("Error"))
	mockDbiInvalid.EXPECT().ClosestKeyFinder().Return(nil)
	mockDbiInvalid.EXPECT().Close()
	mockDbi.EXPECT().Reload(gomock.Any()).Return(mockDbiInvalid, nil)

	// path and key do not actually matter (as long as key is not empty) since
	// the underlying DBI is a mock
	newDb, err := oldDb.Reload("", validationKey1, reloadTimeout)
	require.NotEmpty(t, err)
	require.NotEqual(t, ErrValidationKeyNotFound, err)
	require.Same(t, oldDb, newDb)

	// Test 2: reload DB with a valid DB that does not contain the validation key,
	// expect that they are not switched, the new DB is closed, and correct error
	// is returned
	mockDbiMissingKey := getBaseMockDBI(ctrl)
	// simulates a key miss by noop ForEach call
	mockDbiMissingKey.EXPECT().ForEach(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
	mockDbiMissingKey.EXPECT().ClosestKeyFinder().Return(nil)
	mockDbiMissingKey.EXPECT().Close()
	mockDbi.EXPECT().Reload(gomock.Any()).Return(mockDbiMissingKey, nil)

	newDb, err = oldDb.Reload("", validationKey1, reloadTimeout)
	require.Equal(t, ErrValidationKeyNotFound, err)
	require.Same(t, oldDb, newDb)

	// Test 3: reload DB with a valid DB, expect that they are switched, the old
	// DB is closed, and there is no error
	mockDbiValid := getBaseMockDBI(ctrl)
	// simulates a key hit by returning a value on first call to FindNext, followed by an EOF
	foreachCallback := func(_ []byte, f func(value []byte) error, _ Context) error {
		err := f([]byte("{}"))
		return err
	}
	mockDbiValid.EXPECT().ForEach(gomock.Any(), gomock.Any(), gomock.Any()).Do(foreachCallback).Return(nil)
	mockDbiValid.EXPECT().ClosestKeyFinder().Return(nil)
	mockDbi.EXPECT().Reload(gomock.Any()).Return(mockDbiValid, nil)
	mockDbi.EXPECT().Close()

	newDb, err = oldDb.Reload("", validationKey1, reloadTimeout)
	require.Empty(t, err)
	require.NotSame(t, oldDb, newDb)
}
