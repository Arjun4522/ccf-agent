// Package responder takes blocking action when the detector fires.
//
// Enhancement summary over the basic version:
//  1. Allowlist  — skip trusted PIDs and comm names (systemd, sshd, etc.)
//  2. Process-tree kill — walk /proc to find and kill child processes too
//  3. Network isolation — drop outbound traffic for the offending UID via iptables
//  4. Evidence snapshot — save cmdline, maps, open fds to disk before killing
//  5. Webhook / syslog alerting — HTTP POST to a SIEM or emit to syslog
package responder

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/syslog"
	"net/http"
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

// ---------------------------------------------------------------------------
// Config
// ---------------------------------------------------------------------------

type Config struct {
	// QuarantineDir is where suspicious files are moved on ALERT.
	QuarantineDir string

	// EvidenceDir is where process snapshots are written before a kill.
	EvidenceDir string

	// KillOnAlert: send SIGKILL to the offending PID on ALERT.
	KillOnAlert bool

	// KillProcessTree: also kill all child processes of the offending PID.
	// Prevents ransomware that forks workers from evading single-PID kill.
	KillProcessTree bool

	// PauseOnWarning: send SIGSTOP to the offending PID on WARNING.
	PauseOnWarning bool

	// IsolateNetwork: add an iptables rule dropping traffic for the
	// offending process's UID on ALERT. Stops C2 callbacks and exfiltration.
	// Requires root. Rules are removed on clean shutdown.
	IsolateNetwork bool

	// ResumeWindow: if severity drops within this duration after SIGSTOP,
	// the process is resumed with SIGCONT.
	ResumeWindow time.Duration

	// CooldownWindow: don't act on the same PID twice within this window.
	CooldownWindow time.Duration

	// RespawnWindow: track and re-alert if the same script path is executed
	// again within this window after being killed.
	RespawnWindow time.Duration

	// BlockRespawnedScripts: rename suspicious script files after killing
	// to prevent re-execution (anti-respawn mechanism).
	BlockRespawnedScripts bool

	// AllowlistComms: process names that will never be acted on.
	// e.g. ["systemd", "sshd", "journald", "NetworkManager"]
	AllowlistComms []string

	// AllowlistPIDs: specific PIDs that will never be acted on (e.g. PID 1).
	AllowlistPIDs []uint32

	// WebhookURL: if non-empty, POST a JSON alert payload to this URL on
	// every detection. Compatible with Slack, PagerDuty, or any HTTP sink.
	WebhookURL string

	// WebhookTimeout: HTTP client timeout for webhook calls.
	WebhookTimeout time.Duration

	// UseSyslog: emit detections to the system syslog (LOG_AUTHPRIV | LOG_CRIT).
	UseSyslog bool

	// DryRun: log what would happen but don't actually send signals,
	// modify iptables, or move files.
	DryRun bool
}

func DefaultConfig() Config {
	return Config{
		QuarantineDir:         "/var/lib/ccf-agent/quarantine",
		EvidenceDir:           "/var/lib/ccf-agent/evidence",
		KillOnAlert:           true,
		KillProcessTree:       true,
		PauseOnWarning:        true,
		IsolateNetwork:        false, // opt-in — has side effects
		ResumeWindow:          10 * time.Second,
		CooldownWindow:        30 * time.Second,
		RespawnWindow:         5 * time.Minute, // track scripts for 5 minutes after kill
		BlockRespawnedScripts: true,            // rename script files to prevent respawn
		AllowlistComms: []string{
			"systemd", "systemd-journal", "systemd-udevd",
			"sshd", "journald", "NetworkManager",
			"dbus-daemon", "polkitd", "ccf-agent",
		},
		AllowlistPIDs:  []uint32{1}, // always spare init/systemd
		WebhookURL:     "",
		WebhookTimeout: 5 * time.Second,
		UseSyslog:      false,
		DryRun:         false,
	}
}

// ---------------------------------------------------------------------------
// Responder
// ---------------------------------------------------------------------------

// Responder reacts to detections with blocking actions.
type Responder struct {
	cfg            Config
	log            *zap.Logger
	mu             sync.Mutex
	actioned       map[uint32]time.Time // PID → last action time
	paused         map[uint32]time.Time // PID → SIGSTOP time
	isolatedUIDs   map[uint32]bool      // UIDs with active iptables DROP rules
	allowlistComms map[string]bool
	allowlistPIDs  map[uint32]bool
	httpClient     *http.Client
	syslogWriter   *syslog.Writer // nil if UseSyslog=false
	// Respawn tracking: tracks recently killed script paths to detect re-execution
	// Key: resolved script path, Value: last kill time
	killedScripts map[string]time.Time
}

func New(cfg Config, log *zap.Logger) *Responder {
	comms := make(map[string]bool, len(cfg.AllowlistComms))
	for _, c := range cfg.AllowlistComms {
		comms[c] = true
	}
	pids := make(map[uint32]bool, len(cfg.AllowlistPIDs))
	for _, p := range cfg.AllowlistPIDs {
		pids[p] = true
	}

	r := &Responder{
		cfg:            cfg,
		log:            log,
		actioned:       make(map[uint32]time.Time),
		paused:         make(map[uint32]time.Time),
		isolatedUIDs:   make(map[uint32]bool),
		allowlistComms: comms,
		allowlistPIDs:  pids,
		httpClient:     &http.Client{Timeout: cfg.WebhookTimeout},
		killedScripts:  make(map[string]time.Time),
	}

	if cfg.UseSyslog {
		w, err := syslog.New(syslog.LOG_AUTHPRIV|syslog.LOG_CRIT, "ccf-agent")
		if err != nil {
			log.Warn("syslog unavailable", zap.Error(err))
		} else {
			r.syslogWriter = w
		}
	}

	return r
}

// Run reads detections and acts on them until ctx is cancelled.
func (r *Responder) Run(ctx context.Context, in <-chan detector.Detection) {
	resumeTicker := time.NewTicker(2 * time.Second)
	defer resumeTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			r.shutdown()
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

// ---------------------------------------------------------------------------
// Core dispatch
// ---------------------------------------------------------------------------

func (r *Responder) handle(det detector.Detection) {
	pid := det.Vector.OffenderPID
	parentPID := det.Vector.ParentPID

	// Always send alerts regardless of PID.
	go r.sendAlerts(det)

	if pid == 0 {
		r.log.Warn("no PID — quarantine only",
			zap.String("severity", det.Severity.String()),
			zap.Float64("score", det.Score),
		)
		r.quarantineRecentFiles(det)
		return
	}

	// Allowlist check — never act on trusted processes.
	if r.isAllowlisted(pid) {
		r.log.Info("skipping allowlisted process",
			zap.Uint32("pid", pid),
			zap.String("comm", r.commForPID(pid)),
		)
		return
	}

	// When a child process (e.g. subshell) is detected doing suspicious
	// activity, also kill the parent orchestrator to prevent respawning.
	targetPID := pid
	if parentPID != 0 && !r.isAllowlisted(parentPID) {
		targetPID = parentPID
		r.log.Info("targeting parent orchestrator",
			zap.Uint32("child_pid", pid),
			zap.Uint32("parent_pid", parentPID),
		)
	}

	// Respawn detection: check if this script was recently killed
	scriptPath := r.getScriptPath(targetPID)
	if scriptPath != "" {
		r.mu.Lock()
		if lastKill, wasKilled := r.killedScripts[scriptPath]; wasKilled {
			if time.Since(lastKill) < r.cfg.RespawnWindow {
				r.log.Warn("RESPAWN DETECTED — script re-executed after recent kill",
					zap.String("script", scriptPath),
					zap.Uint32("pid", targetPID),
					zap.Duration("since_last_kill", time.Since(lastKill)),
				)
				// Immediately escalate: skip cooldown, block the script
				if r.cfg.BlockRespawnedScripts {
					r.blockScript(scriptPath)
				}
			}
		}
		r.mu.Unlock()
	}

	// Cooldown — don't hammer the same PID.
	r.mu.Lock()
	if last, ok := r.actioned[targetPID]; ok && time.Since(last) < r.cfg.CooldownWindow {
		r.mu.Unlock()
		return
	}
	r.actioned[targetPID] = time.Now()
	r.mu.Unlock()

	switch det.Severity {
	case detector.SeverityWarning:
		if r.cfg.PauseOnWarning {
			r.pausePID(targetPID, det)
		}

	case detector.SeverityAlert:
		// Sequence: snapshot → pause → quarantine → network isolate → kill tree.
		// Snapshot first so we capture the process before it disappears.
		r.captureEvidence(targetPID, det)
		r.pausePID(targetPID, det)
		r.quarantineRecentFiles(det)
		if r.cfg.IsolateNetwork {
			r.isolateNetworkForPID(targetPID)
		}
		if r.cfg.KillOnAlert {
			if r.cfg.KillProcessTree {
				r.killProcessTree(targetPID, det)
			} else {
				r.killPID(targetPID, det)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Enhancement 1: Allowlist
// ---------------------------------------------------------------------------

func (r *Responder) isAllowlisted(pid uint32) bool {
	if r.allowlistPIDs[pid] {
		return true
	}
	comm := r.commForPID(pid)
	return r.allowlistComms[comm]
}

func (r *Responder) commForPID(pid uint32) string {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/comm", pid))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// ---------------------------------------------------------------------------
// Enhancement 2: Process-tree kill
// ---------------------------------------------------------------------------

// killProcessTree sends SIGKILL to pid and all its descendants.
// It walks /proc once to build the parent→children map, then does a
// depth-first traversal starting from pid. Also kills the process group
// to handle parallel subshells spawned with (...) syntax.
func (r *Responder) killProcessTree(rootPID uint32, det detector.Detection) {
	// Track the script path for respawn detection
	scriptPath := r.getScriptPath(rootPID)

	// First, try to kill the entire process group (handles parallel ransomware)
	r.killProcessGroup(rootPID, det)

	children := r.buildChildMap()
	pids := collectDescendants(rootPID, children)
	// Kill leaves first, then root — avoids parent re-spawning children.
	for i := len(pids) - 1; i >= 0; i-- {
		r.killPID(pids[i], det)
	}

	// Track killed script for respawn detection
	if scriptPath != "" {
		r.mu.Lock()
		r.killedScripts[scriptPath] = time.Now()
		r.mu.Unlock()
	}
}

// killProcessGroup sends SIGKILL to all processes in the same process group
// as pid. This catches parallel subshells spawned with (...) syntax.
func (r *Responder) killProcessGroup(pid uint32, det detector.Detection) {
	pgid, err := getProcessGroupID(pid)
	if err != nil {
		r.log.Debug("could not get PGID for process group kill",
			zap.Uint32("pid", pid),
			zap.Error(err),
		)
		return
	}
	// Skip if already group 1 (init/systemd territory)
	if pgid == 1 {
		return
	}

	r.log.Warn("KILLING process group (SIGKILL)",
		zap.Uint32("pid", pid),
		zap.Int("pgid", pgid),
		zap.String("severity", det.Severity.String()),
	)

	if r.cfg.DryRun {
		return
	}

	// kill -9 -<pgid> kills all processes in the group
	cmd := exec.Command("kill", "-9", "-"+strconv.Itoa(pgid))
	if err := cmd.Run(); err != nil {
		r.log.Error("process group kill failed",
			zap.Int("pgid", pgid),
			zap.Error(err),
		)
	}
}

// getProcessGroupID returns the process group ID for a given PID.
func getProcessGroupID(pid uint32) (int, error) {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return 0, err
	}
	// Format: pid (comm) state ppid pgrp session tty_nr tpgid ...
	// Extract pgrp (5th field after comm in parentheses)
	fields := strings.Fields(string(data))
	if len(fields) < 5 {
		return 0, fmt.Errorf("unexpected stat format")
	}
	pgid, err := strconv.Atoi(fields[3])
	if err != nil {
		return 0, err
	}
	return pgid, nil
}

// getScriptPath returns the resolved path of the executable for a given PID.
// This is used to track scripts that have been killed and detect respawns.
func (r *Responder) getScriptPath(pid uint32) string {
	// Read the symlink /proc/<pid>/exe to get the executable path
	exePath, err := os.Readlink(fmt.Sprintf("/proc/%d/exe", pid))
	if err != nil {
		return ""
	}
	// If it's a symlink to a deleted file (like a script), read cmdline
	if strings.HasSuffix(exePath, " (deleted)") {
		exePath = strings.TrimSuffix(exePath, " (deleted)")
	}
	// Read cmdline to get the actual script path
	cmdline, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
	if err != nil {
		return exePath
	}
	// cmdline is null-separated, first field is the script/binary path
	scriptPath := strings.Split(string(cmdline), "\x00")[0]
	if scriptPath != "" {
		return scriptPath
	}
	return exePath
}

// blockScript renames a suspicious script to prevent re-execution.
// Appends ".ccf-blocked-<timestamp>" to the filename.
func (r *Responder) blockScript(scriptPath string) {
	if scriptPath == "" || r.cfg.DryRun {
		return
	}
	blockedPath := scriptPath + ".ccf-blocked-" + strconv.FormatInt(time.Now().Unix(), 10)
	r.log.Warn("BLOCKING script to prevent respawn",
		zap.String("original", scriptPath),
		zap.String("blocked_to", blockedPath),
	)
	if err := os.Rename(scriptPath, blockedPath); err != nil {
		r.log.Error("failed to block script",
			zap.String("script", scriptPath),
			zap.Error(err),
		)
	}
}

// buildChildMap returns a map of ppid → []pid by reading /proc.
func (r *Responder) buildChildMap() map[uint32][]uint32 {
	children := make(map[uint32][]uint32)
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return children
	}
	for _, e := range entries {
		pid, err := strconv.ParseUint(e.Name(), 10, 32)
		if err != nil {
			continue
		}
		stat, err := os.ReadFile(fmt.Sprintf("/proc/%d/status", pid))
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(stat), "\n") {
			if strings.HasPrefix(line, "PPid:") {
				fields := strings.Fields(line)
				if len(fields) < 2 {
					break
				}
				ppid, err := strconv.ParseUint(fields[1], 10, 32)
				if err != nil {
					break
				}
				children[uint32(ppid)] = append(children[uint32(ppid)], uint32(pid))
				break
			}
		}
	}
	return children
}

// collectDescendants returns pid followed by all its descendants (BFS order).
func collectDescendants(root uint32, children map[uint32][]uint32) []uint32 {
	var result []uint32
	queue := []uint32{root}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		result = append(result, cur)
		queue = append(queue, children[cur]...)
	}
	return result
}

// ---------------------------------------------------------------------------
// Enhancement 3: Network isolation via iptables
// ---------------------------------------------------------------------------

// isolateNetworkForPID reads the UID for the process and installs an iptables
// OUTPUT rule dropping all traffic from that UID. This prevents C2 callbacks
// and data exfiltration while the process is being killed.
func (r *Responder) isolateNetworkForPID(pid uint32) {
	uid := r.uidForPID(pid)
	if uid == 0 {
		// Never block root — too dangerous.
		r.log.Warn("skipping network isolation for root-owned process",
			zap.Uint32("pid", pid),
		)
		return
	}

	r.mu.Lock()
	if r.isolatedUIDs[uid] {
		r.mu.Unlock()
		return // already isolated
	}
	r.isolatedUIDs[uid] = true
	r.mu.Unlock()

	r.log.Warn("isolating network for UID",
		zap.Uint32("uid", uid),
		zap.Uint32("pid", pid),
		zap.Bool("dry_run", r.cfg.DryRun),
	)
	if r.cfg.DryRun {
		return
	}

	// Drop outbound packets from this UID.
	if err := exec.Command("iptables", "-I", "OUTPUT", "1",
		"-m", "owner", "--uid-owner", strconv.FormatUint(uint64(uid), 10),
		"-j", "DROP",
		"-m", "comment", "--comment", fmt.Sprintf("ccf-agent-block-uid-%d", uid),
	).Run(); err != nil {
		r.log.Error("iptables insert failed", zap.Uint32("uid", uid), zap.Error(err))
	}
}

// removeNetworkIsolation removes the iptables rule for a UID.
func (r *Responder) removeNetworkIsolation(uid uint32) {
	if r.cfg.DryRun {
		return
	}
	_ = exec.Command("iptables", "-D", "OUTPUT",
		"-m", "owner", "--uid-owner", strconv.FormatUint(uint64(uid), 10),
		"-j", "DROP",
		"-m", "comment", "--comment", fmt.Sprintf("ccf-agent-block-uid-%d", uid),
	).Run()
}

func (r *Responder) uidForPID(pid uint32) uint32 {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/status", pid))
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "Uid:") {
			fields := strings.Fields(line)
			if len(fields) < 2 {
				return 0
			}
			uid, _ := strconv.ParseUint(fields[1], 10, 32)
			return uint32(uid)
		}
	}
	return 0
}

// ---------------------------------------------------------------------------
// Enhancement 4: Evidence snapshot
// ---------------------------------------------------------------------------

// captureEvidence saves a forensic snapshot of the process to EvidenceDir
// before it is killed. This is the only chance to capture the evidence.
func (r *Responder) captureEvidence(pid uint32, det detector.Detection) {
	if r.cfg.DryRun {
		r.log.Info("dry-run: would capture evidence", zap.Uint32("pid", pid))
		return
	}

	dir := filepath.Join(r.cfg.EvidenceDir,
		fmt.Sprintf("%d_%d", time.Now().Unix(), pid))
	if err := os.MkdirAll(dir, 0700); err != nil {
		r.log.Error("evidence dir create failed", zap.Error(err))
		return
	}

	// Files to snapshot from /proc/<pid>/
	procFiles := []string{"cmdline", "environ", "maps", "status", "net/tcp", "net/udp"}
	for _, f := range procFiles {
		src := fmt.Sprintf("/proc/%d/%s", pid, f)
		data, err := os.ReadFile(src)
		if err != nil {
			continue
		}
		// Replace null bytes in cmdline/environ with spaces for readability.
		if f == "cmdline" || f == "environ" {
			data = bytes.ReplaceAll(data, []byte{0}, []byte{' '})
		}
		dst := filepath.Join(dir, strings.ReplaceAll(f, "/", "_"))
		_ = os.WriteFile(dst, data, 0600)
	}

	// Snapshot open file descriptors.
	fdDir := fmt.Sprintf("/proc/%d/fd", pid)
	if entries, err := os.ReadDir(fdDir); err == nil {
		var fds []string
		for _, e := range entries {
			link, err := os.Readlink(filepath.Join(fdDir, e.Name()))
			if err == nil {
				fds = append(fds, link)
			}
		}
		_ = os.WriteFile(filepath.Join(dir, "fds"), []byte(strings.Join(fds, "\n")), 0600)
	}

	// Write detection metadata.
	meta := map[string]any{
		"at":       det.At.Format(time.RFC3339Nano),
		"severity": det.Severity.String(),
		"score":    det.Score,
		"pid":      pid,
		"cfer":     det.Vector.CFER,
		"turb":     det.Vector.Turbulence,
		"shock":    det.Vector.Shockwave,
		"entropy":  det.Vector.Entropy,
		"nodes":    det.Vector.ActiveNodes,
	}
	if b, err := json.MarshalIndent(meta, "", "  "); err == nil {
		_ = os.WriteFile(filepath.Join(dir, "detection.json"), b, 0600)
	}

	r.log.Info("evidence captured",
		zap.Uint32("pid", pid),
		zap.String("dir", dir),
	)
}

// ---------------------------------------------------------------------------
// Enhancement 5: Webhook / syslog alerting
// ---------------------------------------------------------------------------

// alertPayload is the JSON body sent to the webhook and syslog.
type alertPayload struct {
	At       string  `json:"at"`
	Severity string  `json:"severity"`
	Score    float64 `json:"score"`
	PID      uint32  `json:"pid"`
	Comm     string  `json:"comm,omitempty"`
	CFER     float64 `json:"cfer"`
	Turb     float64 `json:"turbulence"`
	Shock    float64 `json:"shockwave"`
	Entropy  float64 `json:"entropy"`
	Nodes    int     `json:"active_nodes"`
	Reason   string  `json:"reason"`
}

func (r *Responder) sendAlerts(det detector.Detection) {
	pid := det.Vector.OffenderPID
	payload := alertPayload{
		At:       det.At.Format(time.RFC3339Nano),
		Severity: det.Severity.String(),
		Score:    det.Score,
		PID:      pid,
		Comm:     r.commForPID(pid),
		CFER:     det.Vector.CFER,
		Turb:     det.Vector.Turbulence,
		Shock:    det.Vector.Shockwave,
		Entropy:  det.Vector.Entropy,
		Nodes:    det.Vector.ActiveNodes,
		Reason:   det.Reason,
	}

	if r.cfg.WebhookURL != "" {
		r.sendWebhook(payload)
	}
	if r.syslogWriter != nil {
		r.sendSyslog(payload)
	}
}

func (r *Responder) sendWebhook(p alertPayload) {
	body, err := json.Marshal(p)
	if err != nil {
		r.log.Error("webhook marshal failed", zap.Error(err))
		return
	}
	resp, err := r.httpClient.Post(r.cfg.WebhookURL, "application/json", bytes.NewReader(body))
	if err != nil {
		r.log.Error("webhook POST failed", zap.String("url", r.cfg.WebhookURL), zap.Error(err))
		return
	}
	defer resp.Body.Close()
	r.log.Info("webhook sent",
		zap.String("severity", p.Severity),
		zap.Int("http_status", resp.StatusCode),
	)
}

func (r *Responder) sendSyslog(p alertPayload) {
	msg := fmt.Sprintf("ccf-agent %s score=%.3f pid=%d comm=%s cfer=%.3f turb=%.3f shock=%.3f entropy=%.3f",
		p.Severity, p.Score, p.PID, p.Comm, p.CFER, p.Turb, p.Shock, p.Entropy)
	if err := r.syslogWriter.Crit(msg); err != nil {
		r.log.Error("syslog write failed", zap.Error(err))
	}
}

// ---------------------------------------------------------------------------
// Signal helpers (unchanged from basic version)
// ---------------------------------------------------------------------------

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
	if err := proc.Signal(syscallSIGSTOP()); err != nil {
		r.log.Error("SIGSTOP failed", zap.Uint32("pid", pid), zap.Error(err))
		return
	}
	r.mu.Lock()
	r.paused[pid] = time.Now()
	r.mu.Unlock()
}

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

// ---------------------------------------------------------------------------
// Quarantine (unchanged from basic version)
// ---------------------------------------------------------------------------

func (r *Responder) quarantineRecentFiles(det detector.Detection) {
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
		if !cryptoExts[strings.ToLower(filepath.Ext(f))] {
			continue
		}
		dst := filepath.Join(r.cfg.QuarantineDir, filepath.Base(f))
		r.log.Warn("quarantining file",
			zap.String("src", f), zap.String("dst", dst),
			zap.Bool("dry_run", r.cfg.DryRun),
		)
		if r.cfg.DryRun {
			continue
		}
		if err := os.Rename(f, dst); err != nil {
			r.log.Error("quarantine rename failed", zap.String("file", f), zap.Error(err))
			_ = exec.Command("chattr", "+i", f).Run()
		}
	}
}

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

// ---------------------------------------------------------------------------
// Auto-resume and shutdown
// ---------------------------------------------------------------------------

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
					_ = proc.Signal(syscallSIGCONT())
				}
			}
			delete(r.paused, pid)
		}
	}
}

// shutdown resumes all paused processes and cleans up iptables rules.
func (r *Responder) shutdown() {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Resume any SIGSTOP'd processes.
	for pid := range r.paused {
		if proc, err := os.FindProcess(int(pid)); err == nil {
			_ = proc.Signal(syscallSIGCONT())
		}
	}

	// Remove iptables rules we installed.
	for uid := range r.isolatedUIDs {
		r.removeNetworkIsolation(uid)
	}

	// Close syslog.
	if r.syslogWriter != nil {
		_ = r.syslogWriter.Close()
	}
}

// PausedPIDs returns a snapshot of currently SIGSTOP'd PIDs.
func (r *Responder) PausedPIDs() []uint32 {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]uint32, 0, len(r.paused))
	for pid := range r.paused {
		out = append(out, pid)
	}
	return out
}
