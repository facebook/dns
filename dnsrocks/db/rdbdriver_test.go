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
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUnpackLocation(t *testing.T) {
	testCases := []struct {
		name         string
		key          []byte
		value        []byte
		wantLocation []byte
		wantMask     uint8
		wantErr      bool
	}{
		{
			name:    "empty key and value",
			wantErr: false,
		},
		{
			name:    "empty key",
			value:   []byte{2, 0, 0, 0, 3, 4},
			wantErr: true,
		},
		{
			name:    "empty location",
			key:     []byte{1, 2, 3},
			wantErr: false, // empty location is valid
		},
		{
			name:    "short location",
			key:     []byte{1, 2, 3},
			value:   []byte{2, 0, 0}, // value should be at least 4 bytes (uint32 for length of multi-value field)
			wantErr: true,
		},
		{
			name:    "not enough bytes for provided length",
			key:     []byte{1, 2, 3},
			value:   []byte{4, 0, 0, 0, 3, 2}, // 4 is the length of the first multi-value field
			wantErr: true,
		},
		{
			name:    "not enough bytes for long location",
			key:     []byte{1, 2, 3},
			value:   []byte{6, 0, 0, 0, 255, 8, 3, 4, 5, 6}, // 4 is the length of the first multi-value, 255 is the marker, 8 is the length of the location
			wantErr: true,
		},
		{
			name:     "zero length location",
			key:      []byte{1, 2, 3},
			value:    []byte{0, 0, 0, 0},
			wantErr:  false,
			wantMask: 3,
		},
		{
			name:         "two-byte location",
			key:          []byte{1, 2, 3},
			value:        []byte{2, 0, 0, 0, 82, 10}, // 2 is the length of the first multi-value field, 82, 10 is the location
			wantErr:      false,
			wantLocation: []byte{82, 10},
			wantMask:     3,
		},
		{
			name:         "long location without marker",
			key:          []byte{1, 2, 3},
			value:        []byte{4, 0, 0, 0, 82, 10, 7, 8}, // 4 is the length of the first multi-value field, 82, 10, 7, 8 is the location
			wantErr:      false,
			wantLocation: []byte{82, 10, 7, 8},
			wantMask:     3,
		},
		{
			name:         "long location with marker",
			key:          []byte{1, 2, 3},
			value:        []byte{6, 0, 0, 0, 0xff, 4, 97, 108, 101, 120}, // 6 is the length of the first multi-value field, 255 is the marker, 4 is the length of the location, 'alex' is the location
			wantErr:      false,
			wantLocation: []byte("\xff\x04alex"),
			wantMask:     3,
		},
		{
			name:         "multi-value two-byte location",
			key:          []byte{1, 2, 3},
			value:        []byte{2, 0, 0, 0, 82, 10, 2, 0, 0, 0, 3, 4}, // 2 is the length of the first multi-value field, 82, 10 is the location, 2 is the length of the second multi-value field, 3, 4 is the location
			wantErr:      false,
			wantLocation: []byte{82, 10},
			wantMask:     3,
		},
		{
			name:     "multi-value empty location",
			key:      []byte{1, 2, 3},
			value:    []byte{0, 0, 0, 0, 2, 0, 0, 0, 3, 4}, // 0 is the length of the first multi-value field, 2 is the length of the second multi-value field, 3, 4 is the second location
			wantErr:  false,
			wantMask: 3,
		},
		{
			name:         "multi-value long location",
			key:          []byte{1, 2, 3},
			value:        []byte{6, 0, 0, 0, 255, 4, 97, 108, 101, 120, 2, 0, 0, 0, 3, 4}, // 6 is the length of the first multi-value field, 255 is the marker, 4 is the length of the location, 'alex' is the location, 2 is the length of the second multi-value field, 3, 4 is the second location
			wantErr:      false,
			wantLocation: []byte("\xff\x04alex"),
			wantMask:     3,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			gotLocation, gotMask, err := unpackLocation(tc.key, tc.value)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.wantLocation, gotLocation)
			require.Equal(t, tc.wantMask, gotMask)
		})
	}
}
