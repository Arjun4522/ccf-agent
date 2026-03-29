// Package responder takes blocking action when the detector fires.
//
// Three mechanisms, applied in order of escalation:
//  1. SIGSTOP  — pause the process on WARNING (reversible)
//  2. SIGKILL  — terminate on ALERT
//  3. Quarantine — move encrypted files to a safe directory
package responder

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ccf-agent/internal/detector"
	"go.uber.org/zap"
)

type Config struct {
	// QuarantineDir is where suspicious files are moved on ALERT.
	QuarantineDir string

	// KillOnAlert: send SIGKILL to the offending PID on ALERT.
	KillOnAlert bool

	// PauseOnWarning: send SIGSTOP to the offending PID on WARNING.
	// The process is resumed automatically if the score drops within ResumeWindow.
	PauseOnWarning bool

	// ResumeWindow: if severity drops back to NONE within this duration,
	// a SIGSTOP'd process is resumed with SIGCONT.
	ResumeWindow time.Duration

	// CooldownWindow: don't act on the same PID twice within this window.
	CooldownWindow time.Duration

	// DryRun: log what would happen but don't actually send signals or move files.
	DryRun bool
}

func DefaultConfig() Config {
	return Config{
		QuarantineDir:  "/var/lib/ccf-agent/quarantine",
		KillOnAlert:    true,
		PauseOnWarning: true,
		ResumeWindow:   10 * time.Second,
		CooldownWindow: 30 * time.Second,
		DryRun:         false,
	}
}

// Responder reacts to detections with blocking actions.
type Responder struct {
	cfg      Config
	log      *zap.Logger
	mu       sync.Mutex
	actioned map[uint32]time.Time // PID → last action time
	paused   map[uint32]time.Time // PID → time of SIGSTOP (for auto-resume)
}

func New(cfg Config, log *zap.Logger) *Responder {
	return &Responder{
		cfg:      cfg,
		log:      log,
		actioned: make(map[uint32]time.Time),
		paused:   make(map[uint32]time.Time),
	}
}

// Run reads detections and acts on them until ctx is cancelled.
func (r *Responder) Run(ctx context.Context, in <-chan detector.Detection) {
	resumeTicker := time.NewTicker(2 * time.Second)
	defer resumeTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			r.resumeAll()
			return

		case <-resumeTicker.C:
			r.checkResume()

		case det, ok := <-in:
			if !ok {
				return
			}
			r.handle(det)
		}
	}
}

// handle dispatches a Detection to the appropriate action.
func (r *Responder) handle(det detector.Detection) {
	pid := det.Vector.OffenderPID
	if pid == 0 {
		// No PID attribution yet — quarantine only.
		r.log.Warn("detection without PID — file quarantine only",
			zap.String("severity", det.Severity.String()),
			zap.Float64("score", det.Score),
		)
		r.quarantineRecentFiles(det)
		return
	}

	// Cooldown: don't hammer the same PID repeatedly.
	r.mu.Lock()
	if last, ok := r.actioned[pid]; ok && time.Since(last) < r.cfg.CooldownWindow {
		r.mu.Unlock()
		return
	}
	r.actioned[pid] = time.Now()
	r.mu.Unlock()

	switch det.Severity {
	case detector.SeverityWarning:
		if r.cfg.PauseOnWarning {
			r.pausePID(pid, det)
		}

	case detector.SeverityAlert:
		// First pause to stop new file writes, then quarantine, then kill.
		r.pausePID(pid, det)
		r.quarantineRecentFiles(det)
		if r.cfg.KillOnAlert {
			r.killPID(pid, det)
		}
	}
}

// pausePID sends SIGSTOP to freeze the process.
func (r *Responder) pausePID(pid uint32, det detector.Detection) {
	r.log.Warn("PAUSING process (SIGSTOP)",
		zap.Uint32("pid", pid),
		zap.String("severity", det.Severity.String()),
		zap.Float64("score", det.Score),
		zap.Bool("dry_run", r.cfg.DryRun),
	)
	if r.cfg.DryRun {
		return
	}
	proc, err := os.FindProcess(int(pid))
	if err != nil {
		r.log.Error("find process failed", zap.Uint32("pid", pid), zap.Error(err))
		return
	}
	if err := proc.Signal(os.Signal(syscallSIGSTOP())); err != nil {
		r.log.Error("SIGSTOP failed", zap.Uint32("pid", pid), zap.Error(err))
		return
	}
	r.mu.Lock()
	r.paused[pid] = time.Now()
	r.mu.Unlock()
}

// killPID sends SIGKILL to terminate the process.
func (r *Responder) killPID(pid uint32, det detector.Detection) {
	r.log.Warn("KILLING process (SIGKILL)",
		zap.Uint32("pid", pid),
		zap.String("severity", det.Severity.String()),
		zap.Float64("score", det.Score),
		zap.Bool("dry_run", r.cfg.DryRun),
	)
	if r.cfg.DryRun {
		return
	}
	proc, err := os.FindProcess(int(pid))
	if err != nil {
		r.log.Error("find process failed", zap.Uint32("pid", pid), zap.Error(err))
		return
	}
	if err := proc.Kill(); err != nil {
		r.log.Error("SIGKILL failed", zap.Uint32("pid", pid), zap.Error(err))
	}
	r.mu.Lock()
	delete(r.paused, pid)
	r.mu.Unlock()
}

// quarantineRecentFiles moves files with ransomware extensions to the quarantine dir.
func (r *Responder) quarantineRecentFiles(det detector.Detection) {
	// Collect files recently written by the offending process from /proc/<pid>/fd
	pid := det.Vector.OffenderPID
	files := r.collectOpenFiles(pid)
	if len(files) == 0 {
		return
	}

	cryptoExts := map[string]bool{
		".enc": true, ".locked": true, ".crypt": true,
		".crypted": true, ".encrypted": true, ".vault": true,
		".pay": true, ".ransom": true,
	}

	if err := os.MkdirAll(r.cfg.QuarantineDir, 0700); err != nil {
		r.log.Error("create quarantine dir", zap.Error(err))
		return
	}

	for _, f := range files {
		ext := strings.ToLower(filepath.Ext(f))
		if !cryptoExts[ext] {
			continue
		}
		dst := filepath.Join(r.cfg.QuarantineDir, filepath.Base(f))
		r.log.Warn("quarantining file",
			zap.String("src", f),
			zap.String("dst", dst),
			zap.Bool("dry_run", r.cfg.DryRun),
		)
		if r.cfg.DryRun {
			continue
		}
		if err := os.Rename(f, dst); err != nil {
			r.log.Error("quarantine rename failed", zap.String("file", f), zap.Error(err))
			// Fallback: at least make it immutable.
			_ = exec.Command("chattr", "+i", f).Run()
		}
	}
}

// collectOpenFiles reads /proc/<pid>/fd to find currently open file paths.
func (r *Responder) collectOpenFiles(pid uint32) []string {
	if pid == 0 {
		return nil
	}
	fdDir := fmt.Sprintf("/proc/%d/fd", pid)
	entries, err := os.ReadDir(fdDir)
	if err != nil {
		return nil
	}
	var files []string
	for _, e := range entries {
		link, err := os.Readlink(filepath.Join(fdDir, e.Name()))
		if err != nil || !strings.HasPrefix(link, "/") {
			continue
		}
		files = append(files, link)
	}
	return files
}

// checkResume auto-resumes SIGSTOP'd processes after ResumeWindow.
func (r *Responder) checkResume() {
	r.mu.Lock()
	defer r.mu.Unlock()
	for pid, stoppedAt := range r.paused {
		if time.Since(stoppedAt) >= r.cfg.ResumeWindow {
			r.log.Info("auto-resuming process (SIGCONT)",
				zap.Uint32("pid", pid),
				zap.Duration("paused_for", time.Since(stoppedAt)),
			)
			if !r.cfg.DryRun {
				if proc, err := os.FindProcess(int(pid)); err == nil {
					_ = proc.Signal(os.Signal(syscallSIGCONT()))
				}
			}
			delete(r.paused, pid)
		}
	}
}

// resumeAll sends SIGCONT to all currently paused processes (called on shutdown).
func (r *Responder) resumeAll() {
	r.mu.Lock()
	defer r.mu.Unlock()
	for pid := range r.paused {
		if proc, err := os.FindProcess(int(pid)); err == nil {
			_ = proc.Signal(os.Signal(syscallSIGCONT()))
		}
	}
}

// PausedPIDs returns a snapshot of currently paused PIDs (for observability).
func (r *Responder) PausedPIDs() []uint32 {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]uint32, 0, len(r.paused))
	for pid := range r.paused {
		out = append(out, pid)
	}
	return out
}

// pidString is a helper for logging.
func pidString(pid uint32) string {
	return strconv.FormatUint(uint64(pid), 10)
}
