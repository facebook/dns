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

package rdb

import (
	"fmt"
	"io"
	"slices"
	"testing"

	"github.com/facebook/dns/dnsrocks/dnsdata"

	"github.com/stretchr/testify/require"
)

func TestRDBkvSort(t *testing.T) {
	kvBefore := kvList{
		{key: []byte{2}},
		{key: []byte{1}},
		{key: []byte{0}},
		{key: []byte{2}},
	}
	kvAfter := kvList{
		{key: []byte{0}},
		{key: []byte{1}},
		{key: []byte{2}},
		{key: []byte{2}},
	}
	kvBefore.Sort()
	require.Equal(t, kvAfter, kvBefore)
}

func TestRDBappendValues(t *testing.T) {
	testCases := []struct {
		dataBefore []byte
		newVals    [][]byte
		dataAfter  []byte
	}{
		{
			dataBefore: nil,
			newVals:    [][]byte{{1, 2, 3, 4, 5, 6}},
			dataAfter:  []byte{6, 0, 0, 0, 1, 2, 3, 4, 5, 6},
		},
		{
			dataBefore: nil,
			newVals: [][]byte{
				{1, 2, 3, 4, 5, 6},
				{7, 8, 9},
			},
			dataAfter: []byte{
				6, 0, 0, 0, 1, 2, 3, 4, 5, 6,
				3, 0, 0, 0, 7, 8, 9,
			},
		},
		{
			dataBefore: []byte{7, 0, 0, 0, 6, 5, 4, 3, 2, 1, 0},
			newVals: [][]byte{
				{7, 8, 9},
			},
			dataAfter: []byte{
				7, 0, 0, 0, 6, 5, 4, 3, 2, 1, 0,
				3, 0, 0, 0, 7, 8, 9,
			},
		},
		{
			dataBefore: []byte{7, 0, 0, 0, 6, 5, 4, 3, 2, 1, 0},
			newVals: [][]byte{
				{1, 2, 3, 4, 5, 6},
				{7, 8, 9},
			},
			dataAfter: []byte{
				7, 0, 0, 0, 6, 5, 4, 3, 2, 1, 0,
				6, 0, 0, 0, 1, 2, 3, 4, 5, 6,
				3, 0, 0, 0, 7, 8, 9,
			},
		},
	}
	for i := range len(testCases) {
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			data := appendValues(testCases[i].dataBefore, testCases[i].newVals[0])
			for _, nv := range testCases[i].newVals[1:] {
				data = appendValues(data, nv)
			}
			require.Equal(
				t,
				testCases[i].dataAfter,
				data,
			)
		})
	}
}

func TestRDBdelValues(t *testing.T) {
	testCases := []struct {
		dataBefore  []byte
		delVal      []byte
		dataAfter   []byte
		expectedErr error
	}{
		{
			dataBefore:  []byte{0, 0, 0},
			delVal:      []byte{1, 2, 3, 4, 5, 6},
			dataAfter:   nil,
			expectedErr: io.ErrUnexpectedEOF,
		},
		{
			dataBefore:  []byte{6, 0, 0, 0, 1, 2, 3, 4, 5},
			delVal:      []byte{1, 2, 3, 4, 5, 6},
			dataAfter:   nil,
			expectedErr: io.ErrUnexpectedEOF,
		},
		{
			dataBefore:  nil,
			delVal:      []byte{1, 2, 3, 4, 5, 6},
			dataAfter:   nil,
			expectedErr: ErrNXVal,
		},
		{
			dataBefore:  []byte{6, 0, 0, 0, 1, 2, 3, 4, 5, 6},
			delVal:      []byte{1, 2, 3, 4, 5, 6},
			dataAfter:   []byte{},
			expectedErr: nil,
		},
		{
			dataBefore: []byte{
				6, 0, 0, 0, 1, 2, 3, 4, 5, 6,
				3, 0, 0, 0, 7, 8, 9,
			},
			delVal: []byte{1, 2, 3, 4, 5, 6},
			dataAfter: []byte{
				3, 0, 0, 0, 7, 8, 9,
			},
			expectedErr: nil,
		},
		{
			dataBefore: []byte{
				7, 0, 0, 0, 6, 5, 4, 3, 2, 1, 0,
				3, 0, 0, 0, 7, 8, 9,
			},
			delVal:      []byte{7, 8, 9},
			dataAfter:   []byte{7, 0, 0, 0, 6, 5, 4, 3, 2, 1, 0},
			expectedErr: nil,
		},
		{
			dataBefore: []byte{
				7, 0, 0, 0, 6, 5, 4, 3, 2, 1, 0,
				6, 0, 0, 0, 1, 2, 3, 4, 5, 6,
				3, 0, 0, 0, 7, 8, 9,
			},
			delVal: []byte{1, 2, 3, 4, 5, 6},
			dataAfter: []byte{
				7, 0, 0, 0, 6, 5, 4, 3, 2, 1, 0,
				3, 0, 0, 0, 7, 8, 9,
			},
			expectedErr: nil,
		},
	}
	for i := range len(testCases) {
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			after, err := delValue(testCases[i].dataBefore, testCases[i].delVal)
			require.Equal(t, testCases[i].dataAfter, after)
			require.Equal(t, testCases[i].expectedErr, err)
		})
	}
}

func TestRdbStats(t *testing.T) {
	statsStr := `
rocksdb.block.cache.miss COUNT : 2
rocksdb.block.cache.hit COUNT : 0
rocksdb.block.cache.add COUNT : 2
rocksdb.block.cache.add.failures COUNT : 0
rocksdb.block.cache.index.miss COUNT : 0
rocksdb.block.cache.index.hit COUNT : 0
rocksdb.db.get.micros P50 : 34.000000 P95 : 58.000000 P99 : 58.000000 P100 : 58.000000 COUNT : 2 SUM : 88
rocksdb.compaction.times.micros P50 : 0.000000 P95 : 0.000000 P99 : 0.000000 P100 : 0.000000 COUNT : 0 SUM : 0
some broken line which will be discarded
pretending to be good
`
	expected := map[string]int64{
		"rocksdb.block.cache.miss":             2,
		"rocksdb.block.cache.hit":              0,
		"rocksdb.block.cache.add":              2,
		"rocksdb.block.cache.add.failures":     0,
		"rocksdb.block.cache.index.miss":       0,
		"rocksdb.block.cache.index.hit":        0,
		"rocksdb.db.get.micros.P50":            34,
		"rocksdb.db.get.micros.P95":            58,
		"rocksdb.db.get.micros.P99":            58,
		"rocksdb.db.get.micros.P100":           58,
		"rocksdb.compaction.times.micros.P50":  0,
		"rocksdb.compaction.times.micros.P95":  0,
		"rocksdb.compaction.times.micros.P99":  0,
		"rocksdb.compaction.times.micros.P100": 0,
		"pretending":                           0,
	}
	s := rdbStats(statsStr)
	require.Equal(t, expected, s)
}

func TestMerge(t *testing.T) {
	testCases := []struct {
		name  string
		aList []*dnsdata.MapRecord
		bList []*dnsdata.MapRecord
		want  []*dnsdata.MapRecord
	}{
		{
			name:  "both empty",
			aList: []*dnsdata.MapRecord{},
			bList: []*dnsdata.MapRecord{},
			want:  []*dnsdata.MapRecord{},
		},
		{
			name:  "first empty",
			aList: []*dnsdata.MapRecord{},
			bList: []*dnsdata.MapRecord{
				{Key: []byte{12, 33}},
				{Key: []byte{0, 2, 3}},
			},
			want: []*dnsdata.MapRecord{
				{Key: []byte{0, 2, 3}},
				{Key: []byte{12, 33}},
			},
		},
		{
			name: "second empty",
			aList: []*dnsdata.MapRecord{
				{Key: []byte{12, 33}},
				{Key: []byte{0, 2, 3}},
			},
			bList: []*dnsdata.MapRecord{},
			want: []*dnsdata.MapRecord{
				{Key: []byte{0, 2, 3}},
				{Key: []byte{12, 33}},
			},
		},
		{
			name: "two full lists",
			aList: []*dnsdata.MapRecord{
				{Key: []byte{0, 2, 3}},
				{Key: []byte{24, 2, 4}},
				{Key: []byte{1, 10, 8}},
				{Key: []byte{21, 2}},
				{Key: []byte{1}},
			},

			bList: []*dnsdata.MapRecord{
				{Key: []byte{12, 33}},
				{Key: []byte{0, 2, 3}},
			},
			want: []*dnsdata.MapRecord{
				{Key: []byte{0, 2, 3}},
				{Key: []byte{0, 2, 3}},
				{Key: []byte{1}},
				{Key: []byte{1, 10, 8}},
				{Key: []byte{12, 33}},
				{Key: []byte{21, 2}},
				{Key: []byte{24, 2, 4}},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			slices.SortFunc(tc.aList, keyOrder)
			slices.SortFunc(tc.bList, keyOrder)
			result := merge(tc.aList, tc.bList)
			require.Equal(t, tc.want, result)
		})
	}
}
