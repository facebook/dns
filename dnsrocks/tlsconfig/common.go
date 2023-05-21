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
	"crypto/tls"
)

// CryptoSSLConfig contains config specific to Crypto SSL
type CryptoSSLConfig struct {
	CertName string
	Tier     string
}

// SessionTicketKeysConfig contains the config for handling session resumption
type SessionTicketKeysConfig struct {
	SeedFile               string
	SeedFileReloadInterval int
}

// TLSConfig contains config for a given TLS Listener
type TLSConfig struct {
	Port              int
	CertFile          string
	KeyFile           string
	CryptoSSL         CryptoSSLConfig
	SessionTicketKeys SessionTicketKeysConfig
	DoTTLSAEnabled    bool
	DoTTLSATtl        uint32 // TTL of the TLSA record. Note that a value of 0 will make the record default to defaultTLSATtl
}

// LoadTLSCertFromFile loads TLS cert from the path specified in TLSConfig
func LoadTLSCertFromFile(tlsConfig *TLSConfig) (cert tls.Certificate, err error) {
	return tls.LoadX509KeyPair(tlsConfig.CertFile, tlsConfig.KeyFile)
}
