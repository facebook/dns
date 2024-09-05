SHELL := /bin/bash
all: build
.PHONY: all

bpf/vmlinux.h:
	@which bpftool > /dev/null
	bpftool btf dump file /sys/kernel/btf/vmlinux format c > $@
snoop/out/dnswatch_bpf_probe_core.o: bpf/vmlinux.h
	clang -g -O2 -Wall -fpie -fno-stack-protector -I /usr/include/bpf -target bpf -D__TARGET_ARCH_x86 -I bpf  -c bpf/dnswatch.bpf.c -o $@
build: snoop/out/dnswatch_bpf_probe_core.o
	CGO_LDFLAGS="-lbpf -lbcc" CGO_CFLAGS="-I /usr/include/bpf -I /usr/include/bcc -I /usr/include/pcap" go build .
test: snoop/out/dnswatch_bpf_probe_core.o
	CGO_LDFLAGS="-lbpf -lbcc" CGO_CFLAGS="-I /usr/include/bpf -I /usr/include/bcc -I /usr/include/pcap" go test -v -coverprofile=coverage.txt -covermode=atomic ./...
install_linter:
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b ~ 
lint: snoop/out/dnswatch_bpf_probe_core.o install_linter
	CGO_LDFLAGS="-lbpf -lbcc" CGO_CFLAGS="-I /usr/include/bpf -I /usr/include/bcc -I /usr/include/pcap" ~/golangci-lint run -v  ./...
clean:
	rm -f snoop/out/dnswatch_bpf_probe_core.o
	rm -f bpf/vmlinux.h
