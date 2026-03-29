// Package field implements the Cognitive Capability Field (CCF) data structure.
//
// The field F is a sparse map from NodeID → float64 intensity.
// Each incoming MappedEvent:
//  1. Increments the target node's intensity by the event weight.
//  2. Propagates a fraction of that intensity to neighbouring nodes.
//  3. Decays all node intensities on each tick.
//
// The TemporalEngine wraps the Field in a sliding window of snapshots,
// enabling derivative-based metrics (CFER, shockwave) in the feature layer.
package field

import (
	"context"
	"math"
	"sync"
	"time"

	"github.com/ccf-agent/pkg/event"
	"go.uber.org/zap"
)

// ---------------------------------------------------------------------------
// Field
// ---------------------------------------------------------------------------

// Config holds tunable field parameters.
type Config struct {
	// DecayRate is multiplied against every node intensity on each decay tick.
	// 0.9 means 10% loss per tick. Range (0, 1).
	DecayRate float64

	// PropagationFactor is the fraction of a node's intensity that spreads to
	// each direct neighbour on update. Keep low (0.05–0.15) to avoid saturation.
	PropagationFactor float64

	// MaxIntensity clamps any single node to prevent runaway accumulation.
	MaxIntensity float64

	// InactiveThreshold: nodes below this value are pruned from the sparse map
	// during decay to keep memory bounded.
	InactiveThreshold float64

	// DecayInterval controls how often the decay pass runs.
	DecayInterval time.Duration

	// WindowSize is the number of field snapshots the TemporalEngine retains.
	WindowSize int

	// SnapshotInterval controls how often a snapshot is appended to the window.
	SnapshotInterval time.Duration
}

func DefaultConfig() Config {
	return Config{
		// DecayRate lowered from 0.92 → 0.80 so old activity fades faster.
		// At 0.92 background writes accumulate and keep norm/entropy elevated
		// for many seconds, making the field indistinguishable from an attack.
		// At 0.80 a burst of writes decays to <1% in ~20 ticks (~10 s).
		DecayRate: 0.80,

		// PropagationFactor lowered from 0.08 → 0.03 to stop normal writes
		// in one directory from artificially inflating neighbour nodes and
		// driving entropy up on idle workloads.
		PropagationFactor: 0.03,

		MaxIntensity:      10.0,
		InactiveThreshold: 0.01, // prune sooner to keep node count honest
		DecayInterval:     500 * time.Millisecond,
		WindowSize:        30, // 30 × 500 ms = 15-second window (more regression data)
		SnapshotInterval:  500 * time.Millisecond,
	}
}

// Snapshot is an immutable copy of the field at a point in time.
type Snapshot struct {
	At         time.Time
	Intensities map[string]float64
	Norm        float64 // ||F|| = sqrt(Σ intensity²)
}

// Field is the live CCF field. All exported methods are goroutine-safe.
type Field struct {
	mu          sync.RWMutex
	cfg         Config
	intensities map[string]float64
	// adjacency holds the neighbourhood graph used for propagation.
	// Built lazily: two nodes are adjacent if they share a path prefix.
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
// Called periodically by the TemporalEngine tick.
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

// propagate spreads a fraction of nodeID's current intensity to neighbours.
// Neighbour discovery uses path-prefix adjacency (lazy, cached).
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

// neighbours returns the cached adjacency list for nodeID.
// Two nodes are adjacent if one is a path-prefix of the other (depth ±1).
//
// TODO: For non-file nodes (proc:*, priv:*) define explicit adjacency rules,
// e.g. proc:malware.exe → all file nodes it has touched in the last window.
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

// isAdjacent returns true if a and b share a path prefix at depth ±1.
// e.g. "/home/user/docs" and "/home/user/images" are adjacent (same parent).
func isAdjacent(a, b string) bool {
	// Simple prefix check: one is a prefix of the other or they share a parent.
	// TODO: Replace with a proper trie for O(1) lookup at scale.
	if len(a) > len(b) {
		a, b = b, a
	}
	// b is the longer one. Adjacent if b starts with a+"/" (parent-child)
	// or they share the same directory prefix.
	if len(b) > len(a) && b[:len(a)] == a && b[len(a)] == '/' {
		return true
	}
	// Same-depth siblings: share parent path.
	parentA := parentPath(a)
	parentB := parentPath(b)
	return parentA != "" && parentA == parentB
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
// It drives the decay/snapshot ticks and exposes the window to the feature layer.
type TemporalEngine struct {
	field   *Field
	cfg     Config
	log     *zap.Logger
	mu      sync.RWMutex
	window  []Snapshot // circular, len ≤ cfg.WindowSize
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
// Also reads MappedEvents from in and applies them to the field.
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

// Window returns a copy of the current sliding window of snapshots,
// oldest first. The caller must not modify the returned slice.
func (te *TemporalEngine) Window() []Snapshot {
	te.mu.RLock()
	defer te.mu.RUnlock()
	cp := make([]Snapshot, len(te.window))
	copy(cp, te.window)
	return cp
}

// LatestSnapshot returns the most recent snapshot, or zero value if the
// window is empty.
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
		// Slide: drop oldest.
		te.window = append(te.window[1:], s)
	} else {
		te.window = append(te.window, s)
	}
}