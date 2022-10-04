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
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRDBcreateBuckets(t *testing.T) {
	testCases := []struct {
		values        []keyValues
		buckets       []bucket
		minBucketSize int
		maxBucketNum  int
	}{
		{
			// single key
			values: []keyValues{
				{key: []byte{0}},
			},
			buckets: []bucket{
				{startOffset: 0, endOffset: 1},
			},
			minBucketSize: 1,
			maxBucketNum:  1,
		},
		{
			// all keys should be in one basket
			values: []keyValues{
				{key: []byte{0}},
				{key: []byte{0}},
				{key: []byte{0}},
			},
			buckets: []bucket{
				{startOffset: 0, endOffset: 3},
			},
			minBucketSize: 1,
			maxBucketNum:  5,
		},
		{
			// all keys should be in two baskets
			values: []keyValues{
				{key: []byte{0}},
				{key: []byte{0}},
				{key: []byte{0}},
				{key: []byte{1}},
				{key: []byte{1}},
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
			values: []keyValues{
				{key: []byte{0}},
				{key: []byte{1}},
				{key: []byte{2}},
				{key: []byte{3}},
				{key: []byte{4}},
				{key: []byte{5}},
				{key: []byte{6}},
				{key: []byte{7}},
				{key: []byte{8}},
				{key: []byte{9}},
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
			values: []keyValues{
				{key: []byte{0}},
				{key: []byte{0}},
				{key: []byte{0}},
				{key: []byte{0}},
				{key: []byte{0}},
				{key: []byte{5}},
				{key: []byte{6}},
				{key: []byte{7}},
				{key: []byte{8}},
				{key: []byte{9}},
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
		b.createBuckets(testCase.minBucketSize, testCase.maxBucketNum)
		assert.Equal(
			t,
			testCase.buckets,
			b.buckets,
			fmt.Sprintf("Test case #%d mismatch: expected %v, got %v", i, testCase.buckets, b.buckets),
		)
	}
}

func TestScheduleAdd(t *testing.T) {
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

	b := &Builder{values: make([]keyValues, 0, 1)}
	b.ScheduleAdd(tc.key, tc.val)

	// mutate to exercise ownership
	tc.key[0]++
	tc.val[0]++

	if len(b.values) != 1 {
		t.Fatalf("expected 1 value, got %d", len(b.values))
	}
	key := b.values[0].key
	if !bytes.Equal(key, targ.key) {
		t.Errorf("expected key %v, got %v", targ.key, key)
	}
	if len(b.values[0].values) != 1 {
		t.Fatalf("expected 1 value, got %d", len(b.values[0].values))
	}
	val := b.values[0].values[0]
	if !bytes.Equal(val, targ.val) {
		t.Errorf("expected value %v, got %v", targ.val, val)
	}
}
