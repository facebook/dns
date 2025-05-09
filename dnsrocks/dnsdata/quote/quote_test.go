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

package quote

import (
	"errors"
	"testing"
)

type quoteTest struct {
	in  string
	out string
}

var quotetests = []quoteTest{
	{"abc", "abc"},
	{"a\001", "a\\x01"},
	{"\000\001", "\\x00\\x01"},
	{"a\005b", "a\\x05b"},
	{"\000\217", "\\x00\\x8f"},
	{"on", "on"},
	{"\000\054", "\\x00\\054"},
	{"\000\072", "\\x00\\072"},
	{`test " \`, `test " \\`},
}

func TestBquote(t *testing.T) {
	for _, tt := range quotetests {
		if out := string(Bquote([]byte(tt.in))); out != tt.out {
			t.Errorf("samples differ: %v != %v", out, tt.out)
		}
	}
}

type unquoteTest struct {
	in  string
	out string
	err error
}

var unquotetests = []unquoteTest{
	{"abc", "abc", nil},
	{"a\\001", "a\001", nil},
	{"\\000\\001", "\000\001", nil},
	{"a\\001b", "a\001b", nil},
	{"a\\x05b", "a\005b", nil},
	{"\\000\\217", "\000\217", nil},
	{"\\000\\054", "\000,", nil},
	{"\\000\\072", "\000:", nil},
	{`test " \\`, `test " \`, nil},
	{"\\000\\082", "\\000\\082", errors.New("invalid syntax")},
}

func TestUnquote(t *testing.T) {
	for _, tt := range unquotetests {
		out, err := Unquote(tt.in)

		if out != tt.out {
			t.Errorf("samples differ: %v != %v", out, tt.out)
		}
		if err != tt.err && err.Error() != tt.err.Error() {
			t.Errorf("error differ: %v != %v", err, tt.err)
		}
	}
}

func TestBunquote(t *testing.T) {
	for _, tt := range unquotetests {
		out, err := Bunquote([]byte(tt.in))

		if string(out) != tt.out {
			t.Errorf("samples differ: %v != %v", out, tt.out)
		}
		if err != tt.err && err.Error() != tt.err.Error() {
			t.Errorf("error differ: %T != %T", err, tt.err)
		}
	}
}

func BenchmarkBquote(b *testing.B) {
	for n := 0; n < b.N; n++ {
		Bquote([]byte("\001abc"))
	}
}

func BenchmarkUnquoteSimple(b *testing.B) {
	for n := 0; n < b.N; n++ {
		_, err := Unquote(".001abc")
		if err != nil {
			b.Errorf("%v", err)
		}
	}
}

func BenchmarkUnquote(b *testing.B) {
	for n := 0; n < b.N; n++ {
		_, err := Unquote("\\001abc")
		if err != nil {
			b.Errorf("%v", err)
		}
	}
}

func BenchmarkBunquote(b *testing.B) {
	for n := 0; n < b.N; n++ {
		_, err := Bunquote([]byte("\\001abc"))
		if err != nil {
			b.Errorf("%v", err)
		}
	}
}
