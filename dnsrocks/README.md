# dnsrocks

[![lint](https://github.com/facebookincubator/dns/actions/workflows/lint.yml/badge.svg)](https://github.com/facebookincubator/dns/actions/workflows/lint.yml)
[![test](https://github.com/facebookincubator/dns/actions/workflows/test.yml/badge.svg)](https://github.com/facebookincubator/dns/actions/workflows/test.yml)
[![License](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)

# Contents


- [Documentation](docs/getting_started.md)
- [License](#License)

## Documentation

Facebook's authoritative dns server

### cgo-rocksdb
cgo bindings for rocksdb

### cmd
All executables provided by this repo.

### db
data access related logic
### dnsdata
Handling of dns record types
### dnsserver
Basic dns server functions (and base handler)
### fbserver
Full fledged implementation of an authoriative dns server
### go-cdb-mods
modified version of github.com/repustate/go-cdb to support go-modules
### testaid
Bootstraps test data for integration tests
### metrics
Metrics handler containing a stats implementation and prometheus exporter
### logger
Logger, containing logging related utility functions and a dnstap logger
### tlsconfig
Configures tls, used for TLS Session Resumption, dotTLSA and DNSSEC
### testdata
Mock data used for tests
### testutils
Test helpers aiding file access
### whoami
Whoami handler


# License
dnsrocks is licensed under Apache 2.0 as found in the [LICENSE file](LICENSE).
