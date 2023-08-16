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

package debuginfo

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAdd(t *testing.T) {
	v := new(Values)
	v.Add("foo", "bar")
	require.Equal(t, []Pair{{Key: "foo", Val: "bar"}}, []Pair(*v))
}

func TestEncode(t *testing.T) {
	v := new(Values)
	v.Add("foo", "bar")
	v.Add("123", "456")
	require.Equal(t, "foo=bar 123=456", v.Encode())
}

func TestGet(t *testing.T) {
	v := new(Values)
	v.Add("foo", "bar")
	require.Equal(t, "bar", v.Get("foo"))
	require.Empty(t, v.Get("not present"))
}

func TestHas(t *testing.T) {
	v := new(Values)
	v.Add("foo", "bar")
	require.True(t, v.Has("foo"))
	require.False(t, v.Has("not present"))
}

func TestEmptyValue(t *testing.T) {
	v := new(Values)
	v.Add("empty-value", "")
	require.True(t, v.Has("empty-value"))
	require.Empty(t, v.Encode())
}
