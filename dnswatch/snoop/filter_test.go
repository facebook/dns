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

	"github.com/google/gopacket/afpacket"
	"github.com/stretchr/testify/require"
)

func copySlice(data []byte) []byte {
	dest := make([]byte, len(data))
	copy(dest, data)
	return dest
}

func TestComputeRingSizes(t *testing.T) {
	ringSize := 10 * 1024 * 1024
	pageSize := 4 * 1024
	snapLen := 65535

	frameSize, blockSize, numBlocks, err := computeRingSizes(ringSize, snapLen, pageSize)
	require.Nil(t, err)
	require.Equal(t, 65536, frameSize)
	require.Equal(t, 65536*afpacket.DefaultNumBlocks, blockSize)
	require.Equal(t, 10*1024*1024/(65536*afpacket.DefaultNumBlocks), numBlocks)

	ringSize = 10 * 1024
	pageSize = 4 * 1024
	snapLen = 65535

	_, _, _, err = computeRingSizes(ringSize, snapLen, pageSize)
	require.NotNil(t, err)

	require.Equal(t, 10*1024*1024, MBtoB(10))
	require.Equal(t, 23*1024*1024, MBtoB(23))
}

func TestSetBPFFilter(t *testing.T) {
	frameSize := 65536

	rule := "random rule"
	err := SetBPFFilter(nil, rule, frameSize)
	require.Contains(t, err.Error(), "filter expression: syntax error")
	require.NotNil(t, err)
}

func TestDeepCopyDNS(t *testing.T) {
	d, err := RawDecoderByType("dns")
	require.Nil(t, err)

	err = d.Unmarshal(copySlice(dnsRawResponseIPv4))

	shallowCopy := d.(*DNSDecoder).dns
	deepCopy := deepCopyDNS(d.(*DNSDecoder).dns)

	require.Nil(t, err)
	require.Equal(t, true, d.(*DNSDecoder).dns.QR)
	require.Equal(t, "github.com", string(d.(*DNSDecoder).dns.Questions[0].Name))
	require.Equal(t, "github.com", string(shallowCopy.Questions[0].Name))
	require.Equal(t, "github.com", string(deepCopy.Questions[0].Name))
	require.Equal(t, "140.82.121.3", d.(*DNSDecoder).dns.Answers[0].IP.String())
	require.Equal(t, "140.82.121.3", shallowCopy.Answers[0].IP.String())
	require.Equal(t, "140.82.121.3", deepCopy.Answers[0].IP.String())

	d.(*DNSDecoder).dns.Questions[0].Name = []byte("random.com")
	d.(*DNSDecoder).dns.Answers[0].IP[3] = 10

	require.Equal(t, "random.com", string(d.(*DNSDecoder).dns.Questions[0].Name))
	require.Equal(t, "random.com", string(shallowCopy.Questions[0].Name))
	require.Equal(t, "github.com", string(deepCopy.Questions[0].Name))
	require.Equal(t, "140.82.121.10", d.(*DNSDecoder).dns.Answers[0].IP.String())
	require.Equal(t, "140.82.121.10", shallowCopy.Answers[0].IP.String())
	require.Equal(t, "140.82.121.3", deepCopy.Answers[0].IP.String())
}
