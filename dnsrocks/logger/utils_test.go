package logger

import (
	"testing"

	"github.com/miekg/dns"
	"github.com/stretchr/testify/require"
)

// TestCollectDNSFlags tests CollectDNSFlags sets the right value w.r.t. each flag
func TestCollectDNSFlags(t *testing.T) {
	m := new(dns.Msg)
	require.Equal(t, "", CollectDNSFlags(m))

	m.Response = true
	require.Contains(t, CollectDNSFlags(m), "QR")

	m.Authoritative = true
	require.Contains(t, CollectDNSFlags(m), "AA")

	m.Truncated = true
	require.Contains(t, CollectDNSFlags(m), "TC")

	m.RecursionDesired = true
	require.Contains(t, CollectDNSFlags(m), "RD")

	m.RecursionAvailable = true
	require.Contains(t, CollectDNSFlags(m), "RA")

	m.Zero = true
	require.Contains(t, CollectDNSFlags(m), "Z")

	m.AuthenticatedData = true
	require.Contains(t, CollectDNSFlags(m), "AD")

	m.CheckingDisabled = true
	require.Contains(t, CollectDNSFlags(m), "CD")

	flagsBefore := CollectDNSFlags(m)
	m.Opcode = 5
	m.Rcode = 3
	flagsAfter := CollectDNSFlags(m)
	require.Equal(t, flagsBefore, flagsAfter)
}
