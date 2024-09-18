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

package db

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestQNameReverse2(t *testing.T) {
	straight := []byte("\010scontent\002xx\005fbcdn\003net\000")
	expected := []byte("\003net\005fbcdn\002xx\010scontent\000")

	reversed := reverseZoneName(straight)

	require.Equal(t, expected, reversed)
}

func TestQNameReverse3(t *testing.T) {
	straight := []byte("\010scontent\002xx\005fbcdn\003net\000")

	reversed := reverseZoneName(reverseZoneName(straight))

	require.Equal(t, straight, reversed)
}

func TestFindLongestCommonPrefix(t *testing.T) {
	testCases := []struct {
		name       string
		str1       []byte
		str2       []byte
		wantPrefix []byte
		wantLen    int
	}{
		{
			name:    "both empty",
			wantLen: 0,
		},
		{
			name:       "first empty",
			str1:       []byte{},
			str2:       []byte("\003net\005fbcdn\002xx\010scontent\001a\002bc\001e\000"),
			wantPrefix: []byte{},
			wantLen:    0,
		},
		{
			name:       "second empty",
			str1:       []byte("\003net\005fbcdn\002xx\010scontent\002bc\000"),
			str2:       []byte{},
			wantPrefix: []byte{},
			wantLen:    0,
		},
		{
			name:       "match",
			str1:       []byte("\003net\005fbcdn\002xx\010scontent\002bc\000"),
			str2:       []byte("\003net\005fbcdn\002xx\010scontent\001a\002bc\001e\000"),
			wantPrefix: []byte("\003net\005fbcdn\002xx\010scontent"),
			wantLen:    22,
		},
		{
			name:       "no match",
			str1:       []byte("\003net\005fbcdn\002xx\010scontent\002bc\000"),
			str2:       []byte("\003com\005test\002best\010why\000"),
			wantPrefix: []byte{},
			wantLen:    0,
		},
		{
			name:       "bad packed",
			str2:       []byte("\003net\005fbcdn\070xx\010scontent\000"), // \070 here is a bad packing directive, which will cause us to walk past the string length
			str1:       []byte("\003net\005fbcdn\070xx\010scontent\000"),
			wantPrefix: []byte("\003net\005fbcdn"),
			wantLen:    10,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := findCommonLongestPrefix(tc.str1, tc.str2)
			require.Equal(t, tc.wantLen, got)
			gotPrefix := tc.str1[:got]
			require.Equal(t, tc.wantPrefix, gotPrefix)
			require.Equal(t, tc.str2[:got], gotPrefix)
		})
	}
}
