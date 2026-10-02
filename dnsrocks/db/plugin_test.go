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
	"math"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

var pluginQname = []byte{3, 'w', 'w', 'w', 7, 'e', 'x', 'a', 'm', 'p', 'l', 'e', 3, 'c', 'o', 'm', 0}

func newPluginReader(t *testing.T, pluginValue []byte, pluginErr error) *DataReader {
	t.Helper()
	ctrl := gomock.NewController(t)
	mockDbi := NewMockDBI(ctrl)
	mockDbi.EXPECT().FindMap(gomock.Any(), pluginMapKeyElement, gomock.Any()).Return(pluginValue, pluginErr)
	return &DataReader{db: &DB{dbi: mockDbi}}
}

func TestParsePluginValue(t *testing.T) {
	testCases := []struct {
		name     string
		value    []byte
		wantName []byte
		wantArgs []byte
	}{
		{
			name:     "name and args",
			value:    []byte("pinned\000loc=abc1c01"),
			wantName: []byte("pinned"),
			wantArgs: []byte("loc=abc1c01"),
		},
		{
			name:     "name with empty args",
			value:    []byte("pinned\000"),
			wantName: []byte("pinned"),
			wantArgs: []byte{},
		},
		{
			name:     "name without separator",
			value:    []byte("pinned"),
			wantName: []byte("pinned"),
			wantArgs: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			name, args := parsePluginValue(tc.value)
			require.Equal(t, tc.wantName, name)
			require.Equal(t, tc.wantArgs, args)
		})
	}
}

func TestEncodeID(t *testing.T) {
	twoByteID, err := encodeID("ab")
	require.NoError(t, err)
	require.Equal(t, ID{'a', 'b'}, twoByteID)

	longID, err := encodeID("abc1c01")
	require.NoError(t, err)
	require.Equal(t, ID{0xff, 7, 'a', 'b', 'c', '1', 'c', '0', '1'}, longID)

	_, err = encodeID("a")
	require.Error(t, err)
	_, err = encodeID(strings.Repeat("x", math.MaxUint8+1))
	require.Error(t, err)
}

func TestNewLocation(t *testing.T) {
	location, err := NewLocation("plugin", "abc1c01", 24)
	require.NoError(t, err)
	require.Equal(t, Location{
		MapID: ID{0xff, 6, 'p', 'l', 'u', 'g', 'i', 'n'},
		LocID: ID{0xff, 7, 'a', 'b', 'c', '1', 'c', '0', '1'},
		Mask:  24,
	}, location)

	_, err = NewLocation("x", "abc1c01", 0)
	require.ErrorContains(t, err, "map name")
	_, err = NewLocation("plugin", "x", 0)
	require.ErrorContains(t, err, "location name")
}

func TestFindPluginMap(t *testing.T) {
	reader := newPluginReader(t, []byte("pinned\000loc=abc1c01"), nil)
	record, found, err := reader.FindPluginMap(pluginQname)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, PluginMap{Plugin: "pinned", Args: "loc=abc1c01"}, record)
}

func TestFindPluginMapNotFound(t *testing.T) {
	reader := newPluginReader(t, nil, nil)
	_, found, err := reader.FindPluginMap(pluginQname)
	require.NoError(t, err)
	require.False(t, found)
}

func TestFindPluginMapError(t *testing.T) {
	lookupErr := errors.New("lookup failed")
	reader := newPluginReader(t, nil, lookupErr)
	_, _, err := reader.FindPluginMap(pluginQname)
	require.ErrorIs(t, err, lookupErr)
}
