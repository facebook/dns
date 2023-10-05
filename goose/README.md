# Goose
[![lint_goose](https://github.com/facebook/dns/actions/workflows/lint_goose.yml/badge.svg)](https://github.com/facebook/dns/actions/workflows/lint_goose.yml)
[![test_dnswatch](https://github.com/facebook/dns/actions/workflows/test_goose.yml/badge.svg)](https://github.com/facebook/dns/actions/workflows/test_goose.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/facebook/dns/goose)](https://goreportcard.com/report/github.com/facebook/dns/goose)
[![License](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)

## Documentation
Goose is Meta's DNS loadtesting utility

# Build dependencies
- go 1.18 +

## Usage
```shell
Usage of ./goose:
  -daemon
        Running in daemon mode means that metrics will be exported rather than printed to stdout
  -domain string
        Domain for uncached queries
  -enable-logging
        Whether to enable logging or not (default true)
  -exporter-addr string
        Exporter bind address (default ":6869")
  -host string
        IP address of DNS server to test (default "127.0.0.1")
  -input-file string
        The file that contains queries to be made in qname qtype format
  -loglevel string
        Set a log level. Can be: debug, info, warning, error (default "info")
  -max-duration duration
        Maximum duration of test (seconds)
  -max-qps int
        max number of QPS
  -monitor-host string
        DNS queries not sent if the monitored port on this host is down (default "127.0.0.1")
  -monitor-port int
        DNS queries not sent if this port is down on the monitored host (defaults to unbound remote-control port) (default 8953)
  -parallel-connections int
        max number of parallel connections (default 1)
  -port int
        destination port (default 53)
  -pprof
        Enable pprof
  -query-type string
        Query type to be used for the query (default "A")
  -randomise-queries
        Whether to randomise dns queries to bypass potential caching
  -report-json
        Report run results to stdout in json format
  -sample duration
        Sampling frequency for reporting (seconds)
  -timeout duration
        Duration of timeout for queries (default 3s)
  -total-queries int
        Total queries to send (default 50000)
```

## Examples

* 5 parallel connections, 30000 queries to locally running DNSRocks instance with a rate limit of 1000 queries per second with reporting format set to json:
```shell
goose -host ::1 -port 8053 -domain facebook.com  -query-type AAAA -report-json -total-queries 30000 -max-qps 1000 -parallel-connections 5 | jq .
INFO[0000] The total number of DNS requests will be: 30000
INFO[0000] Limiting max qps to: 1000
INFO[0000]
     _______________________________
    < Honking at ::1:8053 with 1000 QPS.>
    --------------------------------
     \
      \
       \
                    ___
                  .^   ""-.
              _.-^( e   _  '.
               '-===.>_.-^ '  ."
                   "     "
                  :    "
                  :    |   __.--._
                  |    '--"       ""-._    _.^)
                 /                     ""-^  _>
                :                          -^>
                :                 .__>    __)
                 \     '._      .__.-'  .-'
                  '.___    '-.__.-'       /
                   '-.__    .    _.'    /
                      \_____> >'.__/_.""
                    .'.----'  | |
                  .' /        | |
                  '^-/       ___| :
                            >--  /
                            >.'.'
                            '-^
INFO[0000] Starting the test and running 5 connections in parallel
INFO[0030] Finished running all connections
INFO[0030] No more requests will be sent
INFO[0030] The test results are:
{
  "Elapsed": 30000494994,
  "Processed": 30000,
  "Errors": 0,
  "Min": 115856,
  "Max": 3188232,
  "Mean": 227724.3652,
  "Median": 208451,
  "Lowerq": 180963,
  "Upperq": 249110,
  "Average": 227724.3652
}
```

* 2 parallel connections, 10000 queries to locally running DNSRocks instance without ratelimiting:
```shell
goose -host ::1 -port 8053 -domain facebook.com  -query-type AAAA -total-queries 10000  -parallel-connections 2
INFO[0000] The total number of DNS requests will be: 10000
INFO[0000]
     _______________________________
    < Honking at ::1:8053 with Unlimited QPS.>
    --------------------------------
     \
      \
       \
                    ___
                  .^   ""-.
              _.-^( e   _  '.
               '-===.>_.-^ '  ."
                   "     "
                  :    "
                  :    |   __.--._
                  |    '--"       ""-._    _.^)
                 /                     ""-^  _>
                :                          -^>
                :                 .__>    __)
                 \     '._      .__.-'  .-'
                  '.___    '-.__.-'       /
                   '-.__    .    _.'    /
                      \_____> >'.__/_.""
                    .'.----'  | |
                  .' /        | |
                  '^-/       ___| :
                            >--  /
                            >.'.'
                            '-^
INFO[0000] Starting the test and running 2 connections in parallel
INFO[0030] Finished running all connections
INFO[0030] No more requests will be sent
INFO[0030] The test results are:
INFO[0000] Response Latency Data:(S/F: 10000/0) Max: 1.490671ms Min: 74.532µs Mean: 191.295µs Median: 179.678µs Upper Quartile: 235.698µs Lower Quartile: 100.126µs
INFO[0000] Requests: Successful: 10000 Failed: 0
INFO[0000] Elapsed: 972.07186ms
```
