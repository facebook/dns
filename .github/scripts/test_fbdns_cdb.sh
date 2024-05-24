#!/bin/bash
set -e
apt-get install -qq jq
cd dnsrocks || exit
echo "Generating test data"
echo """Ztest.com,a.ns.test.com,dns.test.com,123,7200,1800,604800,120,120,,
&test.com,,a.ns.test.com,172800,,
&test.com,,b.ns.test.com,172800,,
=a.ns.test.com,fd09:14f5:dead:beef:1::35,172800,,
=a.ns.test.com,5.5.5.5,172800,,
=b.ns.test.com,fd09:14f5:dead:beef:2::35,172800,,
=b.ns.test.com,5.5.6.5,172800,,
=test.com,::1,4269
=test.com,192.168.0.1,4269""" >> testdata/data/perftest.in
for s in {1..1000000} ; do echo C$s.test.com,test.com,4269 ;done >> testdata/data/perftest.in
echo "DONE Generating test data"
CGO_LDFLAGS_ALLOW=".*" CGO_CFLAGS_ALLOW=".*" go run cmd/dnsrocks-data/dnsrocks-data.go -dbdriver cdb -i testdata/data/perftest.in -o testdata/cdbtest
CGO_LDFLAGS_ALLOW=".*" CGO_CFLAGS_ALLOW=".*" go build cmd/dnsrocks/dnsrocks.go
./dnsrocks  -ip ::1 -port 8053 -dbdriver cdb -dbpath testdata/cdbtest -refuse-any -dnstap-target stdout &
cd ../goose || exit 1
LOST=$(go run  . -input-file ../dnsrocks/testdata/data/dnsperf.txt -port 8053 -host ::1  -parallel-connections 10 -max-duration 60s -report-json | jq .Errors)
if [ "$LOST" != "0" ]
then
    echo "Queries lost is not 0"
    echo "LOST: $LOST"
    echo "This is not great."
    exit 1
fi
echo "ALL GOOD"
