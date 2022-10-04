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
	"fmt"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
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
	assert.Equal(t, kvAfter, kvBefore)
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
	for i := 0; i < len(testCases); i++ {
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			assert.Equal(
				t,
				testCases[i].dataAfter,
				appendValues(testCases[i].dataBefore, testCases[i].newVals),
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
	for i := 0; i < len(testCases); i++ {
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			after, err := delValue(testCases[i].dataBefore, testCases[i].delVal)
			assert.Equal(t, testCases[i].dataAfter, after)
			assert.Equal(t, testCases[i].expectedErr, err)
		})
	}
}
