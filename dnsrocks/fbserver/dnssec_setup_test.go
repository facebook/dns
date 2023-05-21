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

package fbserver

import (
	"github.com/facebookincubator/dns/dnsrocks/testutils"

	"github.com/stretchr/testify/require"

	"testing"
)

const KEYNAME = "Kexample.com.+013+28484"

func TestParseKey(t *testing.T) {
	keys, capacity, splitkeys, err := dnssecParse([]string{testutils.FixturePath("../testdata/data/", KEYNAME)})
	require.Nilf(t, err, "Failed to parse key %s: %s", KEYNAME, err)
	require.Equalf(t, defaultCap, capacity, "Expected default capacity of %d, got %d", defaultCap, capacity)
	require.Falsef(t, splitkeys, "Splitkey")

	require.Equalf(t, 1, len(keys), "Number of keys mismatch")
	require.Equalf(t, uint16(28484), keys[0].K.KeyTag(), "Key tag mismatch")
	// DNSKEY record
	require.Equalf(t,
		"example.com.\t3600\tIN\tDNSKEY\t256 3 13 rSNkY7tjAffsDOnbOhGdKD8jzXE1CDEmAjbZnmB+xJ+q4pHO5d0C6/euObtbJpLKZJJPghZP4C3RYrjfloxrRg==",
		keys[0].K.String(),
		"DNSKEY mismatch")
	// DS record
	require.Equalf(t,
		"example.com.\t3600\tIN\tDS\t28484 13 2 1FF321AC955415D67D41040B6C6F5BDF561DB74518808A72F3057918076D5859",
		keys[0].D.String(),
		"DS mismatch")
}

/*
*
TestInitializeZonesKeys tests that we properly load zones and keys
*/
func TestInitializeZonesKeys(t *testing.T) {
	expectedZones := []string{"example.com.", "example.net."}
	zones, keys, splitkeys, cache, err := initializeZonesKeys([]string{"example.com", "Example.Net"}, []string{testutils.FixturePath("../testdata/data/", KEYNAME)})

	require.Nilf(t, err, "Failed to execute InitializeZonesKeys: %v", err)
	require.Equalf(t, expectedZones, zones, "Expected zones to be normalized. Expected %v, got %v", expectedZones, zones)
	require.Falsef(t, splitkeys, "No split key expected")
	require.NotNilf(t, cache, "cache is not expected to be nil")
	require.NotNilf(t, keys, "keys is not expected to be nil")
}
