# DNSWatch
[![lint_dnswatch](https://github.com/facebookincubator/dns/actions/workflows/lint_dnswatch.yml/badge.svg)](https://github.com/facebookincubator/dns/actions/workflows/lint_dnswatch.yml)
[![test_dnswatch](https://github.com/facebookincubator/dns/actions/workflows/test_dnswatch.yml/badge.svg)](https://github.com/facebookincubator/dns/actions/workflows/test_dnswatch.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/facebookincubator/dns/dnswatch)](https://goreportcard.com/report/github.com/facebookincubator/dns/dnswatch)
[![License](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)

## Documentation
DNSWatch is a DNS snooping utility

It helps with investigating what DNS queries are made on the host by which process

# Dependencies
- libbpf 1.1.0
- libpcap

# Build dependencies
- go 1.18 +
- bpftool
- libpcap-dev
- libbpf-dev
- clang
- make


# Building DNSWatch
- Make sure your build host has all the build dependencies `apt-get install libbpf1 libbpf-dev libpcap0.8 libpcap0.8-dev make clang gcc-multilib`
- You also might need to install bpftool. `apt-get install bpftool`
- run `make`

# Functionality of DNSWatch
### top
A top-like interface which gives a high level overview of how many DNS queries are executed by which process on the system.
This is useful to get a quick glance of DNS query activity, and identify processes doing suspicious amount of DNS queries
The view can be grouped by PID or process name, and sorted by number of A/AAAA/PTR queries, and NOERROR, NXDOMAIN, SERVFAIL responses

![Top](.res/top.gif)


### detailed
A dig-like display of all dns queries made on the host. This enables to inspect DNS traffic in depth

![Detailed](.res/detailed.gif)

### exporter
A Prometheus exporter, which reports the number of DNS queries made grouped by query type (A/AAAA/PTR) and response (NOERROR/SERVFAIL/NXDOMAIN)
sample output
```
[deathowl@dnswatcher dnswatch]$ curl localhost:9422/metrics
# HELP a_queries The number of A queries
# TYPE a_queries counter
a_queries{process="UNK"} 2
a_queries{process="all"} 6492
a_queries{process="curl"} 2
a_queries{process="python3"} 6488
# HELP aaaa_queries The number of AAAA queries
# TYPE aaaa_queries counter
aaaa_queries{process="UNK"} 0
aaaa_queries{process="all"} 2
aaaa_queries{process="curl"} 2
aaaa_queries{process="python3"} 0
# HELP noerror_responses The number of NOERROR responses
# TYPE noerror_responses counter
noerror_responses{process="UNK"} 0
noerror_responses{process="all"} 3243
noerror_responses{process="curl"} 2
noerror_responses{process="python3"} 3241
# HELP nxdomain_responses The number of NXDOMAIN responses
# TYPE nxdomain_responses counter
nxdomain_responses{process="UNK"} 0
nxdomain_responses{process="all"} 0
nxdomain_responses{process="curl"} 0
nxdomain_responses{process="python3"} 0
# HELP ptr_queries The number of PTR queries
# TYPE ptr_queries counter
ptr_queries{process="UNK"} 0
ptr_queries{process="all"} 0
ptr_queries{process="curl"} 0
ptr_queries{process="python3"} 0
# HELP servfail_responses The number of SERVFAIL responses
# TYPE servfail_responses counter
servfail_responses{process="UNK"} 0
servfail_responses{process="all"} 0
servfail_responses{process="curl"} 0
servfail_responses{process="python3"} 0
```
### snoop
A less detailed output than detailed, shows PID/Process name/QTYPE/QNAME and RCODE of all dns queries made on the host

![Snoop](.res/snoop.gif)

### sql
Transform and display DNS activity using 'where', 'orderby', 'groupby'; Supports printing data to csv

Example run and generated csv:

```
[deathowl@dnswatcher dnswatch]$ sudo ./dnswatch sql --orderby -LATENCY --groupby PNAME,QNAME --csv test.csv --period 30s
[deathowl@dnswatcher dnswatch]$ cat test.csv
LATENCY_COUNT,LATENCY_MAX,LATENCY_MEAN,LATENCY_MEDIAN,LATENCY_MIN,PNAME,QNAME
4.000000,18797.000000,9291.000000,9183.500000,0.000000,curl,deathowl.com
4.000000,12119.000000,4364.000000,2668.500000,0.000000,pacman,mirror.osbeck.com
16.000000,15206.000000,1315.125000,0.000000,0.000000,curl,facebook.com
4.000000,726.000000,353.500000,344.000000,0.000000,curl,google.com
```
### nettop
Interactive nettop-like display. This is useful to run on a dns server, to see which client machine makes the most DNS queries.
The view can be sorted by number of A/AAAA/PTR queries, and NOERROR, NXDOMAIN, SERVFAIL responses

# Project structure
### bpf
    The BPF code used to capture DNS traffic
### cmd
    All entrypoints to the functions provided by DNSWatch are defined here
### snoop
    DNS snooping logic is here

# License
DNSWatch is licensed under Apache 2.0 as found in the [LICENSE file](LICENSE).

The BPF code is licensed under GPL-3 as found in the [LICENSE file](bpf/LICENSE).
