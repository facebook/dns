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
	"slices"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/facebook/dns/dnsrocks/dnsdata"
)

func TestRDBcreateBuckets(t *testing.T) {
	testCases := []struct {
		values        []*dnsdata.MapRecord
		buckets       []bucket
		minBucketSize int
		maxBucketNum  int
	}{
		{
			// single key
			values: []*dnsdata.MapRecord{
				{Key: []byte{0}},
			},
			buckets: []bucket{
				{startOffset: 0, endOffset: 1},
			},
			minBucketSize: 1,
			maxBucketNum:  1,
		},
		{
			// all keys should be in one basket
			values: []*dnsdata.MapRecord{
				{Key: []byte{0}},
				{Key: []byte{0}},
				{Key: []byte{0}},
			},
			buckets: []bucket{
				{startOffset: 0, endOffset: 3},
			},
			minBucketSize: 1,
			maxBucketNum:  5,
		},
		{
			// all keys should be in two baskets
			values: []*dnsdata.MapRecord{
				{Key: []byte{0}},
				{Key: []byte{0}},
				{Key: []byte{0}},
				{Key: []byte{1}},
				{Key: []byte{1}},
			},
			buckets: []bucket{
				{startOffset: 0, endOffset: 3},
				{startOffset: 3, endOffset: 5},
			},
			minBucketSize: 2,
			maxBucketNum:  5,
		},
		{
			// four baskets of unequal size
			values: []*dnsdata.MapRecord{
				{Key: []byte{0}},
				{Key: []byte{1}},
				{Key: []byte{2}},
				{Key: []byte{3}},
				{Key: []byte{4}},
				{Key: []byte{5}},
				{Key: []byte{6}},
				{Key: []byte{7}},
				{Key: []byte{8}},
				{Key: []byte{9}},
			},
			buckets: []bucket{
				{startOffset: 0, endOffset: 2},
				{startOffset: 2, endOffset: 4},
				{startOffset: 4, endOffset: 6},
				{startOffset: 6, endOffset: 10},
			},
			minBucketSize: 1,
			maxBucketNum:  4,
		},
		{
			// four baskets of unequal size, different distribution because of keys
			values: []*dnsdata.MapRecord{
				{Key: []byte{0}},
				{Key: []byte{0}},
				{Key: []byte{0}},
				{Key: []byte{0}},
				{Key: []byte{0}},
				{Key: []byte{5}},
				{Key: []byte{6}},
				{Key: []byte{7}},
				{Key: []byte{8}},
				{Key: []byte{9}},
			},
			buckets: []bucket{
				{startOffset: 0, endOffset: 5},
				{startOffset: 5, endOffset: 7},
				{startOffset: 7, endOffset: 9},
				{startOffset: 9, endOffset: 10},
			},
			minBucketSize: 1,
			maxBucketNum:  4,
		},
	}

	for i, testCase := range testCases {
		b := Builder{values: testCase.values}
		b.createWriteBuckets(testCase.minBucketSize, testCase.maxBucketNum)
		require.Equalf(
			t,
			testCase.buckets,
			b.buckets,
			"Test case #%d mismatch: expected %v, got %v", i, testCase.buckets, b.buckets,
		)
	}
}

func TestMergeValueBuckets(t *testing.T) {
	valuesBuckets := [][]*dnsdata.MapRecord{
		{
			{Key: []byte{0}},
			{Key: []byte{0}},
			{Key: []byte{0}},
		},
		{
			{Key: []byte{14}},
		},
		{
			{Key: []byte{6}},
			{Key: []byte{7}},
			{Key: []byte{8}},
			{Key: []byte{9}},
		},
		{
			{Key: []byte{0}},
			{Key: []byte{0}},
			{Key: []byte{5}},
		},
		{}, // empty bucket
		{
			{Key: []byte{12}},
			{Key: []byte{13}},
		},
		{
			{Key: []byte{10}},
			{Key: []byte{11}},
		},
	}
	b := Builder{valueBuckets: valuesBuckets}
	b.mergeValueBuckets()
	require.Equal(t, 1, len(b.valueBuckets))
	require.Equal(t, 15, len(b.values))
	require.True(t, slices.IsSortedFunc(b.values, keyOrder))
}

func TestScheduleAdd(t *testing.T) {
	testData := []dnsdata.MapRecord{
		{
			Key:   []byte{1, 2, 3},
			Value: []byte{8},
		},
		{
			Key:   []byte{2, 7, 9},
			Value: []byte{8, 12},
		},
	}

	b := &Builder{valueBuckets: make([][]*dnsdata.MapRecord, 3)}
	for _, r := range testData {
		b.ScheduleAdd(r)
	}

	require.Equal(t, 1, len(b.valueBuckets[0]))
	require.Equal(t, 0, len(b.valueBuckets[1]))
	require.Equal(t, 1, len(b.valueBuckets[2]))
}
