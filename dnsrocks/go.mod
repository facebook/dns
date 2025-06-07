module github.com/facebook/dns/dnsrocks

go 1.23.0

toolchain go1.23.5

require (
	github.com/coredns/coredns v1.12.2
	github.com/dnstap/golang-dnstap v0.4.0
	github.com/eclesh/welford v0.0.0-20150116075914-eec62615b1f0
	github.com/fsnotify/fsnotify v1.5.1
	github.com/golang/glog v1.2.4
	github.com/hashicorp/golang-lru v0.5.4
	github.com/miekg/dns v1.1.66
	github.com/otiai10/copy v1.6.0
	github.com/prometheus/client_golang v1.22.0
	github.com/prometheus/client_model v0.6.2
	github.com/repustate/go-cdb v0.0.0-20160430174706-6a418fad95e2
	github.com/segmentio/fasthash v1.0.3
	github.com/sirupsen/logrus v1.9.3
	github.com/stretchr/testify v1.10.0
	go.uber.org/mock v0.5.0
	golang.org/x/sync v0.14.0
	golang.org/x/sys v0.33.0
	golang.org/x/term v0.32.0
	google.golang.org/protobuf v1.36.6
)

require (
	github.com/apparentlymart/go-cidr v1.1.0 // indirect
	github.com/aws/aws-sdk-go-v2 v1.36.3 // indirect
	github.com/aws/aws-sdk-go-v2/config v1.29.14 // indirect
	github.com/aws/aws-sdk-go-v2/credentials v1.17.67 // indirect
	github.com/aws/aws-sdk-go-v2/feature/ec2/imds v1.16.30 // indirect
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.3.34 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.6.34 // indirect
	github.com/aws/aws-sdk-go-v2/internal/ini v1.8.3 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/accept-encoding v1.12.3 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.12.15 // indirect
	github.com/aws/aws-sdk-go-v2/service/secretsmanager v1.35.4 // indirect
	github.com/aws/aws-sdk-go-v2/service/sso v1.25.3 // indirect
	github.com/aws/aws-sdk-go-v2/service/ssooidc v1.30.1 // indirect
	github.com/aws/aws-sdk-go-v2/service/sts v1.33.19 // indirect
	github.com/aws/smithy-go v1.22.2 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/coredns/caddy v1.1.2-0.20241029205200-8de985351a98 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/dgryski/go-spooky v0.0.0-20170606183049-ed3d087f40e2 // indirect
	github.com/farsightsec/golang-framestream v0.3.0 // indirect
	github.com/flynn/go-shlex v0.0.0-20150515145356-3f9db97f8568 // indirect
	github.com/go-task/slim-sprig/v3 v3.0.0 // indirect
	github.com/golang/protobuf v1.5.4 // indirect
	github.com/google/pprof v0.0.0-20241210010833-40e02aabc2ad // indirect
	github.com/grpc-ecosystem/grpc-opentracing v0.0.0-20180507213350-8e809c8a8645 // indirect
	github.com/kr/text v0.2.0 // indirect
	github.com/matttproud/golang_protobuf_extensions v1.0.4 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/onsi/ginkgo/v2 v2.22.1 // indirect
	github.com/opentracing/opentracing-go v1.2.0 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/prometheus/common v0.64.0 // indirect
	github.com/prometheus/procfs v0.15.1 // indirect
	github.com/quic-go/quic-go v0.52.0 // indirect
	go.uber.org/automaxprocs v1.6.0 // indirect
	golang.org/x/crypto v0.38.0 // indirect
	golang.org/x/mod v0.24.0 // indirect
	golang.org/x/net v0.40.0 // indirect
	golang.org/x/text v0.25.0 // indirect
	golang.org/x/tools v0.32.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20250512202823-5a2f75b736a9 // indirect
	google.golang.org/grpc v1.72.2 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace github.com/repustate/go-cdb => ./go-cdb-mods
