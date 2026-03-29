// Package mapper translates raw kernel events into semantic capability events.
//
// Two responsibilities:
//  1. Map event.Type → event.Capability (what the process is doing)
//  2. Resolve a canonical NodeID for the affected resource (where it happened)
//
// Node resolution strategy:
//   - File events → directory cluster (e.g. "/home/user/docs" → "/home/user/docs")
//   - Exec events → "proc:<pid>" node
//   - Priv events → "priv:root" node
//
// The mapper is stateless and goroutine-safe.
package mapper

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/ccf-agent/pkg/event"
	"go.uber.org/zap"
)

// Config holds tunable mapper parameters.
type Config struct {
	// DirectoryDepth controls how many path components are kept when building
	// the NodeID from a file path. 2 means "/home/user/docs/file.txt" → "/home/user".
	// Lower = coarser field granularity, fewer nodes, less memory.
	DirectoryDepth int

	// CryptoExtensions is the set of file extensions that trigger a CapCrypto
	// capability (ransomware commonly writes encrypted files with custom extensions).
	CryptoExtensions []string
}

func DefaultConfig() Config {
	return Config{
		DirectoryDepth: 3,
		CryptoExtensions: []string{
			".enc", ".locked", ".crypt", ".crypted", ".encrypted",
			".vault", ".pay", ".ransom",
		},
	}
}

// Mapper converts RawEvents to MappedEvents.
type Mapper struct {
	cfg        Config
	log        *zap.Logger
	cryptoExts map[string]struct{}
}

func New(cfg Config, log *zap.Logger) *Mapper {
	exts := make(map[string]struct{}, len(cfg.CryptoExtensions))
	for _, e := range cfg.CryptoExtensions {
		exts[strings.ToLower(e)] = struct{}{}
	}
	return &Mapper{cfg: cfg, log: log, cryptoExts: exts}
}

// Run reads from in, maps each event, and writes to out.
// Returns when ctx is cancelled.
func (m *Mapper) Run(ctx context.Context, in <-chan event.RawEvent, out chan<- event.MappedEvent) {
	for {
		select {
		case <-ctx.Done():
			return
		case raw, ok := <-in:
			if !ok {
				return
			}
			mapped, ok := m.mapEvent(raw)
			if !ok {
				continue // event filtered out (e.g. kernel internal paths)
			}
			select {
			case out <- mapped:
			case <-ctx.Done():
				return
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Core mapping logic
// ---------------------------------------------------------------------------

func (m *Mapper) mapEvent(raw event.RawEvent) (event.MappedEvent, bool) {
	cap, ok := m.resolveCapability(raw)
	if !ok {
		return event.MappedEvent{}, false
	}

	nodeID := m.resolveNode(raw)

	return event.MappedEvent{
		Raw:        raw,
		Capability: cap,
		NodeID:     nodeID,
		Weight:     event.CapabilityWeight[cap],
	}, true
}

// resolveCapability maps a RawEvent to the highest-severity applicable
// Capability. Returns false if the event should be dropped.
func (m *Mapper) resolveCapability(raw event.RawEvent) (event.Capability, bool) {
	// Drop events with no process name or virtual/kernel FS path prefixes.
	if raw.ProcessName == "" {
		return "", false
	}
	for _, prefix := range []string{"pipe:", "/proc/", "/sys/", "/dev/"} {
		if strings.HasPrefix(raw.Path, prefix) {
			return "", false
		}
	}

	switch raw.Type {
	case event.FileWrite, event.FileOpen:
		// Upgrade to CRYPTO if the path has a known ransomware extension.
		if m.hasCryptoExt(raw.Path) {
			return event.CapCrypto, true
		}
		return event.CapWrite, true

	case event.FileRename:
		// Rename to a crypto extension is a strong ransomware signal.
		if m.hasCryptoExt(raw.DstPath) {
			return event.CapCrypto, true
		}
		return event.CapRename, true

	case event.FileDelete:
		return event.CapDelete, true

	case event.Exec:
		return event.CapExec, true

	case event.SetUID:
		return event.CapPrivEsc, true

	default:
		return "", false
	}
}

// resolveNode returns a canonical node identifier for the resource affected
// by the event. File paths are bucketed to a directory cluster at the
// configured depth to keep the field sparse.
func (m *Mapper) resolveNode(raw event.RawEvent) string {
	switch raw.Type {
	case event.FileOpen, event.FileWrite, event.FileRename, event.FileDelete:
		path := raw.Path
		if raw.Type == event.FileRename {
			// Use destination path for rename — that's where influence lands.
			path = raw.DstPath
		}
		return m.clusterPath(path)

	case event.Exec:
		// TODO: consider clustering by binary directory rather than by PID
		// to detect mass process spawning (e.g. ransomware workers).
		return "proc:" + raw.ProcessName

	case event.SetUID:
		return "priv:root"

	default:
		return "unknown"
	}
}

// clusterPath truncates a file path to cfg.DirectoryDepth components.
// "/home/user/docs/report.docx" with depth=3 → "/home/user/docs"
func (m *Mapper) clusterPath(path string) string {
	if path == "" {
		return "unknown"
	}
	// Ignore proc, sys, dev virtual filesystems — no ransomware interest there.
	for _, prefix := range []string{"/proc/", "/sys/", "/dev/"} {
		if strings.HasPrefix(path, prefix) {
			return ""
		}
	}

	dir := filepath.Dir(path)
	parts := strings.Split(strings.TrimPrefix(dir, "/"), "/")

	depth := m.cfg.DirectoryDepth
	if len(parts) < depth {
		depth = len(parts)
	}
	return "/" + strings.Join(parts[:depth], "/")
}

func (m *Mapper) hasCryptoExt(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	_, ok := m.cryptoExts[ext]
	return ok
}
