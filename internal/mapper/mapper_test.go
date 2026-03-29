package mapper_test

import (
	"context"
	"testing"
	"time"

	"github.com/ccf-agent/internal/mapper"
	"github.com/ccf-agent/pkg/event"
	"go.uber.org/zap"
)

var testLog = zap.NewNop()

func newMapper() *mapper.Mapper {
	return mapper.New(mapper.DefaultConfig(), testLog)
}

func TestCapabilityMapping(t *testing.T) {
	tests := []struct {
		name    string
		raw     event.RawEvent
		wantCap event.Capability
		wantOK  bool
	}{
		{
			name:    "file write → WRITE",
			raw:     event.RawEvent{Type: event.FileWrite, Path: "/home/user/report.txt", ProcessName: "vim"},
			wantCap: event.CapWrite,
			wantOK:  true,
		},
		{
			name:    "file rename to .enc → CRYPTO",
			raw:     event.RawEvent{Type: event.FileRename, Path: "/home/user/file.txt", DstPath: "/home/user/file.enc", ProcessName: "malware"},
			wantCap: event.CapCrypto,
			wantOK:  true,
		},
		{
			name:    "file delete → DELETE",
			raw:     event.RawEvent{Type: event.FileDelete, Path: "/home/user/file.txt", ProcessName: "rm"},
			wantCap: event.CapDelete,
			wantOK:  true,
		},
		{
			name:    "exec → EXEC",
			raw:     event.RawEvent{Type: event.Exec, Path: "/usr/bin/bash", ProcessName: "bash"},
			wantCap: event.CapExec,
			wantOK:  true,
		},
		{
			name:    "setuid → PRIV_ESC",
			raw:     event.RawEvent{Type: event.SetUID, ProcessName: "sudo"},
			wantCap: event.CapPrivEsc,
			wantOK:  true,
		},
		{
			name:   "empty process name → dropped",
			raw:    event.RawEvent{Type: event.FileWrite, Path: "/proc/123/fd/4"},
			wantOK: false,
		},
		{
			name:   "proc virtual FS → dropped",
			raw:    event.RawEvent{Type: event.FileWrite, Path: "/proc/self/maps", ProcessName: "ps"},
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in  := make(chan event.RawEvent, 1)
			out := make(chan event.MappedEvent, 1)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			m := newMapper()
			go m.Run(ctx, in, out)

			in <- tt.raw
			close(in)

			select {
			case mapped := <-out:
				if !tt.wantOK {
					t.Fatalf("expected event to be dropped, got %+v", mapped)
				}
				if mapped.Capability != tt.wantCap {
					t.Errorf("capability: got %q want %q", mapped.Capability, tt.wantCap)
				}
			case <-time.After(50 * time.Millisecond):
				if tt.wantOK {
					t.Fatal("expected mapped event, got none")
				}
			}
		})
	}
}

func TestNodeIDCluster(t *testing.T) {
	tests := []struct {
		path   string
		wantID string
	}{
		{"/home/user/docs/report.docx", "/home/user/docs"},
		{"/home/user/images/photo.jpg", "/home/user/images"},
		{"/var/log/syslog",             "/var/log"},
		{"/tmp/x",                      "/tmp"},
	}

	m := newMapper()
	for _, tt := range tests {
		raw := event.RawEvent{
			Type:        event.FileWrite,
			Path:        tt.path,
			ProcessName: "proc",
		}
		in  := make(chan event.RawEvent, 1)
		out := make(chan event.MappedEvent, 1)

		ctx, cancel := context.WithCancel(context.Background())
		go m.Run(ctx, in, out)

		in <- raw
		select {
		case mapped := <-out:
			if mapped.NodeID != tt.wantID {
				t.Errorf("path %q: nodeID got %q want %q", tt.path, mapped.NodeID, tt.wantID)
			}
		case <-time.After(50 * time.Millisecond):
			t.Errorf("path %q: no mapped event", tt.path)
		}
		cancel()
	}
}