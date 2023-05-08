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
	"net"
	"strings"
	"testing"

	"github.com/google/gopacket/layers"
	"github.com/stretchr/testify/require"
)

func TestDisplayInfoString(t *testing.T) {
	d := &DisplayInfo{
		fields: []FieldID{FieldTYPE, FieldQNAME, FieldRIP, FieldRCODE, FieldRTIME},
	}
	res := strings.Fields(d.String())
	require.Equal(t, 5, len(res))
	require.Equal(t, "UNK", res[0])
	require.Equal(t, "UNK", res[1])
	require.Equal(t, "UNK", res[2])
	require.Equal(t, "UNK", res[3])
	require.Equal(t, "UNK", res[4])

	d.query = &layers.DNS{}
	d.response = &layers.DNS{}
	d.query.Questions = append(d.query.Questions, layers.DNSQuestion{
		Name: []byte("question.name"),
		Type: layers.DNSType(uint16(1)),
	})
	d.response.ResponseCode = layers.DNSResponseCode(uint8(3))
	d.response.Answers = append(d.response.Answers, layers.DNSResourceRecord{
		Name: []byte("question.name"),
		Type: layers.DNSType(uint16(1)),
		IP:   net.IP([]byte{111, 111, 11, 1}),
	})

	res = strings.Fields(d.String())
	require.Equal(t, "A", res[0])
	require.Equal(t, "question.name", res[1])
	require.Equal(t, "111.111.11.1", res[2])
	require.Equal(t, "NXDOMAIN", res[3])

	d.fields = []FieldID{FieldPNAME, FieldLAT, FieldTYPE, FieldQNAME, FieldRIP, FieldRCODE}
	res = strings.Fields(d.String())
	require.Equal(t, 6, len(res))
	require.Equal(t, "UNK", res[0])
	require.Equal(t, "UNK", res[1])
	require.Equal(t, "A", res[2])
	require.Equal(t, "question.name", res[3])
	require.Equal(t, "111.111.11.1", res[4])
	require.Equal(t, "NXDOMAIN", res[5])

	d.pid = 100
	d.pName = "random"
	d.qTimestamp = 100000
	d.rTimestamp = 110000
	d.response.Answers = nil
	d.query.Questions = nil
	d.response.ResponseCode = layers.DNSResponseCode(uint8(0))
	d.query.Questions = append(d.query.Questions, layers.DNSQuestion{
		Name: []byte("questionv6.name"),
		Type: layers.DNSType(uint16(28)),
	})
	d.response.Answers = append(d.response.Answers, layers.DNSResourceRecord{
		Name: []byte("questionv6.name"),
		Type: layers.DNSType(uint16(28)),
		IP:   net.ParseIP("2401:db00:31ff:ff47:face:b00c:0:6cd2"),
	})
	res = strings.Fields(d.String())
	require.Equal(t, 6, len(res))
	require.Equal(t, "random", res[0])
	require.Equal(t, "10", res[1])
	require.Equal(t, "AAAA", res[2])
	require.Equal(t, "questionv6.name", res[3])
	require.Equal(t, "2401:db00:31ff:ff47:face:b00c:0:6cd2", res[4])
	require.Equal(t, "NOERROR", res[5])

	d.pid = 0
	res = strings.Fields(d.DetailedString())
	require.Equal(t, "UNK,", res[2])
	require.Equal(t, "UNK", res[5])

	d.pid = 10
	d.pName = "V2Random"
	d.qTimestamp = 100000
	d.rTimestamp = 110000
	d.query = &layers.DNS{}
	d.response = &layers.DNS{}
	d.query.Questions = append(d.query.Questions, layers.DNSQuestion{
		Name: []byte("question.name"),
		Type: layers.DNSType(uint16(1)),
	})
	d.response.Questions = append(d.response.Questions, layers.DNSQuestion{
		Name: []byte("question.name"),
		Type: layers.DNSType(uint16(1)),
	})
	d.response.ResponseCode = layers.DNSResponseCode(uint8(3))
	d.response.Answers = append(d.response.Answers, layers.DNSResourceRecord{
		Name: []byte("question.name"),
		Type: layers.DNSType(uint16(1)),
	})
	res = strings.Fields(d.DetailedString())
	require.Equal(t, "query:", res[7])
	require.Equal(t, "true,", res[8])
	require.Equal(t, "V2Random", res[5])
	require.Equal(t, "10", res[12])
	require.Equal(t, "QUERY:", res[23])
	require.Equal(t, "1,", res[24])
	require.Equal(t, "ANSWER:", res[25])
	require.Equal(t, "1,", res[26])
	require.Equal(t, "AUTHORITY:", res[27])
	require.Equal(t, "0,", res[28])
	require.Equal(t, "ADDITIONAL:", res[29])
	require.Equal(t, "0", res[30])
	require.Equal(t, ";question.name", res[34])

	d.query = nil
	res = strings.Fields(d.DetailedString())
	require.Equal(t, "query:", res[7])
	require.Equal(t, "false,", res[8])
}
