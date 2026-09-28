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

package pluginargs

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateAccepts(t *testing.T) {
	testCases := []struct {
		name string
		args string
	}{
		{name: "no arguments", args: ""},
		{name: "single pair", args: "k=v"},
		{name: "several pairs", args: "k1=v1;k2=v2;k3=v3"},
		{name: "separator inside the value", args: "expr=a=b"},
		{name: "list inside the value", args: "regions=abc|def"},
		{name: "keys differing only by case are distinct", args: "k=1;K=2"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			require.NoError(t, Validate(tc.args))
		})
	}
}

func TestValidateRejects(t *testing.T) {
	testCases := []struct {
		name string
		args string
		want error
	}{
		{name: "trailing delimiter", args: "k=v;", want: ErrEmptyArg},
		{name: "leading delimiter", args: ";k=v", want: ErrEmptyArg},
		{name: "doubled delimiter", args: "k1=v1;;k2=v2", want: ErrEmptyArg},
		{name: "only a delimiter", args: ";", want: ErrEmptyArg},
		{name: "bare key", args: "debug", want: ErrMissingSeparator},
		{name: "bare key beside a pair", args: "debug;tier=foo", want: ErrMissingSeparator},
		{name: "missing key", args: "=v", want: ErrEmptyKey},
		{name: "only a separator", args: "=", want: ErrEmptyKey},
		{name: "separator with no value", args: "k=", want: ErrEmptyValue},
		{name: "separator with no value beside a pair", args: "k1=v1;k2=", want: ErrEmptyValue},
		{name: "duplicate key", args: "k=v1;k=v2", want: ErrDuplicateKey},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			require.ErrorIs(t, Validate(tc.args), tc.want)
		})
	}
}

func TestGet(t *testing.T) {
	const args = "tier=foo;expr=a=b;app_id=www"

	testCases := []struct {
		name      string
		key       string
		wantValue string
		wantFound bool
	}{
		{name: "first key", key: "tier", wantValue: "foo", wantFound: true},
		{name: "last key", key: "app_id", wantValue: "www", wantFound: true},
		{name: "value keeps later separators", key: "expr", wantValue: "a=b", wantFound: true},
		{name: "absent key", key: "nope", wantFound: false},
		{name: "a prefix of a key is not that key", key: "app", wantFound: false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			value, found := Get(args, tc.key)
			require.Equal(t, tc.wantFound, found)
			require.Equal(t, tc.wantValue, value)
		})
	}
}

func TestGetOnEmptyArgs(t *testing.T) {
	value, found := Get("", "k")
	require.False(t, found)
	require.Empty(t, value)
}
