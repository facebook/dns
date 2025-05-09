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
	"bytes"
	"strconv"
	"unicode/utf8"
)

// Bunquote will unquote bytes using rules described in https://tools.ietf.org/html/rfc1035#section-5.1
func Bunquote(b []byte) ([]byte, error) {
	if len(b) == 0 {
		return b, nil
	}

	if !bytes.ContainsRune(b, '\\') {
		return b, nil
	}
	var runeTmp [utf8.UTFMax]byte
	buf := make([]byte, 0, 3*len(b)/2) // Try to avoid more allocations.
	s := string(b)
	for len(s) > 0 {
		c, multibyte, ss, err := strconv.UnquoteChar(s, 0)
		if err != nil {
			return b, err
		}
		s = ss
		if c < utf8.RuneSelf || !multibyte {
			buf = append(buf, byte(c))
		} else {
			n := utf8.EncodeRune(runeTmp[:], c)
			buf = append(buf, runeTmp[:n]...)
		}
	}
	return buf, nil
}

// Unquote is Bunquote for strings
func Unquote(s string) (string, error) {
	r, err := Bunquote([]byte(s))
	return string(r), err
}

// Bquote will quote byte slice
func Bquote(b []byte) []byte {
	s := []byte(strconv.Quote(string(b[:])))
	// ',' is a field separator, so we should always encode it
	if bytes.ContainsRune(s, ',') {
		s = bytes.ReplaceAll(s, []byte(","), []byte("\\054"))
	}
	// ':' is a legacy field separator, encode it as well
	if bytes.ContainsRune(s, ':') {
		s = bytes.ReplaceAll(s, []byte(":"), []byte("\\072"))
	}
	if len(s) < 2 {
		return b
	}
	s = s[1 : len(s)-1] // without the quotes around
	// we don't need quote to be escaped
	s = bytes.ReplaceAll(s, []byte(`\"`), []byte(`"`))
	return s
}
