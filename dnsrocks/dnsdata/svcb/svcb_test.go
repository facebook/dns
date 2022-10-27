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
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testCase struct {
	input []byte
	text  []byte
	wire  []byte
}

// all the test cases below are from the RFC draft
// https://datatracker.ietf.org/doc/html/draft-ietf-dnsop-svcb-https-06
// Appendix D
// some of the input are quoted with "" and some are not. svcb can handle
// both cases. However, the text output from svcb package is always quoted with
// ""
var paramTestCases = []testCase{
	{
		input: []byte("ipv4hint=192.0.2.1|1.2.3.4"),
		text:  []byte("ipv4hint=\"192.0.2.1|1.2.3.4\""),
		wire: []byte{
			0, 4, // key type ipv4hint
			0, 8, // length 4
			0xc0, 0, 2, 1, // 192.0.2.1
			1, 2, 3, 4, // 1.2.3.4
		},
	},
	{
		input: []byte("ipv6hint=::ffff:198.51.100.100"),
		// due to the implementation of Go's standard net package,
		// we cannot recover the original IPv6 dual form. But luckily
		// we have ipv6hint, so we know that it used to have leading ::ffff
		text: []byte("ipv6hint=\"198.51.100.100\""),
		wire: []byte{
			0, 6, // key type ipv6hint
			0, 16, // length 16
			0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0xff, 0xff, 0xc6, 0x33, 0x64, 0x64, // ::ffff:198.51.100.100
		},
	},
	{
		input: []byte("ipv6hint=\"2001:db8::1|2001:db8::53:1\""),
		text:  []byte("ipv6hint=\"2001:db8::1|2001:db8::53:1\""),
		wire: []byte{
			0, 6, // key type ipv6hint
			0, 32, // length 32 (two IPv6 addresses)
			0x20, 0x01, 0x0d, 0xb8, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, // 2001:db8::1
			0x20, 0x01, 0x0d, 0xb8, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0x53, 0, 1, // 2001:db8::53:1
		},
	},
	{
		input: []byte("mandatory=ipv4hint|alpn"),     // note that the values are unsorted
		text:  []byte("mandatory=\"alpn|ipv4hint\""), // note that the values are sorted now
		wire: []byte{
			0, 0, // key type mandatory
			0, 4, // length 4
			0, 1, // alpn
			0, 4, // ipv4hint
		}, // sorted
	},
	{
		input: []byte("alpn=h2|h3-19"),
		text:  []byte("alpn=\"h2|h3-19\""),
		wire: []byte{
			0, 1, // key type alpn
			0, 9, // length 9
			2,          // alpn len = 2
			0x68, 0x32, // h2
			5,                            // alpn len = 5
			0x68, 0x33, 0x2d, 0x31, 0x39, // h3-19
		},
	},
	{
		input: []byte("port=53"),
		text:  []byte("port=\"53\""),
		wire: []byte{
			0, 3, // key type port
			0, 2, // length = 2
			0, 0x35, // port 53
		},
	},
	{
		// this test case is not from RFC
		// note that the ech base64 includes two "="s for padding
		input: []byte("echconfig=\"dHJhZmZpYw==\""),
		text:  []byte("echconfig=\"dHJhZmZpYw==\""),
		wire: []byte{
			0, 5, // key type echconfig
			0, 7, // length = 7
			't', 'r', 'a', 'f', 'f', 'i', 'c', // the ech public key is "traffic"
		},
	},
}

var paramListTestCases = []testCase{
	{
		input: []byte("ipv4hint=192.0.2.1;mandatory=ipv4hint|alpn;alpn=h3-29|h2"),
		text:  []byte("mandatory=\"alpn|ipv4hint\";alpn=\"h3-29|h2\";ipv4hint=\"192.0.2.1\""),
		wire: []byte{
			0, 0, // key type mandatory
			0, 4, // length 4
			0, 1, // alpn
			0, 4, // ipv4hint
			0, 1, // key type alpn
			0, 9, // param value length 9
			5,                       // first alpn size=5
			'h', '3', '-', '2', '9', // h3-29
			2,        // second alpn size=2
			'h', '2', // h2
			0, 4, // key 4
			0, 4, // length 4
			0xc0, 0, 2, 1, // 192.0.2.1
		},
	},
	{
		input: []byte("port=8080;no-default-alpn="),
		text:  []byte("no-default-alpn=\"\";port=\"8080\""),
		wire: []byte{
			0, 2, // key type no default alpn
			0, 0, // param length = 0
			0, 3, // key type port
			0, 2, // param length = 2
			0x1f, 0x90, // port 8080
		},
	},
}

var badStrParams = [][]byte{
	// the following 6 svc params have
	// empty values
	[]byte("mandatory="),
	[]byte("alpn="),
	[]byte("ipv4hint="),
	[]byte("echconfig="),
	[]byte("ipv6hint="),
	[]byte("port="),
	[]byte("no-default-alpn=h2"),       // no-default-alpn should have no value
	[]byte("ipv4hint"),                 // no "=" and values
	[]byte("foo=bar"),                  // non-RFC keys
	[]byte("port=1b"),                  // port has to be a decimal number
	[]byte("ipv4hint=ab.cd.ef.fg"),     // not a valid IPv4 address
	[]byte("IPv4Hint=1.2.3.4"),         // key has to be in lower case
	[]byte("ipv4hint=face:b00c::"),     // use IPv6 address in ipv4hint
	[]byte("echconfig=***bad***"),      // not a valid base64 encoded str
	[]byte("mandatory=foo|bar"),        // invalid keys in mandatory
	[]byte("mandatory=alpn|alpn"),      // values in mandatory have to be unique
	[]byte("mandatory=ALPN|IPv4Hint"),  // values in mandatory have to be lowercased (just like the param keys)
	[]byte("mandatory=mandatory"),      // mandatory cannot points to itself
	[]byte("ipv6hint=f:a:c:e:b:o:o:k"), // not a valid IPv6 address
	[]byte("ipv6hint=1.2.3.4"),         // use IPv4 address in ipv6hint
}

var badTinyDNSRecords = [][]byte{
	[]byte("no-default-alpn=h2;port=53;ipv4hint=1.2.3.4"),
	[]byte("no-default-alpn=;echconfig=\"dHJhZmZpYw==\";port=x"),
	[]byte("mandatory=echconfig;ipv4hint=facebook"),
	[]byte("ipv4hint=1.2.3.4;ipv4hint=2.3.4.5"), // keys are not unique (should aggregate)
	[]byte("mandatory=ipv4hint|alpn;alpn=h2"),   // a mandatory key is missing
	[]byte("port=8080,no-default-alpn="),        // didn't use ; to split svcparams
}

func TestParam(t *testing.T) {
	for _, testcase := range paramTestCases {
		kv := param{}
		err := kv.fromText(testcase.input)
		require.Nil(t, err)

		var wireout bytes.Buffer
		err = kv.toWire(&wireout)
		require.NoError(t, err)

		if !reflect.DeepEqual(wireout.Bytes(), testcase.wire) {
			t.Fatalf(
				"wire format differs [input: %s] -> output: %v, expect: %v",
				testcase.input,
				wireout.Bytes(),
				testcase.wire,
			)
		}

		var textout bytes.Buffer
		kv.toText(&textout)
		if !reflect.DeepEqual(textout.Bytes(), testcase.text) {
			t.Fatalf(
				"text format differs [input: %s] -> output: %s, expect: %s",
				testcase.input,
				textout.Bytes(),
				testcase.text,
			)
		}
	}
}

func TestParamList(t *testing.T) {
	for _, testcase := range paramListTestCases {
		l := ParamList{}
		err := l.FromText(testcase.input)
		require.Nil(t, err)

		var wireout bytes.Buffer
		err = l.ToWire(&wireout)
		require.NoError(t, err)
		if !reflect.DeepEqual(wireout.Bytes(), testcase.wire) {
			t.Fatalf(
				"wire format differs [input: %s] -> output: %v, expect: %v",
				testcase.input,
				wireout.Bytes(),
				testcase.wire,
			)
		}

		var textout bytes.Buffer
		l.ToText(&textout)
		if !reflect.DeepEqual(textout.Bytes(), testcase.text) {
			t.Fatalf(
				"text format differs [input: %s] -> output: %s, expect: %s",
				testcase.input,
				textout.Bytes(),
				testcase.text,
			)
		}
	}
}

func TestBadParam(t *testing.T) {
	for _, bad := range badStrParams {
		kv := param{}
		err := kv.fromText(bad)

		assert.Error(t, err, "%s should be an invalid parameter", bad)
	}
}

func TestBadParamList(t *testing.T) {
	for _, bad := range badTinyDNSRecords {
		l := ParamList{}
		err := l.FromText(bad)

		assert.Error(t, err, "%s should be an invalid SVCB/HTTPS TinyDNS record", bad)
	}
}
