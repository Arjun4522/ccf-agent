// Package api provides the HTTP REST and WebSocket API for the CCF Agent dashboard.
//
// Endpoints:
//
//	GET  /api/status       — system health snapshot
//	GET  /api/detections   — recent detection ring buffer (?limit=N)
//	GET  /api/quarantine   — quarantine file list
//	POST /api/action       — send an action (resume/kill/quarantine/restore/delete/clear_alerts)
//	GET  /api/config       — read current detector+responder config
//	POST /api/config       — apply hot config update
//	GET  /api/field        — current field snapshot (nodes + edges)
//	GET  /ws/detections    — WebSocket: real-time stream of detection/status/field events
//	GET  /metrics          — Prometheus text metrics
package api

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ccf-agent/internal/detector"
	"github.com/ccf-agent/internal/field"
	"github.com/ccf-agent/internal/responder"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

// ---------------------------------------------------------------------------
// Ring buffer for recent detections
// ---------------------------------------------------------------------------

const ringSize = 1000

type ringBuffer struct {
	mu   sync.RWMutex
	buf  [ringSize]detector.Detection
	head int // next write position
	n    int // how many entries are filled
}

func (r *ringBuffer) push(d detector.Detection) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.buf[r.head] = d
	r.head = (r.head + 1) % ringSize
	if r.n < ringSize {
		r.n++
	}
}

// latest returns up to `limit` most-recent entries, newest first.
func (r *ringBuffer) latest(limit int) []detector.Detection {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if limit <= 0 || limit > r.n {
		limit = r.n
	}
	out := make([]detector.Detection, limit)
	// head-1 is the last written index.
	for i := 0; i < limit; i++ {
		idx := (r.head - 1 - i + ringSize) % ringSize
		out[i] = r.buf[idx]
	}
	return out
}

// ---------------------------------------------------------------------------
// WS broadcaster
// ---------------------------------------------------------------------------

type wsClient struct {
	send chan []byte
}

type broadcaster struct {
	mu      sync.RWMutex
	clients map[*wsClient]struct{}
}

func newBroadcaster() *broadcaster {
	return &broadcaster{clients: make(map[*wsClient]struct{})}
}

func (b *broadcaster) register(c *wsClient) {
	b.mu.Lock()
	b.clients[c] = struct{}{}
	b.mu.Unlock()
}

func (b *broadcaster) deregister(c *wsClient) {
	b.mu.Lock()
	delete(b.clients, c)
	b.mu.Unlock()
}

func (b *broadcaster) broadcast(msg []byte) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for c := range b.clients {
		select {
		case c.send <- msg:
		default:
			// slow client: drop
		}
	}
}

// ---------------------------------------------------------------------------
// Prometheus-style counter store
// ---------------------------------------------------------------------------

type counters struct {
	mu            sync.Mutex
	totalWarnings int64
	totalAlerts   int64
	eventsTotal   int64
	startTime     time.Time

	// Rate tracking: events counted in the last completed 1-second bucket.
	rateEvents   int64
	rateBucketAt time.Time
	eventsPerSec float64
}

func (c *counters) incWarning() { c.mu.Lock(); c.totalWarnings++; c.mu.Unlock() }
func (c *counters) incAlert()   { c.mu.Lock(); c.totalAlerts++; c.mu.Unlock() }
func (c *counters) incEvent() {
	c.mu.Lock()
	c.eventsTotal++
	c.rateEvents++
	now := time.Now()
	if now.Sub(c.rateBucketAt) >= time.Second {
		elapsed := now.Sub(c.rateBucketAt).Seconds()
		if elapsed > 0 {
			c.eventsPerSec = float64(c.rateEvents) / elapsed
		}
		c.rateEvents = 0
		c.rateBucketAt = now
	}
	c.mu.Unlock()
}

func (c *counters) snapshot() (warnings, alerts, events int64, uptime float64, eventsPerSec float64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.totalWarnings, c.totalAlerts, c.eventsTotal, time.Since(c.startTime).Seconds(), c.eventsPerSec
}

// ---------------------------------------------------------------------------
// Server
// ---------------------------------------------------------------------------

var upgrader = websocket.Upgrader{
	CheckOrigin:     func(_ *http.Request) bool { return true },
	ReadBufferSize:  1024,
	WriteBufferSize: 4096,
}

// Server is the API HTTP server. Wire it into main after creating it.
type Server struct {
	log      *zap.Logger
	ring     *ringBuffer
	bc       *broadcaster
	counters *counters
	det      *detector.Detector
	resp     *responder.Responder
	te       *field.TemporalEngine

	// Hot-config guarded by cfgMu
	cfgMu   sync.RWMutex
	detCfg  detector.Config
	respCfg responder.Config

	mux *http.ServeMux
}

// New creates a Server. Call Ingest() from a goroutine to push detections in.
func New(
	log *zap.Logger,
	det *detector.Detector,
	resp *responder.Responder,
	te *field.TemporalEngine,
	detCfg detector.Config,
	respCfg responder.Config,
) *Server {
	s := &Server{
		log:      log,
		ring:     &ringBuffer{},
		bc:       newBroadcaster(),
		counters: &counters{startTime: time.Now(), rateBucketAt: time.Now()},
		det:      det,
		resp:     resp,
		te:       te,
		detCfg:   detCfg,
		respCfg:  respCfg,
		mux:      http.NewServeMux(),
	}
	s.routes()
	return s
}

func (s *Server) routes() {
	s.mux.HandleFunc("/api/status", s.handleStatus)
	s.mux.HandleFunc("/api/detections", s.handleDetections)
	s.mux.HandleFunc("/api/quarantine", s.handleQuarantine)
	s.mux.HandleFunc("/api/action", s.handleAction)
	s.mux.HandleFunc("/api/config", s.handleConfig)
	s.mux.HandleFunc("/api/field", s.handleField)
	s.mux.HandleFunc("/ws/detections", s.handleWS)
	s.mux.HandleFunc("/metrics", s.handleMetrics)
}

// ServeHTTP satisfies http.Handler so Server can be used directly.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

// ListenAndServe starts the HTTP server on addr and blocks until ctx is done.
func (s *Server) ListenAndServe(ctx context.Context, addr string) error {
	srv := &http.Server{Addr: addr, Handler: s}
	go func() {
		<-ctx.Done()
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutCtx)
	}()
	s.log.Info("api server listening", zap.String("addr", addr))
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

// Ingest receives a detection from the main pipeline, stores it in the ring
// buffer, increments counters, and broadcasts it to all WS clients.
func (s *Server) Ingest(d detector.Detection) {
	// Resolve the process name while the process is still alive.
	// commForPID reads /proc/<pid>/comm; if the responder kills the process
	// after this point the name is already captured in the Detection struct.
	if d.ProcessName == "" {
		d.ProcessName = commForPID(d.Vector.OffenderPID)
	}
	s.ring.push(d)
	switch d.Severity {
	case detector.SeverityWarning:
		s.counters.incWarning()
	case detector.SeverityAlert:
		s.counters.incAlert()
	}
	s.counters.incEvent()

	// Broadcast detection event to WebSocket clients.
	if msg, err := s.encodeWS("detection", s.detectionToAPI(d)); err == nil {
		s.bc.broadcast(msg)
	}

	// Also broadcast updated status after each detection.
	if status, err := s.buildStatus(); err == nil {
		if msg, err2 := s.encodeWS("status", status); err2 == nil {
			s.bc.broadcast(msg)
		}
	}
}

// BroadcastField encodes the current field snapshot and sends it to all WS clients.
// Call this periodically from main (e.g. every 2s).
func (s *Server) BroadcastField() {
	snap, ok := s.te.LatestSnapshot()
	if !ok {
		return
	}
	fs := snapshotToAPI(snap)
	if msg, err := s.encodeWS("field", fs); err == nil {
		s.bc.broadcast(msg)
	}
}

// ---------------------------------------------------------------------------
// JSON wire types (match ui/src/types/index.ts exactly)
// ---------------------------------------------------------------------------

type apiFeatureVector struct {
	CFER        float64 `json:"cfer"`
	Turbulence  float64 `json:"turbulence"`
	Shockwave   float64 `json:"shockwave"`
	Entropy     float64 `json:"entropy"`
	ActiveNodes int     `json:"activeNodes"`
	OffenderPID uint32  `json:"offenderPID"`
	ParentPID   uint32  `json:"parentPID"`
}

type apiDetection struct {
	ID          string           `json:"id"`
	Timestamp   string           `json:"timestamp"`
	Severity    string           `json:"severity"`
	Score       float64          `json:"score"`
	Vector      apiFeatureVector `json:"vector"`
	Action      string           `json:"action"`
	Reason      string           `json:"reason"`
	ProcessName string           `json:"processName,omitempty"`
}

type apiStatus struct {
	Status             string  `json:"status"`
	Uptime             float64 `json:"uptime"`
	TotalWarnings      int64   `json:"totalWarnings"`
	TotalAlerts        int64   `json:"totalAlerts"`
	MonitoredProcesses int     `json:"monitoredProcesses"`
	FieldNodes         int     `json:"fieldNodes"`
	EventsPerSecond    float64 `json:"eventsPerSecond"`
	CPUUsage           float64 `json:"cpuUsage"`
	MemoryMb           float64 `json:"memoryMb"`
	LastUpdated        string  `json:"lastUpdated"`
}

type apiQuarantinedFile struct {
	ID            string `json:"id"`
	Path          string `json:"path"`
	QuarantinedAt string `json:"quarantinedAt"`
	OriginPID     int    `json:"originPID"`
	ProcessName   string `json:"processName"`
	Size          int64  `json:"size"`
	Hash          string `json:"hash"`
	Status        string `json:"status"`
}

type apiConfig struct {
	WarningScore        float64 `json:"warningScore"`
	AlertScore          float64 `json:"alertScore"`
	FastThreshold       float64 `json:"fastThreshold"`
	ConfirmMultiplier   float64 `json:"confirmMultiplier"`
	CferThreshold       float64 `json:"cferThreshold"`
	TurbulenceThreshold float64 `json:"turbulenceThreshold"`
	ShockwaveThreshold  float64 `json:"shockwaveThreshold"`
	EntropyThreshold    float64 `json:"entropyThreshold"`
	EnableSigstop       bool    `json:"enableSigstop"`
	EnableSigkill       bool    `json:"enableSigkill"`
	EnableQuarantine    bool    `json:"enableQuarantine"`
	DecayRate           float64 `json:"decayRate"`
	WindowSize          int     `json:"windowSize"`
	SnapshotIntervalMs  int     `json:"snapshotIntervalMs"`
	JSONLogging         bool    `json:"jsonLogging"`
	DebugMode           bool    `json:"debugMode"`
	DryRun              bool    `json:"dryRun"`
}

type apiFieldNode struct {
	ID        string  `json:"id"`
	Label     string  `json:"label"`
	Intensity float64 `json:"intensity"`
	Type      string  `json:"type"`
}

type apiFieldEdge struct {
	Source string  `json:"source"`
	Target string  `json:"target"`
	Weight float64 `json:"weight"`
}

type apiFieldSnapshot struct {
	At    string         `json:"at"`
	Nodes []apiFieldNode `json:"nodes"`
	Edges []apiFieldEdge `json:"edges"`
	Norm  float64        `json:"norm"`
}

type wsEnvelope struct {
	Type    string      `json:"type"`
	Payload interface{} `json:"payload"`
}

// ---------------------------------------------------------------------------
// Handler: GET /api/status
// ---------------------------------------------------------------------------

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	status, err := s.buildStatus()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, status)
}

func (s *Server) buildStatus() (*apiStatus, error) {
	warnings, alerts, _, uptime, eventsPerSec := s.counters.snapshot()

	snap, hasSnap := s.te.LatestSnapshot()
	fieldNodes := 0
	if hasSnap {
		fieldNodes = len(snap.Intensities)
	}

	// Count unique PIDs in ring buffer as a proxy for monitored processes.
	recent := s.ring.latest(200)
	pidSet := make(map[uint32]struct{}, len(recent))
	for _, d := range recent {
		if d.Vector.OffenderPID != 0 {
			pidSet[d.Vector.OffenderPID] = struct{}{}
		}
	}

	agentStatus := "RUNNING"
	if alerts > 0 && len(recent) > 0 && time.Since(recent[0].At) < 60*time.Second {
		agentStatus = "ALERT"
	}

	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)

	return &apiStatus{
		Status:             agentStatus,
		Uptime:             uptime,
		TotalWarnings:      warnings,
		TotalAlerts:        alerts,
		MonitoredProcesses: len(pidSet),
		FieldNodes:         fieldNodes,
		EventsPerSecond:    eventsPerSec,
		CPUUsage:           0, // no cgo, skip — frontend can show 0
		MemoryMb:           float64(ms.Alloc) / 1_048_576,
		LastUpdated:        time.Now().UTC().Format(time.RFC3339),
	}, nil
}

// ---------------------------------------------------------------------------
// Handler: GET /api/detections
// ---------------------------------------------------------------------------

func (s *Server) handleDetections(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	limit := 200
	if lStr := r.URL.Query().Get("limit"); lStr != "" {
		if n, err := strconv.Atoi(lStr); err == nil && n > 0 {
			limit = n
		}
	}
	raw := s.ring.latest(limit)
	out := make([]apiDetection, len(raw))
	for i, d := range raw {
		out[i] = s.detectionToAPI(d)
	}
	writeJSON(w, out)
}

// ---------------------------------------------------------------------------
// Handler: GET /api/quarantine
// ---------------------------------------------------------------------------

func (s *Server) handleQuarantine(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.cfgMu.RLock()
	dir := s.respCfg.QuarantineDir
	s.cfgMu.RUnlock()

	files, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			writeJSON(w, []apiQuarantinedFile{})
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var out []apiQuarantinedFile
	for _, f := range files {
		if f.IsDir() || strings.HasSuffix(f.Name(), ".ccf-meta.json") {
			continue
		}
		info, err := f.Info()
		if err != nil {
			continue
		}
		fullPath := filepath.Join(dir, f.Name())

		// Read sidecar metadata if present.
		meta := readQuarantineMeta(fullPath)

		hash := hashFile(fullPath)

		qf := apiQuarantinedFile{
			ID:            f.Name(),
			Path:          meta.OriginalPath,
			QuarantinedAt: meta.QuarantinedAt,
			OriginPID:     meta.OriginPID,
			ProcessName:   meta.ProcessName,
			Size:          info.Size(),
			Hash:          hash,
			Status:        "quarantined",
		}
		if qf.Path == "" {
			qf.Path = fullPath
		}
		if qf.QuarantinedAt == "" {
			qf.QuarantinedAt = info.ModTime().UTC().Format(time.RFC3339)
		}
		out = append(out, qf)
	}
	if out == nil {
		out = []apiQuarantinedFile{}
	}
	writeJSON(w, out)
}

// quarantineMeta is the sidecar JSON written alongside each quarantined file.
type quarantineMeta struct {
	OriginalPath  string `json:"originalPath"`
	QuarantinedAt string `json:"quarantinedAt"`
	OriginPID     int    `json:"originPID"`
	ProcessName   string `json:"processName"`
}

func readQuarantineMeta(quarantinedPath string) quarantineMeta {
	data, err := os.ReadFile(quarantinedPath + ".ccf-meta.json")
	if err != nil {
		return quarantineMeta{}
	}
	var m quarantineMeta
	_ = json.Unmarshal(data, &m)
	return m
}

func hashFile(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return fmt.Sprintf("%x", sha256.Sum256(data))
}

// ---------------------------------------------------------------------------
// Handler: POST /api/action
// ---------------------------------------------------------------------------

type actionRequest struct {
	Action string `json:"action"` // resume | kill | quarantine | restore | delete | clear_alerts
	PID    uint32 `json:"pid,omitempty"`
	FileID string `json:"fileId,omitempty"`
}

func (s *Server) handleAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req actionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	switch req.Action {
	case "resume":
		if req.PID == 0 {
			http.Error(w, "pid required for resume", http.StatusBadRequest)
			return
		}
		if proc, err := os.FindProcess(int(req.PID)); err == nil {
			_ = proc.Signal(syscallSIGCONT())
		}

	case "kill":
		if req.PID == 0 {
			http.Error(w, "pid required for kill", http.StatusBadRequest)
			return
		}
		if proc, err := os.FindProcess(int(req.PID)); err == nil {
			_ = proc.Kill()
		}

	case "restore":
		if req.FileID == "" {
			http.Error(w, "fileId required for restore", http.StatusBadRequest)
			return
		}
		s.cfgMu.RLock()
		dir := s.respCfg.QuarantineDir
		s.cfgMu.RUnlock()
		src := filepath.Join(dir, req.FileID)
		meta := readQuarantineMeta(src)
		dst := meta.OriginalPath
		if dst == "" {
			http.Error(w, "no original path metadata — cannot restore", http.StatusUnprocessableEntity)
			return
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if err := os.Rename(src, dst); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		_ = os.Remove(src + ".ccf-meta.json")

	case "delete":
		if req.FileID == "" {
			http.Error(w, "fileId required for delete", http.StatusBadRequest)
			return
		}
		s.cfgMu.RLock()
		dir := s.respCfg.QuarantineDir
		s.cfgMu.RUnlock()
		target := filepath.Join(dir, req.FileID)
		if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		_ = os.Remove(target + ".ccf-meta.json")

	case "clear_alerts":
		// No-op: ring buffer is not cleared but UI can refresh.

	default:
		http.Error(w, "unknown action: "+req.Action, http.StatusBadRequest)
		return
	}

	writeJSON(w, map[string]bool{"ok": true})
}

// ---------------------------------------------------------------------------
// Handler: GET+POST /api/config
// ---------------------------------------------------------------------------

func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.cfgMu.RLock()
		dc := s.detCfg
		rc := s.respCfg
		s.cfgMu.RUnlock()
		writeJSON(w, configToAPI(dc, rc))

	case http.MethodPost:
		var incoming apiConfig
		if err := json.NewDecoder(r.Body).Decode(&incoming); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		s.cfgMu.Lock()
		applyAPIConfig(&s.detCfg, &s.respCfg, incoming)
		dc := s.detCfg
		rc := s.respCfg
		s.cfgMu.Unlock()

		// Apply to live detector and responder.
		s.det.SetConfig(dc)
		s.resp.SetConfig(rc)

		writeJSON(w, configToAPI(dc, rc))

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func configToAPI(dc detector.Config, rc responder.Config) apiConfig {
	return apiConfig{
		WarningScore:        dc.WarningScore,
		AlertScore:          dc.AlertScore,
		FastThreshold:       dc.FastThreshold,
		ConfirmMultiplier:   dc.ConfirmMultiplier,
		CferThreshold:       dc.CFERThreshold,
		TurbulenceThreshold: dc.TurbulenceThreshold,
		ShockwaveThreshold:  dc.ShockwaveThreshold,
		EntropyThreshold:    dc.EntropyThreshold,
		EnableSigstop:       rc.PauseOnWarning,
		EnableSigkill:       rc.KillOnAlert,
		EnableQuarantine:    !rc.DryRun,
		DecayRate:           0, // field config not hot-reloadable yet; return 0 as sentinel
		WindowSize:          dc.SlowWindowSize,
		SnapshotIntervalMs:  500, // default; not hot-reloadable yet
		JSONLogging:         false,
		DebugMode:           false,
		DryRun:              rc.DryRun,
	}
}

func applyAPIConfig(dc *detector.Config, rc *responder.Config, a apiConfig) {
	if a.WarningScore > 0 {
		dc.WarningScore = a.WarningScore
	}
	if a.AlertScore > 0 {
		dc.AlertScore = a.AlertScore
	}
	if a.FastThreshold > 0 {
		dc.FastThreshold = a.FastThreshold
	}
	if a.ConfirmMultiplier > 0 {
		dc.ConfirmMultiplier = a.ConfirmMultiplier
	}
	if a.CferThreshold > 0 {
		dc.CFERThreshold = a.CferThreshold
	}
	if a.TurbulenceThreshold > 0 {
		dc.TurbulenceThreshold = a.TurbulenceThreshold
	}
	if a.ShockwaveThreshold > 0 {
		dc.ShockwaveThreshold = a.ShockwaveThreshold
	}
	if a.EntropyThreshold > 0 {
		dc.EntropyThreshold = a.EntropyThreshold
	}
	rc.PauseOnWarning = a.EnableSigstop
	rc.KillOnAlert = a.EnableSigkill
	rc.DryRun = a.DryRun
}

// ---------------------------------------------------------------------------
// Handler: GET /api/field
// ---------------------------------------------------------------------------

func (s *Server) handleField(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	snap, ok := s.te.LatestSnapshot()
	if !ok {
		writeJSON(w, apiFieldSnapshot{
			At:    time.Now().UTC().Format(time.RFC3339),
			Nodes: []apiFieldNode{},
			Edges: []apiFieldEdge{},
			Norm:  0,
		})
		return
	}
	writeJSON(w, snapshotToAPI(snap))
}

// snapshotToAPI converts a field.Snapshot into the frontend FieldSnapshot shape.
func snapshotToAPI(snap field.Snapshot) apiFieldSnapshot {
	nodes := make([]apiFieldNode, 0, len(snap.Intensities))
	for id, intensity := range snap.Intensities {
		nodes = append(nodes, apiFieldNode{
			ID:        id,
			Label:     labelForNode(id),
			Intensity: intensity,
			Type:      typeForNode(id),
		})
	}

	// Build edges: two nodes are adjacent when one is a direct parent dir of the other,
	// or they share the same parent directory (same as field.isAdjacent logic).
	edges := buildEdges(nodes)

	return apiFieldSnapshot{
		At:    snap.At.UTC().Format(time.RFC3339),
		Nodes: nodes,
		Edges: edges,
		Norm:  snap.Norm,
	}
}

func labelForNode(id string) string {
	if strings.HasPrefix(id, "proc:") {
		return strings.TrimPrefix(id, "proc:")
	}
	if strings.HasPrefix(id, "priv:") {
		return strings.TrimPrefix(id, "priv:")
	}
	// directory path — show last two components
	parts := strings.Split(strings.TrimRight(id, "/"), "/")
	if len(parts) >= 2 {
		return parts[len(parts)-2] + "/" + parts[len(parts)-1]
	}
	return id
}

func typeForNode(id string) string {
	if strings.HasPrefix(id, "proc:") {
		return "process"
	}
	if strings.HasPrefix(id, "priv:") {
		return "process"
	}
	return "directory"
}

func buildEdges(nodes []apiFieldNode) []apiFieldEdge {
	var edges []apiFieldEdge
	for i := 0; i < len(nodes); i++ {
		for j := i + 1; j < len(nodes); j++ {
			a, b := nodes[i].ID, nodes[j].ID
			if fieldAdjacent(a, b) {
				w := (nodes[i].Intensity + nodes[j].Intensity) / 2
				edges = append(edges, apiFieldEdge{Source: a, Target: b, Weight: w})
			}
		}
	}
	if edges == nil {
		edges = []apiFieldEdge{}
	}
	return edges
}

func fieldAdjacent(a, b string) bool {
	if len(a) > len(b) {
		a, b = b, a
	}
	if len(b) > len(a) && strings.HasPrefix(b, a) && b[len(a)] == '/' {
		return true
	}
	return parentDir(a) != "" && parentDir(a) == parentDir(b)
}

func parentDir(p string) string {
	for i := len(p) - 1; i > 0; i-- {
		if p[i] == '/' {
			return p[:i]
		}
	}
	return ""
}

// ---------------------------------------------------------------------------
// Handler: GET /ws/detections
// ---------------------------------------------------------------------------

func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		s.log.Warn("ws upgrade failed", zap.Error(err))
		return
	}

	client := &wsClient{send: make(chan []byte, 256)}
	s.bc.register(client)
	defer func() {
		s.bc.deregister(client)
		conn.Close()
	}()

	// Writer goroutine.
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case msg, ok := <-client.send:
				if !ok {
					return
				}
				conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
				if err := conn.WriteMessage(websocket.TextMessage, msg); err != nil {
					return
				}
			case <-ticker.C:
				conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
				if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
					return
				}
			}
		}
	}()

	// Read loop (drain pong / close frames).
	conn.SetReadLimit(512)
	conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	conn.SetPongHandler(func(_ string) error {
		conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			break
		}
	}
	close(client.send)
}

// ---------------------------------------------------------------------------
// Handler: GET /metrics  (Prometheus text format)
// ---------------------------------------------------------------------------

func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	warnings, alerts, events, uptime, _ := s.counters.snapshot()
	snap, hasSnap := s.te.LatestSnapshot()
	fieldNodes := 0
	if hasSnap {
		fieldNodes = len(snap.Intensities)
	}
	pausedPIDs := len(s.resp.PausedPIDs())

	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	fmt.Fprintf(w, "# HELP ccf_uptime_seconds Seconds since agent start\n")
	fmt.Fprintf(w, "# TYPE ccf_uptime_seconds gauge\n")
	fmt.Fprintf(w, "ccf_uptime_seconds %.2f\n\n", uptime)

	fmt.Fprintf(w, "# HELP ccf_detections_total Total detections by severity\n")
	fmt.Fprintf(w, "# TYPE ccf_detections_total counter\n")
	fmt.Fprintf(w, "ccf_detections_total{severity=\"WARNING\"} %d\n", warnings)
	fmt.Fprintf(w, "ccf_detections_total{severity=\"ALERT\"} %d\n\n", alerts)

	fmt.Fprintf(w, "# HELP ccf_events_total Total feature vectors evaluated\n")
	fmt.Fprintf(w, "# TYPE ccf_events_total counter\n")
	fmt.Fprintf(w, "ccf_events_total %d\n\n", events)

	fmt.Fprintf(w, "# HELP ccf_field_nodes Current number of active field nodes\n")
	fmt.Fprintf(w, "# TYPE ccf_field_nodes gauge\n")
	fmt.Fprintf(w, "ccf_field_nodes %d\n\n", fieldNodes)

	fmt.Fprintf(w, "# HELP ccf_paused_pids Processes currently held by SIGSTOP\n")
	fmt.Fprintf(w, "# TYPE ccf_paused_pids gauge\n")
	fmt.Fprintf(w, "ccf_paused_pids %d\n", pausedPIDs)
}

// ---------------------------------------------------------------------------
// Conversion helpers
// ---------------------------------------------------------------------------

func (s *Server) detectionToAPI(d detector.Detection) apiDetection {
	action := "NONE"
	s.cfgMu.RLock()
	rc := s.respCfg
	s.cfgMu.RUnlock()
	switch d.Severity {
	case detector.SeverityWarning:
		if rc.PauseOnWarning {
			action = "SIGSTOP"
		}
	case detector.SeverityAlert:
		if rc.KillOnAlert {
			action = "SIGKILL"
		} else {
			action = "QUARANTINE"
		}
	}

	// Stable deterministic ID: sha256(timestamp + pid)
	idSeed := fmt.Sprintf("%s:%d", d.At.Format(time.RFC3339Nano), d.Vector.OffenderPID)
	id := fmt.Sprintf("%x", sha256.Sum256([]byte(idSeed)))[:16]

	return apiDetection{
		ID:        id,
		Timestamp: d.At.UTC().Format(time.RFC3339),
		Severity:  d.Severity.String(),
		Score:     d.Score,
		Vector: apiFeatureVector{
			CFER:        d.Vector.CFER,
			Turbulence:  d.Vector.Turbulence,
			Shockwave:   d.Vector.Shockwave,
			Entropy:     d.Vector.Entropy,
			ActiveNodes: d.Vector.ActiveNodes,
			OffenderPID: d.Vector.OffenderPID,
			ParentPID:   d.Vector.ParentPID,
		},
		Action:      action,
		Reason:      d.Reason,
		ProcessName: d.ProcessName,
	}
}

func (s *Server) encodeWS(msgType string, payload interface{}) ([]byte, error) {
	return json.Marshal(wsEnvelope{Type: msgType, Payload: payload})
}

func commForPID(pid uint32) string {
	if pid == 0 {
		return ""
	}
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/comm", pid))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// ---------------------------------------------------------------------------
// JSON helper
// ---------------------------------------------------------------------------

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		// headers already written; log only
		_ = err
	}
}

// syscallSIGCONT returns SIGCONT — declared in a platform file to avoid
// importing syscall here for portability.
