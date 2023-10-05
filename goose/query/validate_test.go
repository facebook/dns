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
