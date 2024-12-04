SHELL := /bin/bash
all: build
.PHONY: all

LIBBLAZESYM_SRC := $(abspath ./blazesym/)
LIBBLAZESYM_INC := $(abspath $(LIBBLAZESYM_SRC)/capi/include)
LIBBLAZESYM_OBJ := $(abspath $(LIBBLAZESYM_SRC)/target/release/libblazesym_c.a)
ALL_LDFLAGS := $(LDFLAGS) $(EXTRA_LDFLAGS)
bpf/vmlinux.h:
	@which bpftool > /dev/null
	bpftool btf dump file /sys/kernel/btf/vmlinux format c > $@
$(LIBBLAZESYM_SRC)/target/release/libblazesym_c.a::
	$(Q)cd $(LIBBLAZESYM_SRC) && cargo build --package=blazesym-c --release
snoop/out/dnswatch_bpf_probe_core.o: bpf/vmlinux.h
	clang -g -O2 -Wall -fpie -fno-stack-protector -I /usr/include/bpf -target bpf -D__TARGET_ARCH_x86 -I bpf  -c bpf/dnswatch.bpf.c -o $@
build: snoop/out/dnswatch_bpf_probe_core.o $(LIBBLAZESYM_SRC)/target/release/libblazesym_c.a
	CGO_LDFLAGS=" -I $(LIBBLAZESYM_INC) -L /usr/lib -L $(abspath $(LIBBLAZESYM_SRC)/target/release) -lsystemd -lcap -lblazesym_c -libverbs -lnl-route-3 -lnl-3 -lzstd -lbpf -ldbus-1 -static " CGO_CFLAGS="-I /usr/include/bpf -I /usr/include/pcap -I $(LIBBLAZESYM_INC) -L /usr/lib -L $(abspath $(LIBBLAZESYM_SRC)/target/release)" go build .
test: snoop/out/dnswatch_bpf_probe_core.o $(LIBBLAZESYM_SRC)/target/release/libblazesym_c.a
	CGO_LDFLAGS=" -I $(LIBBLAZESYM_INC) -L /usr/lib -L $(abspath $(LIBBLAZESYM_SRC)/target/release) -lsystemd -lcap -lblazesym_c -libverbs -lnl-route-3 -lnl-3 -lzstd -lbpf -ldbus-1 -static " CGO_CFLAGS="-I /usr/include/bpf -I /usr/include/pcap -I $(LIBBLAZESYM_INC) -L /usr/lib -L $(abspath $(LIBBLAZESYM_SRC)/target/release)" go test -v -coverprofile=coverage.txt -covermode=atomic ./...
install_linter:
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b ~ 
lint: snoop/out/dnswatch_bpf_probe_core.o install_linter $(LIBBLAZESYM_SRC)/target/release/libblazesym_c.a
	CGO_LDFLAGS=" -I $(LIBBLAZESYM_INC) -L /usr/lib -L $(abspath $(LIBBLAZESYM_SRC)/target/release) -lsystemd -lcap -lblazesym_c -libverbs -lnl-route-3 -lnl-3 -lzstd -lbpf -ldbus-1 -static " CGO_CFLAGS="-I /usr/include/bpf -I /usr/include/pcap -I $(LIBBLAZESYM_INC) -L /usr/lib -L $(abspath $(LIBBLAZESYM_SRC)/target/release)" ~/golangci-lint run -v  ./...
clean:
	rm -f snoop/out/dnswatch_bpf_probe_core.o
	rm -f bpf/vmlinux.h
	rm -rf $(LIBBLAZESYM_OBJ)