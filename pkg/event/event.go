// Package event defines the core data types that flow through the CCF pipeline.
// Every layer communicates exclusively through these types.
package event

// Type is a raw kernel event category captured by the eBPF probe.
type Type string

const (
	FileOpen   Type = "file_open"
	FileWrite  Type = "file_write"
	FileRename Type = "file_rename"
	FileDelete Type = "file_delete"
	Exec       Type = "exec"
	SetUID     Type = "setuid"
)

// RawEvent is the uninterpreted kernel event emitted by the eBPF probe.
// Fields map directly to what the perf ring buffer delivers.
type RawEvent struct {
	TimestampNS uint64 // kernel monotonic clock, nanoseconds
	PID         uint32
	PPID        uint32
	ProcessName string // comm field, max 16 bytes from kernel
	Type        Type
	Path        string // absolute path; empty for non-file events
	DstPath     string // rename target; empty for non-rename events
	UID         uint32
	GID         uint32
}

// Capability is the semantic intent inferred from a RawEvent by the mapper.
type Capability string

const (
	CapWrite   Capability = "WRITE"
	CapRename  Capability = "RENAME"
	CapDelete  Capability = "DELETE"
	CapExec    Capability = "EXEC"
	CapPrivEsc Capability = "PRIV_ESC"
	CapCrypto  Capability = "CRYPTO" // reserved; detected via heuristic
)

// CapabilityWeight is the field intensity contribution of each capability.
// Higher weight = stronger influence on the field.
var CapabilityWeight = map[Capability]float64{
	CapWrite:   0.6,
	CapRename:  0.8,
	CapDelete:  1.0,
	CapExec:    0.5,
	CapPrivEsc: 1.2,
	CapCrypto:  1.5,
}

// MappedEvent is a RawEvent enriched with its inferred Capability.
// This is the currency of the mapper → field constructor interface.
type MappedEvent struct {
	Raw        RawEvent
	Capability Capability
	NodeID     string  // canonical node identifier resolved by the mapper
	Weight     float64 // pre-looked-up from CapabilityWeight
	PPID       uint32  // parent PID for process tree analysis
}
