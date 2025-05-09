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

package svcb

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"sort"
	"strconv"
)

// this file should only include value marshallers for
// SVCB SvcParams

// a value marshaller takes a human readable inputs,
// and returns the wire format of this input, and whether
// this marshaller encounters any error when parsing.

// **value marshallers may deal with multiple values, and thus need to parse the values first**
// Some svcparams do not support multi values, see this table:
// +-----------------+------------------+
// | SvcParam        | Multiple Values? |
// +-----------------+------------------+
// | mandatory       | Yes              |
// +-----------------+------------------+
// | alpn            | Yes              |
// +-----------------+------------------+
// | no-default-alpn | No               |
// +-----------------+------------------+
// | port            | No               |
// +-----------------+------------------+
// | ipv4hint        | Yes              |
// +-----------------+------------------+
// | ech             | No               |
// +-----------------+------------------+
// | ipv6hint        | Yes              |
// +-----------------+------------------+
type valueMarshaller = func([]byte) ([]byte, error)

func mandatoryMarshaller(input []byte) ([]byte, error) {
	values := bytes.Split(input, valueDelimInternal)
	sort.SliceStable(values, func(i, j int) bool {
		inum := strToParamNum[string(values[i])]
		jnum := strToParamNum[string(values[j])]
		return uint16(inum) < uint16(jnum)
	})

	var buf bytes.Buffer
	buf.Grow(len(values) * 2)

	seen := make(map[paramNum]struct{})

	for _, value := range values {
		knum, ok := strToParamNum[string(value)]
		if !ok {
			return nil, fmt.Errorf("%s is not a valid mandatory value", input)
		} else if knum == mandatory {
			return nil, errors.New("mandatory itself cannot be mandatory")
		}
		_, haveseen := seen[knum]
		if haveseen {
			return nil, fmt.Errorf("%s in mandatory values has appeared more than once", value)
		}
		seen[knum] = struct{}{}
		err := binary.Write(&buf, binary.BigEndian, uint16(knum))
		if err != nil {
			return nil, err
		}
	}
	return buf.Bytes(), nil
}

// nolint:unparam
func alpnMarshaller(input []byte) ([]byte, error) {
	alpns := bytes.Split(input, valueDelimInternal)

	var buf bytes.Buffer
	buf.Grow(len(input) + len(alpns))

	for _, alpn := range alpns {
		buf.WriteByte(byte(len(alpn)))
		buf.Write(alpn)
	}
	return buf.Bytes(), nil
}

// no-default-alpn should have empty value
// nolint:unparam
func nodefaultalpnMarshaller(input []byte) ([]byte, error) {
	if len(input) > 0 {
		return nil, errors.New("the value for no-default-alpn should be empty")
	}
	return nil, nil
}

// port needs to be in big endian
func portMarshaller(input []byte) ([]byte, error) {
	port, err := strconv.ParseUint(string(input), 10, 16)
	if err != nil {
		return nil, err
	}
	buf := make([]byte, 2)
	binary.BigEndian.PutUint16(buf, uint16(port))
	return buf, nil
}

func ipv4hintMarshaller(input []byte) ([]byte, error) {
	ipv4s := bytes.Split(input, valueDelimInternal)
	var buf bytes.Buffer
	buf.Grow(len(ipv4s) * net.IPv4len)

	for _, ipv4 := range ipv4s {
		v4 := net.ParseIP(string(ipv4))
		if v4 == nil {
			return nil, fmt.Errorf("%s is not a valid IPv4 address", ipv4)
		}
		v4 = v4.To4()
		if v4 == nil {
			return nil, fmt.Errorf("%s is a valid address but cannot be converted to 4-byte form", ipv4)
		}
		buf.Write([]byte(v4))
	}
	return buf.Bytes(), nil
}

func echMarshaller(input []byte) ([]byte, error) {
	output := make([]byte, base64.StdEncoding.DecodedLen(len(input)))
	size, err := base64.StdEncoding.Decode(output, input)
	if err != nil {
		return nil, err
	}
	return output[:size], nil
}

func ipv6hintMarshaller(input []byte) ([]byte, error) {
	ipv6s := bytes.Split(input, valueDelimInternal)

	var buf bytes.Buffer
	buf.Grow(len(ipv6s) * net.IPv6len)

	for _, ipv6 := range ipv6s {
		// this "if" ensures that we can recognize the IPv4-mapped v6 address
		// but cannot guarantee that `ipv6` is 100% a valid IPv6 address (needs to see ParseIP)
		if !bytes.Contains(ipv6, []byte(":")) {
			return nil, fmt.Errorf("%s is not a valid IPv6 address", ipv6)
		}
		v6 := net.ParseIP(string(ipv6))
		if v6 == nil {
			return nil, fmt.Errorf("%s is not a parsable IPv6 address", ipv6)
		}
		// if ParseIP above returns a real value, v6 must be a valid 16 bytes IPv6 address
		buf.Write([]byte(v6))
	}
	return buf.Bytes(), nil
}
