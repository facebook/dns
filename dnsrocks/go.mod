module github.com/facebookincubator/dns/dnsrocks

go 1.18

require (
	github.com/coredns/coredns v1.10.0
	github.com/dnstap/golang-dnstap v0.4.0
	github.com/fsnotify/fsnotify v1.5.1
	github.com/golang/glog v1.0.0
	github.com/golang/mock v1.6.0
	github.com/hashicorp/golang-lru v0.5.4
	github.com/miekg/dns v1.1.50
	github.com/otiai10/copy v1.6.0
	github.com/pkg/errors v0.9.1
	github.com/prometheus/client_golang v1.13.0
	github.com/prometheus/client_model v0.2.0
	github.com/repustate/go-cdb v0.0.0-20160430174706-6a418fad95e2
	github.com/sirupsen/logrus v1.8.1
	github.com/stretchr/testify v1.7.1
	golang.org/x/sync v0.0.0-20220722155255-886fb9371eb4
	golang.org/x/sys v0.0.0-20220804214406-8e32c043e418
	golang.org/x/term v0.0.0-20210927222741-03fcf44c2211
	google.golang.org/protobuf v1.28.1
)

require (
	github.com/apparentlymart/go-cidr v1.1.0 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.1.2 // indirect
	github.com/coredns/caddy v1.1.1 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/dgryski/go-spooky v0.0.0-20170606183049-ed3d087f40e2 // indirect
	github.com/farsightsec/golang-framestream v0.3.0 // indirect
	github.com/flynn/go-shlex v0.0.0-20150515145356-3f9db97f8568 // indirect
	github.com/golang/protobuf v1.5.2 // indirect
	github.com/grpc-ecosystem/grpc-opentracing v0.0.0-20180507213350-8e809c8a8645 // indirect
	github.com/matttproud/golang_protobuf_extensions v1.0.1 // indirect
	github.com/opentracing/opentracing-go v1.2.0 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/prometheus/common v0.37.0 // indirect
	github.com/prometheus/procfs v0.8.0 // indirect
	golang.org/x/crypto v0.0.0-20220722155217-630584e8d5aa // indirect
	golang.org/x/mod v0.6.0-dev.0.20220419223038-86c51ed26bb4 // indirect
	golang.org/x/net v0.0.0-20220722155237-a158d28d115b // indirect
	golang.org/x/text v0.3.7 // indirect
	golang.org/x/tools v0.1.12 // indirect
	google.golang.org/genproto v0.0.0-20220624142145-8cd45d7dbd1f // indirect
	google.golang.org/grpc v1.49.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace github.com/repustate/go-cdb => ./go-cdb-mods
