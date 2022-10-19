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
	"os"
	"testing"
)

func TestDeriveSerial(t *testing.T) {
	tmpfile, err := os.CreateTemp("", "testserial")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name()) // clean up
	serial, err := DeriveSerial(tmpfile)
	if err != nil {
		t.Fatal(err)
	}
	if serial == 0 {
		t.Error("expected non-zero serial, got 0")
	}
}
