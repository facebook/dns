/*
Copyright (c) Facebook, Inc. and its affiliates.
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
    http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

/*
** BPF Script (not a C file)
*/

// @fb-only: #include <bpf/vmlinux/vmlinux.h>
#include "vmlinux.h" // @oss-only

#include <bpf/bpf_core_read.h>
#include <bpf/bpf_endian.h>
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_tracing.h>
#include <string.h>

// HASHMAP_SIZE is a big prime number
#define HASHMAP_SIZE 100003
#define DNS_PROBE_PORT 53

struct dnswatch_kprobe_event_data {
  u32 tgid;
  u32 pid;
  int sock_port_nr;
  char fn_id;
};

// dnswatch_kprobe_output_events adds events in perf map
// BPF_PERF_OUTPUT(dnswatch_kprobe_output_events);
struct {
  __uint(type, BPF_MAP_TYPE_RINGBUF);
  __uint(max_entries, 1 << 24);
} dnswatch_kprobe_output_events SEC(".maps");

// sendmsg_solver populates the dnswatch_kprobe_event_data struct for each
// callback.
static int
sendmsg_solver(struct pt_regs* ctx, char fn_id, u16 dport, u16 sport) {
  if (dport != bpf_htons(DNS_PROBE_PORT)) {
    return 0;
  }

  // bpf_get_current_pid_tgid() returns a single u64
  // most significant 32 bits => thread group id
  // least significant 32 bits => process id
  u64 __pid_tgid = bpf_get_current_pid_tgid();
  u32 __tgid = __pid_tgid >> 32;
  u32 __pid = __pid_tgid;

  struct dnswatch_kprobe_event_data* data = bpf_ringbuf_reserve(
      &dnswatch_kprobe_output_events,
      sizeof(struct dnswatch_kprobe_event_data),
      0);

  if (!data)
    return 0;
  data->tgid = __tgid;
  data->pid = __pid;
  data->sock_port_nr = (int)sport;
  data->fn_id = fn_id;

  bpf_ringbuf_submit(data, 0);
  return 0;
}

SEC("fentry/udpv6_sendmsg")
int BPF_PROG(
    dnswatch_kprobe_udpv6_sendmsg,
    struct sock* sk,
    struct msghdr* msg) {
  struct sock_common* sk_common = (struct sock_common*)sk;
  struct sockaddr_in6* sin6;
  u16 dport, sport;

  sin6 = (struct sockaddr_in6*)msg->msg_name;
  // handle connectionless udp ipv6 sockets. If the process did not call
  // connect(udp_fc,...) the dport is set to 0 in struct sock, so we need to get
  // the dport from (struct msghdr*)msg->(struct
  // sockaddr_in6*)msg_name->sin6_port.
  dport = sk_common->skc_dport;
  if (sin6) {
    bpf_probe_read_kernel(&dport, sizeof(u16), &sin6->sin6_port);
  }
  sport = sk_common->skc_num;

  return sendmsg_solver(ctx, 0, dport, sport);
}

SEC("fentry/udp_sendmsg")
int BPF_PROG(dnswatch_kprobe_udp_sendmsg, struct sock* sk, struct msghdr* msg) {
  struct sock_common* sk_common = (struct sock_common*)sk;
  struct sockaddr_in* sin = msg->msg_name;
  u16 dport, sport;

  // handle connectionless udp ipv4 sockets. Same as udp ipv6, but different
  // structs and fields.
  dport = sk_common->skc_dport;
  if (sin) {
    bpf_probe_read_kernel(&dport, sizeof(u16), &sin->sin_port);
  }
  sport = sk_common->skc_num;

  return sendmsg_solver(ctx, 1, dport, sport);
}

SEC("fentry/tcp_sendmsg")
int BPF_PROG(dnswatch_kprobe_tcp_sendmsg, struct sock* sk, struct msghdr* msg) {
  u16 dport, sport;

  dport = sk->__sk_common.skc_dport;
  sport = sk->__sk_common.skc_num;

  return sendmsg_solver(ctx, 2, dport, dport);
}

char LICENSE[] SEC("license") = "GPL";
