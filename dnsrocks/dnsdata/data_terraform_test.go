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

package dnsdata

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTerraformFormatting(t *testing.T) {
	type testCase struct {
		input         string
		expectedType  string
		expectedValue string
	}

	testCases := []testCase{
		{
			input:         "+test.com,1.2.3.5,3600",
			expectedType:  "A",
			expectedValue: "1.2.3.5",
		},
		{
			input:         "+test.net,fd0a:14f5:dead:beef:1::36,3600",
			expectedType:  "AAAA",
			expectedValue: "fd0a:14f5:dead:beef:1::36",
		},
		{
			input:         "&subzone.test.com,,a.ns.subzone.test.com,3600",
			expectedType:  "NS",
			expectedValue: "a.ns.subzone.test.com",
		},
		{
			input:         "Cwww.test.com,test.com,3600",
			expectedType:  "CNAME",
			expectedValue: "test.com",
		},
		{
			input:         "^168.192.in-addr.arpa,some.host.net,86400,,",
			expectedType:  "PTR",
			expectedValue: "some.host.net",
		},
		{
			input:         "@test.com,,mail.test.com,0,86400,,",
			expectedType:  "MX",
			expectedValue: "0 mail.test.com",
		},
		{
			input:         "'test.com,some text,3600",
			expectedType:  "TXT",
			expectedValue: "some text",
		},
		{
			input:         "Stest.com,,db.test.com,10,0,443,86400,,",
			expectedType:  "SRV",
			expectedValue: "0 443 10 db.test.com",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.input, func(t *testing.T) {
			codec := new(Codec)
			codec.Acc.NoPrefixSets = true
			codec.NoRnetOutput = true

			record, err := codec.decodeRecord([]byte(tc.input))
			require.Nil(t, err)

			if compositeRecord, ok := record.(CompositeRecord); ok {
				records := compositeRecord.DerivedRecords()

				// for the purpose of this test we consider only single mapped records
				require.Len(t, records, 1)
				record = records[0]
			}

			terraformRecord := record.(TerraformRecord)

			terraformType, err := WireTypeToTerraformString(terraformRecord.WireType())
			require.Nil(t, err)
			require.Equal(t, tc.expectedType, terraformType)

			terraformValue, err := terraformRecord.TerraformValue()
			require.Nil(t, err)
			require.Equal(t, tc.expectedValue, terraformValue)
		})
	}
}
