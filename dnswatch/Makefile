all: build
.PHONY: all

bpf/vmlinux.h:
	 bpftool btf dump file /sys/kernel/btf/vmlinux format c > $@
snoop/out/dnswatch_bpf_probe_core.o: bpf/vmlinux.h
	clang -g -O2 -Wall -fpie -fno-stack-protector -I /usr/include/bpf -target bpf -D__TARGET_ARCH_x86 -I bpf  -c bpf/dnswatch.bpf.c -o $@
build: snoop/out/dnswatch_bpf_probe_core.o
	CGO_LDFLAGS="-lbpf" CGO_CFLAGS="-I /usr/include/bpf  -I /usr/include/pcap" go build .
clean:
	rm -f snoop/out/dnswatch_bpf_core.o
	rm -f bpf/vmlinux.h
