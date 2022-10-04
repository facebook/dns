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

package svcb

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"net"
	"strconv"
)

// this file should only include value unmarshallers for
// SVCB SvcParams

// a value unmarshaller takes a wire format data, and writes
// the human readable output to the bytes buffer. Since
// the wire format data are usually parsed and preprocessed,
// they are trusted and therefore unmarshaller doesn't return error

// **value unmarshaller may deal with multiple values in one call**
type valueUnmarshaller = func([]byte, *bytes.Buffer)

func mandatoryUnmarshaller(input []byte, out *bytes.Buffer) {
	for offset := 0; offset < len(input); offset += 2 {
		if offset != 0 {
			out.Write(valueDelimInternal)
		}
		rawnum := binary.BigEndian.Uint16(input[offset : offset+2])
		out.WriteString(paramNumToStr[paramNum(rawnum)])
	}
}

func alpnUnmarshaller(input []byte, out *bytes.Buffer) {
	offset := 0
	for offset < len(input) {
		if offset != 0 {
			out.Write(valueDelimInternal)
		}
		l := int(input[offset])
		offset++
		out.Write(input[offset : offset+l])
		offset += l
	}
}

// no-default-alpn should print no value
func nodefaultalpnUnmarshaller(input []byte, out *bytes.Buffer) {
}

func portUnmarshaller(input []byte, out *bytes.Buffer) {
	port := binary.BigEndian.Uint16(input)
	out.WriteString(
		strconv.FormatUint(uint64(port), 10),
	)
}

func ipv4hintUnmarshaller(input []byte, out *bytes.Buffer) {
	for offset := 0; offset < len(input); offset += net.IPv4len {
		if offset != 0 {
			out.Write(valueDelimInternal)
		}
		out.WriteString(net.IP(input[offset : offset+net.IPv4len]).To4().String())
	}
}

func echUnmarshaller(input []byte, out *bytes.Buffer) {
	output := make([]byte, base64.StdEncoding.EncodedLen(len(input)))
	base64.StdEncoding.Encode(output, input)
	out.Write(output)
}

func ipv6hintUnmarshaller(input []byte, out *bytes.Buffer) {
	for offset := 0; offset < len(input); offset += net.IPv6len {
		if offset != 0 {
			out.Write(valueDelimInternal)
		}
		out.WriteString(net.IP(input[offset : offset+net.IPv6len]).To16().String())
	}
}
