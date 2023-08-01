# Building DNSRocks

## Prerequisites
- [Rocksdb](https://github.com/facebook/rocksdb/releases) 7.3 or newer
- [Go 1.18](https://github.com/facebook/dns/blob/main/dnsrocks/go.mod#L3)


## Verifying every dependency is installed correctly
rocksdb is being found properly by pkg-config (needed by our CGO bindings)
```
pkg-config --dump-personality rocksdb
Triplet: default
DefaultSearchPaths: /usr/lib64/pkgconfig /usr/share/pkgconfig
SystemIncludePaths: /usr/include
SystemLibraryPaths: /usr/lib64
```
Verifying go version
```
go version
go version go1.18.7 linux/amd64
```

## Building DNSRocks
```
~/work/dns/dnsrocks/cmd/dnsrocks$ CGO_LDFLAGS_ALLOW=".*" CGO_CFLAGS_ALLOW=".*" go build .
# github.com/facebook/dns/dnsrocks/cgo-rocksdb
cc1: warning: command-line option ‘-std=c++11’ is valid for C++/ObjC++ but not for C
# github.com/facebook/dns/dnsrocks/cgo-rocksdb
cc1: warning: command-line option ‘-std=c++11’ is valid for C++/ObjC++ but not for C
# github.com/facebook/dns/dnsrocks/cgo-rocksdb
cc1: warning: command-line option ‘-std=c++11’ is valid for C++/ObjC++ but not for C
# github.com/facebook/dns/dnsrocks/cgo-rocksdb
cc1: warning: command-line option ‘-std=c++11’ is valid for C++/ObjC++ but not for C
# github.com/facebook/dns/dnsrocks/cgo-rocksdb
cc1: warning: command-line option ‘-std=c++11’ is valid for C++/ObjC++ but not for C
# github.com/facebook/dns/dnsrocks/cgo-rocksdb
cc1: warning: command-line option ‘-std=c++11’ is valid for C++/ObjC++ but not for C
# github.com/facebook/dns/dnsrocks/cgo-rocksdb
cc1: warning: command-line option ‘-std=c++11’ is valid for C++/ObjC++ but not for C
# github.com/facebook/dns/dnsrocks/cgo-rocksdb
cc1: warning: command-line option ‘-std=c++11’ is valid for C++/ObjC++ but not for C
# github.com/facebook/dns/dnsrocks/cgo-rocksdb
cc1: warning: command-line option ‘-std=c++11’ is valid for C++/ObjC++ but not for C
# github.com/facebook/dns/dnsrocks/cgo-rocksdb
cc1: warning: command-line option ‘-std=c++11’ is valid for C++/ObjC++ but not for C
# github.com/facebook/dns/dnsrocks/cgo-rocksdb
cc1: warning: command-line option ‘-std=c++11’ is valid for C++/ObjC++ but not for C
```
produces the dnsrocks binary
