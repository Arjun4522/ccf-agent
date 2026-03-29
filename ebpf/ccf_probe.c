// ccf_probe.c — eBPF probe loaded by the collector.
//
// Attaches to tracepoints for the events the CCF agent cares about.
// Compiled at runtime by cilium/ebpf via go:generate; do not build manually.
//
// Kernel requirement: 4.18+ for ring buffer / perf_event_output.
// Run with: CAP_BPF + CAP_PERFMON (kernel ≥ 5.8) or CAP_SYS_ADMIN.

//go:build ignore

#include "vmlinux.h"
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_tracing.h>
#include <bpf/bpf_core_read.h>

#define MAX_PATH 256
#define TASK_COMM_LEN 16

// ---------------------------------------------------------------------------
// Shared event struct written into the perf ring buffer.
// Must stay in sync with collector/probe_types.go (same field layout).
// ---------------------------------------------------------------------------
struct ccf_event {
    __u64 timestamp_ns;
    __u32 pid;
    __u32 ppid;
    char  comm[TASK_COMM_LEN];
    __u32 event_type;   // maps to EventType enum below
    __u32 uid;
    __u32 gid;
    char  path[MAX_PATH];
    char  dst_path[MAX_PATH]; // rename only
};

// event_type values — keep in sync with collector/probe_types.go
#define EVT_FILE_OPEN   0
#define EVT_FILE_WRITE  1
#define EVT_FILE_RENAME 2
#define EVT_FILE_DELETE 3
#define EVT_EXEC        4
#define EVT_SETUID      5

// ---------------------------------------------------------------------------
// Per-CPU scratch map — avoids placing the large ccf_event on the BPF stack
// (BPF stack limit is 512 bytes; ccf_event is ~556 bytes).
// ---------------------------------------------------------------------------
struct {
    __uint(type, BPF_MAP_TYPE_PERCPU_ARRAY);
    __uint(max_entries, 1);
    __type(key, __u32);
    __type(value, struct ccf_event);
} ccf_scratch SEC(".maps");

// ---------------------------------------------------------------------------
// Perf event output map — one slot per CPU.
// ---------------------------------------------------------------------------
struct {
    __uint(type, BPF_MAP_TYPE_PERF_EVENT_ARRAY);
    __uint(key_size, sizeof(__u32));
    __uint(value_size, sizeof(__u32));
} ccf_events SEC(".maps");

// ---------------------------------------------------------------------------
// Filter map — PID blocklist (agent itself, init, kthreads).
// Userspace fills this at startup.
// ---------------------------------------------------------------------------
struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, 256);
    __type(key, __u32);
    __type(value, __u8);
} pid_filter SEC(".maps");

// ---------------------------------------------------------------------------
// Helper: get a zeroed scratch event pointer (per-CPU, never NULL).
// ---------------------------------------------------------------------------
static __always_inline struct ccf_event *get_scratch(void) {
    __u32 key = 0;
    return bpf_map_lookup_elem(&ccf_scratch, &key);
}

// ---------------------------------------------------------------------------
// Helper: populate common fields from current task.
// ---------------------------------------------------------------------------
static __always_inline void fill_common(struct ccf_event *e) {
    struct task_struct *task = (struct task_struct *)bpf_get_current_task();
    e->timestamp_ns = bpf_ktime_get_ns();
    e->pid  = bpf_get_current_pid_tgid() >> 32;
    e->ppid = BPF_CORE_READ(task, real_parent, tgid);
    e->uid  = bpf_get_current_uid_gid() & 0xFFFFFFFF;
    e->gid  = bpf_get_current_uid_gid() >> 32;
    bpf_get_current_comm(e->comm, sizeof(e->comm));
}

static __always_inline int is_filtered(__u32 pid) {
    __u8 *v = bpf_map_lookup_elem(&pid_filter, &pid);
    return v != NULL;
}

// ---------------------------------------------------------------------------
// Tracepoint: sys_enter_openat (file open for write)
// ---------------------------------------------------------------------------
SEC("tracepoint/syscalls/sys_enter_openat")
int tp_openat(struct trace_event_raw_sys_enter *ctx) {
    __u32 pid = bpf_get_current_pid_tgid() >> 32;
    if (is_filtered(pid)) return 0;

    int flags = (int)ctx->args[2];
    // Only care about opens that could write (O_WRONLY=1, O_RDWR=2)
    if (!(flags & 3)) return 0;

    struct ccf_event *e = get_scratch();
    if (!e) return 0;
    __builtin_memset(e, 0, sizeof(*e));

    fill_common(e);
    e->event_type = EVT_FILE_OPEN;
    bpf_probe_read_user_str(e->path, sizeof(e->path), (void *)ctx->args[1]);
    bpf_perf_event_output(ctx, &ccf_events, BPF_F_CURRENT_CPU, e, sizeof(*e));
    return 0;
}

// ---------------------------------------------------------------------------
// Tracepoint: sys_enter_write
// ---------------------------------------------------------------------------
SEC("tracepoint/syscalls/sys_enter_write")
int tp_write(struct trace_event_raw_sys_enter *ctx) {
    __u32 pid = bpf_get_current_pid_tgid() >> 32;
    if (is_filtered(pid)) return 0;

    struct ccf_event *e = get_scratch();
    if (!e) return 0;
    __builtin_memset(e, 0, sizeof(*e));

    fill_common(e);
    e->event_type = EVT_FILE_WRITE;
    bpf_perf_event_output(ctx, &ccf_events, BPF_F_CURRENT_CPU, e, sizeof(*e));
    return 0;
}

// ---------------------------------------------------------------------------
// Tracepoint: sys_enter_renameat2
// ---------------------------------------------------------------------------
SEC("tracepoint/syscalls/sys_enter_renameat2")
int tp_rename(struct trace_event_raw_sys_enter *ctx) {
    __u32 pid = bpf_get_current_pid_tgid() >> 32;
    if (is_filtered(pid)) return 0;

    struct ccf_event *e = get_scratch();
    if (!e) return 0;
    __builtin_memset(e, 0, sizeof(*e));

    fill_common(e);
    e->event_type = EVT_FILE_RENAME;
    bpf_probe_read_user_str(e->path,     sizeof(e->path),     (void *)ctx->args[1]);
    bpf_probe_read_user_str(e->dst_path, sizeof(e->dst_path), (void *)ctx->args[3]);
    bpf_perf_event_output(ctx, &ccf_events, BPF_F_CURRENT_CPU, e, sizeof(*e));
    return 0;
}

// ---------------------------------------------------------------------------
// Tracepoint: sys_enter_unlinkat (file delete)
// ---------------------------------------------------------------------------
SEC("tracepoint/syscalls/sys_enter_unlinkat")
int tp_unlink(struct trace_event_raw_sys_enter *ctx) {
    __u32 pid = bpf_get_current_pid_tgid() >> 32;
    if (is_filtered(pid)) return 0;

    struct ccf_event *e = get_scratch();
    if (!e) return 0;
    __builtin_memset(e, 0, sizeof(*e));

    fill_common(e);
    e->event_type = EVT_FILE_DELETE;
    bpf_probe_read_user_str(e->path, sizeof(e->path), (void *)ctx->args[1]);
    bpf_perf_event_output(ctx, &ccf_events, BPF_F_CURRENT_CPU, e, sizeof(*e));
    return 0;
}

// ---------------------------------------------------------------------------
// Tracepoint: sched_process_exec
// ---------------------------------------------------------------------------
SEC("tracepoint/sched/sched_process_exec")
int tp_exec(struct trace_event_raw_sched_process_exec *ctx) {
    __u32 pid = bpf_get_current_pid_tgid() >> 32;
    if (is_filtered(pid)) return 0;

    struct ccf_event *e = get_scratch();
    if (!e) return 0;
    __builtin_memset(e, 0, sizeof(*e));

    fill_common(e);
    e->event_type = EVT_EXEC;
    unsigned short fname_off = ctx->__data_loc_filename & 0xFFFF;
    bpf_probe_read_kernel_str(e->path, sizeof(e->path),
                              (void *)ctx + fname_off);
    bpf_perf_event_output(ctx, &ccf_events, BPF_F_CURRENT_CPU, e, sizeof(*e));
    return 0;
}

// ---------------------------------------------------------------------------
// Tracepoint: sys_enter_setuid (privilege escalation)
// ---------------------------------------------------------------------------
SEC("tracepoint/syscalls/sys_enter_setuid")
int tp_setuid(struct trace_event_raw_sys_enter *ctx) {
    __u32 pid = bpf_get_current_pid_tgid() >> 32;
    if (is_filtered(pid)) return 0;

    __u32 target_uid = (__u32)ctx->args[0];
    if (target_uid != 0) return 0; // only care about escalation to root

    struct ccf_event *e = get_scratch();
    if (!e) return 0;
    __builtin_memset(e, 0, sizeof(*e));

    fill_common(e);
    e->event_type = EVT_SETUID;
    bpf_perf_event_output(ctx, &ccf_events, BPF_F_CURRENT_CPU, e, sizeof(*e));
    return 0;
}

char _license[] SEC("license") = "GPL";