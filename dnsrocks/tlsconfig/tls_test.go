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
	"io/ioutil"
	"os"
	"testing"

	"github.com/facebookincubator/dns/dnsrocks/testaid"

	"github.com/stretchr/testify/assert"
)

// Create a TLSConfig suitable for dotTLSAHandler
func makeTLSConfig(certfile string, sessionticketcfg SessionTicketKeysConfig) *TLSConfig {
	return &TLSConfig{
		CertFile:          certfile,
		KeyFile:           certfile,
		SessionTicketKeys: sessionticketcfg,
	}
}

// TestInitTLSConfigLoadsCertAndKey tests that tls implementation successsfully loads
// and sets Certificates on tls.Config struct based on our TLSConfig struct
func TestInitTLSConfigLoadsCertAndKey(t *testing.T) {
	certfile := testaid.MkTestCert(t)
	defer os.Remove(certfile)
	tlsconfigstruct := makeTLSConfig(certfile, SessionTicketKeysConfig{})

	tlsconfig, err := InitTLSConfig(tlsconfigstruct)
	assert.NotNil(t, tlsconfig.Certificates)
	assert.Nil(t, err)
}

// TestInitTLSConfigLoadsCertAndKey tests that tls implementation returns err
// in case of invalid tls certificate paths are provided
func TestInitTLSConfigErrorsOnInvalidCertAndKeyPath(t *testing.T) {
	tmpfile, err := ioutil.TempFile("", "example")
	assert.Nil(t, err)
	os.Remove(tmpfile.Name())
	tlsconfigstruct := makeTLSConfig(tmpfile.Name(), SessionTicketKeysConfig{})

	tlsconfig, err := InitTLSConfig(tlsconfigstruct)
	assert.NotNil(t, err)
	assert.Nil(t, tlsconfig)
}

// TestInitTLSConfigLoadsCertAndKey tests that tls implementation returns no err
// in case of invalid tls session ticket seed paths are provided
func TestInitTLSConfigNoErrorOnInvalidResumptionTicket(t *testing.T) {
	certfile := testaid.MkTestCert(t)
	defer os.Remove(certfile)
	tmpfile, err := ioutil.TempFile("", "example")
	assert.Nil(t, err)
	os.Remove(tmpfile.Name())
	tlsconfigstruct := makeTLSConfig(certfile, SessionTicketKeysConfig{SeedFile: tmpfile.Name()})

	tlsconfig, err := InitTLSConfig(tlsconfigstruct)
	assert.Nil(t, err)
	assert.NotNil(t, tlsconfig)
}
