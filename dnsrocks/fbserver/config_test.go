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

package fbserver

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIPAnsSet(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  ipAns
	}{
		{
			name:  "IPv4 uses default max answer",
			value: "192.0.2.53",
			want:  ipAns{"192.0.2.53": 1},
		},
		{
			name:  "IPv6 uses configured max answer",
			value: "2001:0db8::35,8",
			want:  ipAns{"2001:db8::35": 8},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := make(ipAns)
			require.NoError(t, got.Set(test.value))
			require.Equal(t, test.want, got)
		})
	}
}

func TestIPAnsSetRejectsInvalidValues(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		wantErr string
	}{
		{
			name:    "invalid IP",
			value:   "not-an-ip",
			wantErr: "invalid IP address \"not-an-ip\"",
		},
		{
			name:    "non-numeric max answer",
			value:   "192.0.2.53,many",
			wantErr: "invalid max answer \"many\"",
		},
		{
			name:    "zero max answer",
			value:   "192.0.2.53,0",
			wantErr: "max answer must be greater than zero, got 0",
		},
		{
			name:    "negative max answer",
			value:   "192.0.2.53,-1",
			wantErr: "max answer must be greater than zero, got -1",
		},
		{
			name:    "too many fields",
			value:   "192.0.2.53,1,2",
			wantErr: "invalid argument value \"192.0.2.53,1,2\"",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := make(ipAns)
			err := got.Set(test.value)
			require.ErrorContains(t, err, test.wantErr)
			require.Empty(t, got)
		})
	}
}

func TestIPAnsString(t *testing.T) {
	require.Equal(t, "", ipAns(nil).String())
	require.Equal(t, "", ipAns{}.String())
	require.Equal(
		t,
		"192.0.2.35,8 192.0.2.53,1 2001:db8::35,4",
		ipAns{
			"2001:db8::35": 4,
			"192.0.2.53":   1,
			"192.0.2.35":   8,
		}.String(),
	)
}
