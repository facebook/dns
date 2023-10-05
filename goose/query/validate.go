package query

import (
	"fmt"
	"github.com/miekg/dns"
)

// ValidateResponse returns error if DNS response not valid
type ValidateResponse func(response *dns.Msg) error

// CheckResponse - very simple - just checks length of reply for now
// TODO pcullen make this better
func CheckResponse(response *dns.Msg) error {
	if len(response.Answer) != 1 {
		return fmt.Errorf("DNS request: no reply received")
	}
	return nil
}
