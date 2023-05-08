/*
Copyright (c) Facebook, Inc. and its affiliates.
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

package snoop

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFieldFromString(t *testing.T) {
	str, err := FieldFromString("PID")
	require.Nil(t, err)
	require.Equal(t, FieldID(0), str)

	str, err = FieldFromString("pname")
	require.Nil(t, err)
	require.Equal(t, FieldID(1), str)

	str, err = FieldFromString("RCODE")
	require.Nil(t, err)
	require.Equal(t, FieldID(5), str)

	_, err = FieldFromString("RCODEE")
	require.NotNil(t, err)
}

func TestParseFields(t *testing.T) {
	fields, err := ParseFields("QNAME,TYPE,PID,QTIME,RTIME")
	require.Nil(t, err)
	require.Equal(t, FieldID(4), fields[0])
	require.Equal(t, FieldID(3), fields[1])
	require.Equal(t, FieldID(0), fields[2])
	require.Equal(t, FieldID(7), fields[3])
	require.Equal(t, FieldID(8), fields[4])

	_, err = ParseFields("QNAME,type,PID,")
	require.NotNil(t, err)
	_, err = ParseFields("QQNAME,TYPE,PID")
	require.NotNil(t, err)
	_, err = ParseFields("")
	require.NotNil(t, err)
}
