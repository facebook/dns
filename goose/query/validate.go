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
