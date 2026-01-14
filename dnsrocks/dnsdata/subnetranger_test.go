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

package dnsdata

import (
	"bytes"
	"fmt"
	"reflect"
	"testing"
)

func TestSubnetRanger(t *testing.T) {
	tc := []string{
		"%de,0.0.0.0/0,m1",
		"%ab,2a00:1fa0:42d8::/64,m1",
		"%a\\001,192.168.1.0/24,m1",
	}
	targText := `!\155\061,::
!\155\061,0.0.0.0,0,\144\145
!\155\061,192.168.1.0,24,\141\001
!\155\061,192.168.2.0,0,\144\145
!\155\061,::1:0:0:0
!\155\061,2a00:1fa0:42d8::,64,\141\142
!\155\061,2a00:1fa0:42d8:1::
`
	targMap := []MapRecord{
		// RangePoint origin
		{
			Key:   []byte{0, 0, 0, '!', 'm', '1', 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0},
			Value: []byte(nil),
		},
		// RangePoint start ::/0
		{
			Key:   []byte{0, 0, 0, '!', 'm', '1', 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xff, 0xff, 0x0, 0x0, 0x0, 0x0, 0x60},
			Value: []byte{'d', 'e'},
		},
		// RangePoint start 192.168.1.0/24
		{
			Key:   []byte{0, 0, 0, '!', 'm', '1', 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xff, 0xff, 192, 168, 1, 0, 120},
			Value: []byte{'a', '\001'},
		},
		// RangePoint end 192.168.1.0/24
		{
			Key:   []byte{0, 0, 0, '!', 'm', '1', 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xff, 0xff, 192, 168, 2, 0, 0x60},
			Value: []byte{'d', 'e'},
		},
		// RangePoint end 0.0.0.0/0
		{
			Key:   []byte{0, 0, 0, '!', 'm', '1', 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x1, 0x00, 0x00, 0, 0, 0, 0, 0},
			Value: []byte(nil),
		},
		// RangePoint start 2a00:1fa0:42d8::/64
		{
			Key:   []byte{0, 0, 0, '!', 'm', '1', 0x2a, 0x0, 0x1f, 0xa0, 0x42, 0xd8, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 64},
			Value: []byte{'a', 'b'},
		},
		// RangePoint end 2a00:1fa0:42d8::/64
		{
			Key:   []byte{0, 0, 0, '!', 'm', '1', 0x2a, 0x0, 0x1f, 0xa0, 0x42, 0xd8, 0x0, 0x1, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0},
			Value: []byte(nil),
		},
	}
	t.Run("MarshalMap", func(t *testing.T) {
		codec := new(Codec)
		codec.Acc.Ranger.Enable()
		codec.Acc.NoPrefixSets = true
		codec.NoRnetOutput = true
		out := make([]MapRecord, 0) //nolint:prealloc
		for _, in := range tc {
			v, err := codec.ConvertLn([]byte(in))
			if err != nil {
				t.Fatalf("error converting %s: %v", in, err)
			}
			out = append(out, v...)
		}
		v, err := codec.Acc.MarshalMap()
		out = append(out, v...)
		if err != nil {
			t.Fatalf("error encoding %v (opaque acc): %v", tc, err)
		}
		if fmt.Sprintf("%v", out) != fmt.Sprintf("%v", targMap) {
			t.Fatalf("encoded differs:\n---- got: \n%#v---- expected:\n%#v====", out, targMap)
		}
	})
	t.Run("MarshalText", func(t *testing.T) {
		codec := new(Codec)
		codec.Acc.Ranger.Enable()
		codec.Acc.NoPrefixSets = true
		for _, in := range tc {
			if _, err := codec.DecodeLn([]byte(in)); err != nil {
				t.Fatalf("error decoding %s: %v", in, err)
			}
		}
		out, err := codec.Acc.MarshalText()
		if err != nil {
			t.Fatalf("error encoding %v (opaque Acc): %v", tc, err)
		}
		if string(out) != targText {
			t.Fatalf("text encoding mismatch:\n---- got: \n%v---- expected:\n%v====", string(out), targText)
		}
	})
	t.Run("Feedback", func(t *testing.T) {
		codec := new(Codec)
		codec.Acc.Ranger.Enable()
		codec.Acc.NoPrefixSets = true
		codec.NoRnetOutput = true
		out := []MapRecord{}
		for _, in := range bytes.Split([]byte(targText), []byte("\n")) {
			if len(in) < 1 {
				continue
			}
			v, err := codec.ConvertLn(in)
			if err != nil {
				t.Fatalf("error converting %v: %v", string(in), err)
			}
			out = append(out, v...)
		}
		if !reflect.DeepEqual(out, targMap) {
			t.Fatalf("encoded differs: \n---- got: \n%#v\n---- expected:\n%#v\n====", out, targMap)
		}
	})
}
