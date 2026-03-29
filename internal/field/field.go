// Package field implements the Cognitive Capability Field (CCF) data structure.
package field

import (
	"context"
	"math"
	"sync"
	"time"

	"github.com/ccf-agent/pkg/event"
	"go.uber.org/zap"
)

// Config holds tunable field parameters.
type Config struct {
	DecayRate         float64
	PropagationFactor float64
	MaxIntensity      float64
	InactiveThreshold float64
	DecayInterval     time.Duration
	WindowSize        int
	SnapshotInterval  time.Duration
}

func DefaultConfig() Config {
	return Config{
		// DecayRate 0.85: fast enough to clear idle background (≈15 ticks / 7.5 s
		// to reach 1% of original), slow enough that the attack burst stays
		// detectable across the full 30-snapshot regression window.
		// 0.80 was too aggressive — the attack signal faded before CFER could
		// build up a positive slope, so regression returned ≈ 0.
		DecayRate: 0.85,

		// PropagationFactor 0.03: low enough to avoid false entropy inflation
		// from single-directory I/O, high enough that ransomware touching many
		// directories propagates meaningfully across the field.
		PropagationFactor: 0.03,

		MaxIntensity:      10.0,
		InactiveThreshold: 0.01,
		DecayInterval:     500 * time.Millisecond,

		// 30 snapshots = 15 s regression window. Enough history for CFER slope
		// to distinguish a sustained attack from a brief burst.
		WindowSize:       30,
		SnapshotInterval: 500 * time.Millisecond,
	}
}

// Snapshot is an immutable copy of the field at a point in time.
type Snapshot struct {
	At          time.Time
	Intensities map[string]float64
	Norm        float64 // ||F|| = sqrt(Σ intensity²)
}

// Field is the live CCF field. All exported methods are goroutine-safe.
type Field struct {
	mu          sync.RWMutex
	cfg         Config
	intensities map[string]float64
	adjacency   map[string][]string
}

func NewField(cfg Config) *Field {
	return &Field{
		cfg:         cfg,
		intensities: make(map[string]float64),
		adjacency:   make(map[string][]string),
	}
}

// Update applies a MappedEvent to the field: increment + propagate.
func (f *Field) Update(ev event.MappedEvent) {
	if ev.NodeID == "" {
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.increment(ev.NodeID, ev.Weight)
	f.propagate(ev.NodeID)
}

// Snapshot returns an immutable copy of the current field.
func (f *Field) Snapshot() Snapshot {
	f.mu.RLock()
	defer f.mu.RUnlock()

	cp := make(map[string]float64, len(f.intensities))
	var norm float64
	for k, v := range f.intensities {
		cp[k] = v
		norm += v * v
	}
	return Snapshot{
		At:          time.Now(),
		Intensities: cp,
		Norm:        math.Sqrt(norm),
	}
}

// Decay reduces all node intensities and prunes inactive nodes.
func (f *Field) Decay() {
	f.mu.Lock()
	defer f.mu.Unlock()

	for node, intensity := range f.intensities {
		next := intensity * f.cfg.DecayRate
		if next < f.cfg.InactiveThreshold {
			delete(f.intensities, node)
			delete(f.adjacency, node)
		} else {
			f.intensities[node] = next
		}
	}
}

// NodeIntensity returns the current intensity of a node (0 if absent).
func (f *Field) NodeIntensity(nodeID string) float64 {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.intensities[nodeID]
}

// ---------------------------------------------------------------------------
// Internal field operations
// ---------------------------------------------------------------------------

func (f *Field) increment(nodeID string, weight float64) {
	f.intensities[nodeID] = math.Min(
		f.intensities[nodeID]+weight,
		f.cfg.MaxIntensity,
	)
}

func (f *Field) propagate(nodeID string) {
	neighbours := f.neighbours(nodeID)
	if len(neighbours) == 0 {
		return
	}
	spread := f.intensities[nodeID] * f.cfg.PropagationFactor
	for _, nb := range neighbours {
		f.intensities[nb] = math.Min(
			f.intensities[nb]+spread,
			f.cfg.MaxIntensity,
		)
	}
}

func (f *Field) neighbours(nodeID string) []string {
	if nb, ok := f.adjacency[nodeID]; ok {
		return nb
	}
	var neighbours []string
	for other := range f.intensities {
		if other == nodeID {
			continue
		}
		if isAdjacent(nodeID, other) {
			neighbours = append(neighbours, other)
		}
	}
	f.adjacency[nodeID] = neighbours
	return neighbours
}

func isAdjacent(a, b string) bool {
	if len(a) > len(b) {
		a, b = b, a
	}
	if len(b) > len(a) && b[:len(a)] == a && b[len(a)] == '/' {
		return true
	}
	return parentPath(a) != "" && parentPath(a) == parentPath(b)
}

func parentPath(p string) string {
	for i := len(p) - 1; i > 0; i-- {
		if p[i] == '/' {
			return p[:i]
		}
	}
	return ""
}

// ---------------------------------------------------------------------------
// TemporalEngine
// ---------------------------------------------------------------------------

// TemporalEngine wraps a Field with a sliding window of Snapshots.
type TemporalEngine struct {
	field  *Field
	cfg    Config
	log    *zap.Logger
	mu     sync.RWMutex
	window []Snapshot
}

func NewTemporalEngine(field *Field, cfg Config, log *zap.Logger) *TemporalEngine {
	return &TemporalEngine{
		field:  field,
		cfg:    cfg,
		log:    log,
		window: make([]Snapshot, 0, cfg.WindowSize),
	}
}

// Run drives decay and snapshot ticks until ctx is cancelled.
func (te *TemporalEngine) Run(ctx context.Context, in <-chan event.MappedEvent) {
	decayTicker    := time.NewTicker(te.cfg.DecayInterval)
	snapshotTicker := time.NewTicker(te.cfg.SnapshotInterval)
	defer decayTicker.Stop()
	defer snapshotTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-in:
			if !ok {
				return
			}
			te.field.Update(ev)
		case <-decayTicker.C:
			te.field.Decay()
		case <-snapshotTicker.C:
			snap := te.field.Snapshot()
			te.appendSnapshot(snap)
			te.log.Debug("snapshot",
				zap.Float64("norm", snap.Norm),
				zap.Int("active_nodes", len(snap.Intensities)),
			)
		}
	}
}

// Window returns a copy of the current sliding window, oldest first.
func (te *TemporalEngine) Window() []Snapshot {
	te.mu.RLock()
	defer te.mu.RUnlock()
	cp := make([]Snapshot, len(te.window))
	copy(cp, te.window)
	return cp
}

// LatestSnapshot returns the most recent snapshot.
func (te *TemporalEngine) LatestSnapshot() (Snapshot, bool) {
	te.mu.RLock()
	defer te.mu.RUnlock()
	if len(te.window) == 0 {
		return Snapshot{}, false
	}
	return te.window[len(te.window)-1], true
}

func (te *TemporalEngine) appendSnapshot(s Snapshot) {
	te.mu.Lock()
	defer te.mu.Unlock()
	if len(te.window) >= te.cfg.WindowSize {
		te.window = append(te.window[1:], s)
	} else {
		te.window = append(te.window, s)
	}
}