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
	str1 := []byte("\003net\005fbcdn\002xx\010scontent\002bc\000")
	str2 := []byte("\003net\005fbcdn\002xx\010scontent\001a\002bc\001e\000")
	expectedPrefix := []byte("\003net\005fbcdn\002xx\010scontent\000")

	prefixLen := findCommonLongestPrefix(str1, str2)

	require.Equal(t, len(expectedPrefix)-1, prefixLen)

	prefixLen = findCommonLongestPrefix(str2, str1)

	require.Equal(t, len(expectedPrefix)-1, prefixLen)
}
