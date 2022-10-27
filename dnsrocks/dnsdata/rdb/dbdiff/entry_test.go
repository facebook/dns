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

package dbdiff

import (
	"testing"

	"github.com/facebookincubator/dns/dnsrocks/dnsdata"
)

func TestConv(t *testing.T) {
	convTests := []struct {
		in       string
		nout     int
		errParse error
		errConv  error
	}{
		{in: "+&example.org,,b.ns.example.org,172800", nout: 1},
		{in: "-&example.org,1.1.1.1,a.ns.example.org,172800", nout: 2},
		{in: "badDiff", errParse: ErrBadOp},
		{in: "+badRecord", errConv: dnsdata.ErrBadRType},
	}
	codec := new(dnsdata.Codec)
	for _, tc := range convTests {
		e := Entry{}
		if err := e.Parse(tc.in); err != tc.errParse {
			t.Errorf("parse: expected error '%v', got '%v' for input '%v'", tc.errParse, err, tc.in)
			continue
		} else if err != nil {
			continue
		}
		if err := e.Convert(codec); err != tc.errConv {
			t.Errorf("convert: expected error '%v', got '%v' for input '%v'", tc.errConv, err, tc.in)
			continue
		} else if err != nil {
			continue
		}
		if nout := len(e.Records); nout != tc.nout {
			t.Errorf("expected out '%v', got '%v' for input '%v'", tc.nout, nout, tc.in)
		}
	}
}
