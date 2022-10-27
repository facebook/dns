# Getting started

One of the most important command to get started is `dnsrocks-data` which enables creation of rocksdb or cdb databases out of input files that match our required [data_format](data_format.md)
Example:
```
./dnsrocks-data -i ../../testdata/data/data.in -o ~/example_rdb
2022/10/18 17:28:56 Creating database /home/death0wl/example_rdb
2022/10/18 17:28:57 Reading ...
2022/10/18 17:28:57 Sorting ...
2022/10/18 17:28:57 Creating buckets no smaller than 30000 items each, and no more than 8 buckets total
2022/10/18 17:28:57 Created buckets: [{0 301}]
2022/10/18 17:28:57 w#0 saving 301 values into /home/death0wl/example_rdb/rdb0.sst
2022/10/18 17:28:57 w#0 wrote 2880 bytes to /home/death0wl/example_rdb/rdb0.sst finishing write...
2022/10/18 17:28:57 1 buckets saved, 180 keys in 0.002 seconds, 90000.00 keys per second, 0.0 MiB total
2022/10/18 17:28:57 Ingesting done, cleanup
2022/10/18 17:28:57 Building done
2022/10/18 17:28:57 301 records written
```
---
This generated database then, can be used to run your authoritative dns server instance using the `dnsrocks` command
Example:
```
 ./dnsrocks  -ip ::1 -port 8053 -tcp=false -dbdriver rocksdb -dbpath  /home/death0wl/example_rdb
I1018 17:31:49.029569  256282 db.go:300] Loading /home/death0wl/example_rdb using rocksdb driver
I1018 17:31:49.030862  256282 metrics_prom.go:49] Starting prometheus metrics server at ":18888"
I1018 17:31:49.034316  256282 server.go:252] -whoami-domain was not specified, not initializing whoamiHandler
I1018 17:31:49.034339  256282 server.go:263] refuse-any flag was not specified, not initializing anyHandler
I1018 17:31:49.034350  256282 server.go:273] Creating handler with VIP ::1 and max answer 1
I1018 17:31:49.034360  256282 server.go:289] Not enabling DNSSEC Handler, either DNSSEC zones or keys are not specified. Zones: '', Keys: ''
E1018 17:31:49.034924  256282 server.go:467] LogMapAge: Timestamp key not found
```

You can use dig to verify your dns server is alive and serving requests:
```
dig  foo.example.net  @localhost -p 8053

; <<>> DiG 9.16.33-RH <<>> foo.example.net @localhost -p 8053
;; global options: +cmd
;; Got answer:
;; ->>HEADER<<- opcode: QUERY, status: NOERROR, id: 29946
;; flags: qr aa rd; QUERY: 1, ANSWER: 1, AUTHORITY: 0, ADDITIONAL: 1
;; WARNING: recursion requested but not available

;; OPT PSEUDOSECTION:
; EDNS: version: 0, flags:; udp: 4096
;; QUESTION SECTION:
;foo.example.net.        IN    A

;; ANSWER SECTION:
foo.example.net.    180    IN    A    1.1.1.1

;; Query time: 0 msec
;; SERVER: ::1#8053(::1)
;; WHEN: Tue Oct 18 17:35:14 IST 2022
;; MSG SIZE  rcvd: 75
```
