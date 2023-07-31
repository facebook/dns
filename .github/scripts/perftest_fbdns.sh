#!/bin/bash
apt-get install -qq dnsperf
cd dnsrocks || exit
mkdir testdata/perftest
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
for s in {555..888888} ; do echo -C$s.test.com,test.com,4269 ;done >> testdata/data/perftest.diff
for s in {555..888888} ; do echo +C$s.test.com,deathowl.com,4269 ;done >> testdata/data/perftest.diff
echo "DONE Generating test data"
CGO_LDFLAGS_ALLOW=".*" CGO_CFLAGS_ALLOW=".*" go run cmd/dnsrocks-data/dnsrocks-data.go -i testdata/data/perftest.in -o testdata/perftest
CGO_LDFLAGS_ALLOW=".*" CGO_CFLAGS_ALLOW=".*" go build cmd/dnsrocks/dnsrocks.go
./dnsrocks  -ip ::1 -port 8053 -dbdriver rocksdb -dbpath testdata/perftest -refuse-any -dnstap-target stdout &
INITIAL_LOST=$(dnsperf -d testdata/data/dnsperf.txt -p 8053 -s ::1  -c 80 -T 10 -l 60 | grep lost)
CGO_LDFLAGS_ALLOW=".*" CGO_CFLAGS_ALLOW=".*" go run cmd/dnsrocks-applyrdb/dnsrocks-applyrdb.go -i testdata/data/perftest.diff -o testdata/perftest
AFTER_LOST=$(dnsperf -d testdata/data/dnsperf.txt -p 8053 -s ::1  -c 80 -T 10 -l 60 | grep lost)
if [ "$INITIAL_LOST" != "$AFTER_LOST" ]
then
    echo "Queries lost after applying change does not equal the amount before applying the change"
    echo "BEFORE: $INITIAL_LOST"
    echo "AFTER: $AFTER_LOST"
    echo "This is not great."
    exit 1
fi
echo "ALL GOOD"
