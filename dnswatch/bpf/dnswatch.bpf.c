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
  char comm[80];
  char cmdline[120];
  int sock_port_nr;
  char fn_id;
};

// dnswatch_kprobe_output_events adds events in perf map
// BPF_PERF_OUTPUT(dnswatch_kprobe_output_events);
struct {
  __uint(type, BPF_MAP_TYPE_RINGBUF);
  __uint(max_entries, 1 << 24);
} dnswatch_kprobe_output_events SEC(".maps");

// tgid_info value for hash map tgid_cmdline, used to map tgid to cmdline
struct tgid_info {
  // original_tgid is used to check for hash collisions
  u32 original_tgid;
  char cmdline[120];
};

// tgid_cmdline used to map tgid hashes to the cmdline of the process
// BPF_HASH(tgid_cmdline, u32, struct tgid_info, HASHMAP_SIZE);
struct {
  __uint(type, BPF_MAP_TYPE_HASH);
  __uint(max_entries, HASHMAP_SIZE);
  __type(key, u32);
  __type(value, struct tgid_info);
} tgid_cmdline SEC(".maps");

// Based on /sys/kernel/debug/tracing/events/syscalls/sys_enter_execve/format
struct execve_args {
  unsigned short common_type;
  unsigned char common_flags;
  unsigned char common_preempt_count;
  int common_pid;
  int __syscall_nr;
  const char* filename;
  const char* const* argv;
  const char* const* envp;
};

// syscall__execve maps tgids to cmdlines and populates tgid_filename
SEC("tracepoint/syscalls/sys_enter_execve")
int tp_syscall_execve(struct execve_args* ctx) {
  u32 __tgid = bpf_get_current_pid_tgid() >> 32;
  u32 __tgid_hash = __tgid % HASHMAP_SIZE;
  const char* arg;

  struct tgid_info __tgid_info = {0};

  __tgid_info.original_tgid = __tgid;

  __tgid_info.cmdline[0] = 0;

  for (int i = 0; i < 4; i++) {
    bpf_probe_read_kernel(&arg, sizeof(void*), &ctx->argv[i]);
    if (!arg)
      break;
    char* startByte = __tgid_info.cmdline + i * 30;
    u32 bytesToCopy = 30;
    char* lastByte = startByte + bytesToCopy - 1;

    bpf_probe_read_user(startByte, bytesToCopy, arg);
    *lastByte = 0;
  }

  bpf_map_update_elem(&tgid_cmdline, &__tgid_hash, &__tgid_info, 0);
  return 0;
}

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
  bpf_get_current_comm(&data->comm, sizeof(data->comm));
  data->sock_port_nr = (int)sport;
  data->fn_id = fn_id;

  u32 __tgid_hash = __tgid % HASHMAP_SIZE;
  // struct tgid_info* __tgid_info = tgid_cmdline.lookup(&__tgid_hash);
  struct tgid_info* __tgid_info =
      bpf_map_lookup_elem(&tgid_cmdline, &__tgid_hash);
  if (__tgid_info == 0 || __tgid_info->original_tgid != __tgid) {
    data->cmdline[0] = 0;
  } else {
    memcpy(data->cmdline, __tgid_info->cmdline, sizeof(data->cmdline));
  }

  bpf_ringbuf_submit(data, 0);
  return 0;
}

SEC("kprobe/udpv6_sendmsg")
int BPF_KPROBE(
    dnswatch_kprobe_udpv6_sendmsg,
    struct sock* sk,
    struct msghdr* msg) {
  struct inet_sock* inet = (struct inet_sock*)sk;
  struct sockaddr_in6* sin6;
  u16 dport, sport;

  bpf_probe_read_kernel(&sin6, sizeof(void*), &msg->msg_name);

  // handle connectionless udp ipv6 sockets. If the process did not call
  // connect(udp_fc,...) the dport is set to 0 in struct sock, so we need to get
  // the dport from (struct msghdr*)msg->(struct
  // sockaddr_in6*)msg_name->sin6_port.
  if (sin6) {
    bpf_probe_read_kernel(&dport, sizeof(dport), &sin6->sin6_port);
  } else {
    bpf_probe_read_kernel(&dport, sizeof(dport), &sk->__sk_common.skc_dport);
  }
  bpf_probe_read_kernel(&sport, sizeof(sport), &sk->__sk_common.skc_num);

  return sendmsg_solver(ctx, 0, dport, sport);
}

SEC("kprobe/udp_sendmsg")
int BPF_KPROBE(
    dnswatch_kprobe_udp_sendmsg,
    struct sock* sk,
    struct msghdr* msg) {
  struct inet_sock* inet = (struct inet_sock*)sk;
  struct sockaddr_in* sin;
  u16 dport, sport;

  bpf_probe_read_kernel(&sin, sizeof(void*), &msg->msg_name);

  // handle connectionless udp ipv4 sockets. Same as udp ipv6, but different
  // structs and fields.
  if (sin) {
    bpf_probe_read_kernel(&dport, sizeof(dport), &sin->sin_port);
  } else {
    bpf_probe_read_kernel(&dport, sizeof(dport), &sk->__sk_common.skc_dport);
  }
  bpf_probe_read_kernel(&sport, sizeof(sport), &sk->__sk_common.skc_num);

  return sendmsg_solver(ctx, 1, dport, sport);
}

SEC("kprobe/tcp_sendmsg")
int BPF_KPROBE(
    dnswatch_kprobe_tcp_sendmsg,
    struct sock* sk,
    struct msghdr* msg) {
  u16 dport, sport;

  bpf_probe_read_kernel(&dport, sizeof(dport), &sk->__sk_common.skc_dport);
  bpf_probe_read_kernel(&sport, sizeof(sport), &sk->__sk_common.skc_num);

  return sendmsg_solver(ctx, 2, dport, dport);
}

char LICENSE[] SEC("license") = "GPL";
