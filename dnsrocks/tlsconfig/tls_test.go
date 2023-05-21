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

package tlsconfig

import (
	"os"
	"testing"

	"github.com/facebookincubator/dns/dnsrocks/testaid"

	"github.com/stretchr/testify/require"
)

// Create a TLSConfig suitable for dotTLSAHandler
func makeTLSConfig(certfile string, sessionticketcfg SessionTicketKeysConfig) *TLSConfig {
	return &TLSConfig{
		CertFile:          certfile,
		KeyFile:           certfile,
		SessionTicketKeys: sessionticketcfg,
	}
}

// TestInitTLSConfigLoadsCertAndKey tests that tls implementation successfully loads
// and sets Certificates on tls.Config struct based on our TLSConfig struct
func TestInitTLSConfigLoadsCertAndKey(t *testing.T) {
	certfile := testaid.MkTestCert(t)
	defer os.Remove(certfile)
	tlsconfigstruct := makeTLSConfig(certfile, SessionTicketKeysConfig{})

	tlsconfig, err := InitTLSConfig(tlsconfigstruct)
	require.NotNil(t, tlsconfig.Certificates)
	require.Nil(t, err)
}

// TestInitTLSConfigLoadsCertAndKey tests that tls implementation returns err
// in case of invalid tls certificate paths are provided
func TestInitTLSConfigErrorsOnInvalidCertAndKeyPath(t *testing.T) {
	tmpfile, err := os.CreateTemp("", "example")
	require.Nil(t, err)
	os.Remove(tmpfile.Name())
	tlsconfigstruct := makeTLSConfig(tmpfile.Name(), SessionTicketKeysConfig{})

	tlsconfig, err := InitTLSConfig(tlsconfigstruct)
	require.NotNil(t, err)
	require.Nil(t, tlsconfig)
}

// TestInitTLSConfigLoadsCertAndKey tests that tls implementation returns no err
// in case of invalid tls session ticket seed paths are provided
func TestInitTLSConfigNoErrorOnInvalidResumptionTicket(t *testing.T) {
	certfile := testaid.MkTestCert(t)
	defer os.Remove(certfile)
	tmpfile, err := os.CreateTemp("", "example")
	require.Nil(t, err)
	os.Remove(tmpfile.Name())
	tlsconfigstruct := makeTLSConfig(certfile, SessionTicketKeysConfig{SeedFile: tmpfile.Name()})

	tlsconfig, err := InitTLSConfig(tlsconfigstruct)
	require.Nil(t, err)
	require.NotNil(t, tlsconfig)
}
