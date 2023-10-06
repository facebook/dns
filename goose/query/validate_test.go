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

package query

import (
	"testing"

	"github.com/miekg/dns"
	"github.com/stretchr/testify/require"
)

func A(rr string) *dns.A {
	r, _ := dns.NewRR(rr)
	return r.(*dns.A)
}

func AAAA(rr string) *dns.AAAA {
	r, _ := dns.NewRR(rr)
	return r.(*dns.AAAA)
}

func SOA(rr string) *dns.SOA {
	r, _ := dns.NewRR(rr)
	return r.(*dns.SOA)
}

func msgSingleARecord() *dns.Msg {
	return &dns.Msg{
		Answer: []dns.RR{A(
			"a.test.miek.nl.	1800	IN	A	139.162.196.78",
		)},
	}
}

func msgEmpty() *dns.Msg {
	return &dns.Msg{
		Ns: []dns.RR{SOA(
			"miek.nl.	1800	IN	SOA	ns.miek.nl. dnsmaster.miek.nl. 2017100301 200 100 604800 3600",
		)},
	}
}

func Test_CheckResponse(t *testing.T) {

	w := CheckResponse(msgSingleARecord())
	require.Nil(t, w)

	x := CheckResponse(msgEmpty())
	require.NotNil(t, x)

}
