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
	"encoding/binary"
	"fmt"
	"sort"
)

type paramNum uint16

const (
	mandatory     paramNum = 0
	alpn          paramNum = 1
	nodefaultalpn paramNum = 2
	port          paramNum = 3
	ipv4hint      paramNum = 4
	ech           paramNum = 5
	ipv6hint      paramNum = 6
)

var (
	paramNumToStr = map[paramNum]string{
		mandatory:     "mandatory",
		alpn:          "alpn",
		nodefaultalpn: "no-default-alpn",
		port:          "port",
		ipv4hint:      "ipv4hint",
		ech:           "ech",
		ipv6hint:      "ipv6hint",
	}
	strToParamNum = reverseParamNumToStr()
)

// svcb allows multiple values for a key. Therefore there
// needs to be a delimiter for the values.
var (
	// valueDelimInternal is the delimiter we use to split the multiple
	// values in a key-value pair in svcb svcparams. Only for FB DNS pipeline
	valueDelimInternal = []byte("|")
	// paramDelim is the delimiter we used to split SvcParams
	paramDelim = []byte(";")
)

var (
	// kvSeparator is the "=" between key and values.
	kvSeparator = []byte("=")
)

var (
	// valueMarshallers maps the human readable
	// key names with their marshaller functions
	valueMarshallers = map[paramNum]valueMarshaller{
		mandatory:     mandatoryMarshaller,
		alpn:          alpnMarshaller,
		ipv4hint:      ipv4hintMarshaller,
		ipv6hint:      ipv6hintMarshaller,
		ech:           echMarshaller,
		port:          portMarshaller,
		nodefaultalpn: nodefaultalpnMarshaller,
	}

	// valueUnmarshallers maps the human readable
	// key names with their unmarshaller functions
	valueUnmarshallers = map[paramNum]valueUnmarshaller{
		mandatory:     mandatoryUnmarshaller,
		alpn:          alpnUnmarshaller,
		ipv4hint:      ipv4hintUnmarshaller,
		ipv6hint:      ipv6hintUnmarshaller,
		ech:           echUnmarshaller,
		port:          portUnmarshaller,
		nodefaultalpn: nodefaultalpnUnmarshaller,
	}
)

// param represents a svcb parameter with a `keynum` and `value`
// the value stored in this type is in wire format
type param struct {
	keynum paramNum
	value  []byte
}

// reverse the paramNumToStr map
func reverseParamNumToStr() map[string]paramNum {
	m := make(map[string]paramNum)
	for k, v := range paramNumToStr {
		m[v] = k
	}
	return m
}

// fromText parses the `text` (should be in TinyDNS format)
// and saves the wire format of the svcparamkey and svcparamvalue
// in param key and value
func (p *param) fromText(text []byte) error {
	parsed := bytes.SplitN(text, kvSeparator, 2)
	if len(parsed) != 2 {
		return fmt.Errorf("error parsing SVCB/HTTPS parameter: %s", text)
	}
	k, v := parsed[0], parsed[1]

	knum, ok := strToParamNum[string(k)]
	if !ok {
		return fmt.Errorf("unknown SVCB/HTTPS parameter: %s", text)
	}

	if knum != nodefaultalpn && len(v) == 0 {
		return fmt.Errorf("value for %s cannot be empty", k)
	}

	// if we can find knum, we should be able to
	// find its value marshaller
	data, err := valueMarshallers[knum](bytes.Trim(v, "\""))
	if err != nil {
		return err
	}

	p.value = data
	p.keynum = knum
	return nil
}

// toText outputs the human-readable TinyDNS format of param (key=value...)
// since the data saved by param are processed, we should
// trust them and therefore toText won't return error
func (p *param) toText(buf *bytes.Buffer) {
	buf.WriteString(paramNumToStr[p.keynum])
	buf.Write(kvSeparator)

	printer := valueUnmarshallers[p.keynum]
	buf.WriteByte('"')
	printer(p.value, buf)
	buf.WriteByte('"')
}

func (p *param) toWire(buf *bytes.Buffer) error {
	// 2 bytes for the key (uint16)
	// 2 bytes for the length-prefix of the value
	// value bytes
	buf.Grow(2 + 2 + len(p.value))

	err := binary.Write(buf, binary.BigEndian, uint16(p.keynum))
	if err != nil {
		return err
	}
	err = binary.Write(buf, binary.BigEndian, uint16(len(p.value)))
	if err != nil {
		return err
	}
	_, err = buf.Write(p.value)
	if err != nil {
		return err
	}
	return nil
}

// ParamList represents a list of key-value pairs
// for a SVCB RR. Exposing this should be enough
type ParamList []param

// FromText fills ParamList with a list of
// TinyDNS fields
func (l *ParamList) FromText(rawparams []byte) error {
	// seen maps paramkeys and their indices in the parameter
	// list. This is very useful because we use it to check:
	//     a. whether the keys in this list are unique
	//     b. the mandatory keys are all available in this list
	text := bytes.Split(rawparams, paramDelim)
	seen := make(map[paramNum]int)
	for idx := 0; idx < len(text) && len(text[idx]) > 0; idx++ {
		p := param{}
		err := p.fromText(text[idx])
		if err != nil {
			return err
		}
		_, presence := seen[p.keynum]
		if presence {
			return fmt.Errorf("error parsing %s: keys have to be unique", text[idx])
		}
		seen[p.keynum] = idx
		*l = append(*l, p)
	}

	// verify if the keys specified in mandatory are
	// all available
	mandatoryidx, hasmandatory := seen[mandatory]
	if hasmandatory {
		mandatorylist := (*l)[mandatoryidx].value
		// every 2 bytes represents a key
		for bindex := 0; bindex < len(mandatorylist); bindex += 2 {
			key := binary.BigEndian.Uint16(mandatorylist[bindex : bindex+2])
			_, haskey := seen[paramNum(key)]
			if !haskey {
				// we can directly query the map paramNumToStr
				// because this key, although is missing in the list,
				// is at least a valid SVCB parameter key (has processed
				// by fromText and mandatoryMarshaller)
				return fmt.Errorf("%s is mandatory but missing in parameter list", paramNumToStr[paramNum(key)])
			}
		}
	}

	// note that the svcparams have to be sorted
	// otherwise the RR will be considered as malformed
	sort.SliceStable(*l, func(i, j int) bool {
		return (*l)[i].keynum < (*l)[j].keynum
	})
	return nil
}

// ToText outputs the TinyDNS-like format of ParamList to
// `out`
func (l *ParamList) ToText(out *bytes.Buffer) {
	for idx, p := range *l {
		if idx != 0 {
			out.Write(paramDelim)
		}
		p.toText(out)
	}
}

// ToWire outputs the wire format of ParamList to
// `out`
func (l *ParamList) ToWire(out *bytes.Buffer) error {
	for _, p := range *l {
		err := p.toWire(out)
		if err != nil {
			return err
		}
	}
	return nil
}
