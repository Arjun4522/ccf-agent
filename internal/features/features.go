// Package features computes the CCF detection feature vector from the
// temporal window of field snapshots.
//
// Features implemented:
//   - CFER       : Capability Field Expansion Rate — linear regression slope
//     of ||F_t|| over the window. Idle ≈ 0; attack ≫ 0.
//   - Turbulence : variance of ||F_t|| over the window — behavioural instability
//   - Shockwave  : second derivative of ||F_t|| — sudden onset detection
//   - Entropy    : Shannon entropy of node intensity distribution — spread indicator
package features

import (
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ccf-agent/internal/field"
)

// Vector is the feature vector fed into the detector stage.
type Vector struct {
	ComputedAt  time.Time
	CFER        float64 // regression slope of norm over window
	Turbulence  float64 // variance of norm over window
	Shockwave   float64 // second derivative of norm
	Entropy     float64 // Shannon entropy H = -Σ p log₂ p
	ActiveNodes int     // number of nodes with non-zero intensity
	OffenderPID uint32  // PID of the most active node's process, 0 if unknown
	ParentPID   uint32  // Parent PID of the offender (for process tree kills)
}

type RunningCFER struct {
	mu        sync.RWMutex
	n         int
	sumX      float64
	sumY      float64
	sumXY     float64
	sumX2     float64
	points    []float64
	maxPoints int
}

func NewRunningCFER(maxPoints int) *RunningCFER {
	return &RunningCFER{
		maxPoints: maxPoints,
		points:    make([]float64, 0, maxPoints),
	}
}

func (r *RunningCFER) Add(y float64) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.n++
	r.sumY += y
	r.sumX2 += float64(r.n * r.n)
	r.sumX += float64(r.n)
	r.sumXY += float64(r.n) * y

	r.points = append(r.points, y)
	if len(r.points) > r.maxPoints {
		r.points = r.points[1:]
		r.n = len(r.points)
		r.recalc()
	}
}

func (r *RunningCFER) recalc() {
	r.sumX = 0
	r.sumY = 0
	r.sumXY = 0
	r.sumX2 = 0
	for i, v := range r.points {
		x := float64(i + 1)
		r.sumX += x
		r.sumY += v
		r.sumXY += x * v
		r.sumX2 += x * x
	}
}

func (r *RunningCFER) Slope() float64 {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.n < 2 {
		return 0
	}
	denom := float64(r.n)*r.sumX2 - r.sumX*r.sumX
	if denom == 0 {
		return 0
	}
	return (float64(r.n)*r.sumXY - r.sumX*r.sumY) / denom
}

func (r *RunningCFER) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.n
}

// Extractor computes a feature Vector from a snapshot window.
// It is stateless and goroutine-safe.
type Extractor struct {
	runningCFER *RunningCFER
}

func New() *Extractor { return &Extractor{} }

func NewWithStreaming(maxPoints int) *Extractor {
	return &Extractor{
		runningCFER: NewRunningCFER(maxPoints),
	}
}

func (e *Extractor) ComputeWithStreaming(snap field.Snapshot, window []field.Snapshot) (Vector, bool) {
	if e.runningCFER == nil {
		e.runningCFER = NewRunningCFER(150)
	}

	e.runningCFER.Add(snap.Norm)

	norms := extractNorms(window)

	offenderPID := topPID(snap)
	parentPID := parentPIDForPID(offenderPID)

	hasEnoughPoints := e.runningCFER.Count() >= 5
	cfer := e.runningCFER.Slope()

	if len(norms) < 3 {
		return Vector{
			ComputedAt:  time.Now(),
			CFER:        cfer,
			Turbulence:  0,
			Shockwave:   0,
			Entropy:     computeEntropy(snap),
			ActiveNodes: len(snap.Intensities),
			OffenderPID: offenderPID,
			ParentPID:   parentPID,
		}, hasEnoughPoints
	}

	return Vector{
		ComputedAt:  time.Now(),
		CFER:        cfer,
		Turbulence:  computeTurbulence(norms),
		Shockwave:   computeShockwave(norms),
		Entropy:     computeEntropy(snap),
		ActiveNodes: len(snap.Intensities),
		OffenderPID: offenderPID,
		ParentPID:   parentPID,
	}, hasEnoughPoints
}

// Compute derives a feature Vector from the provided snapshot window.
// Returns false if the window is too short for meaningful computation
// (minimum 3 snapshots needed for the second derivative).
func (e *Extractor) Compute(window []field.Snapshot) (Vector, bool) {
	if len(window) < 3 {
		return Vector{}, false
	}

	norms := extractNorms(window)

	offenderPID := topPID(window[len(window)-1])
	parentPID := parentPIDForPID(offenderPID)

	return Vector{
		ComputedAt:  time.Now(),
		CFER:        computeCFER(norms),
		Turbulence:  computeTurbulence(norms),
		Shockwave:   computeShockwave(norms),
		Entropy:     computeEntropy(window[len(window)-1]),
		ActiveNodes: len(window[len(window)-1].Intensities),
		OffenderPID: offenderPID,
		ParentPID:   parentPID,
	}, true
}

// ---------------------------------------------------------------------------
// Individual feature computations
// ---------------------------------------------------------------------------

// computeCFER computes the Capability Field Expansion Rate using linear
// regression slope over the full snapshot window.
//
// Using regression instead of point derivative prevents idle +/- oscillations
// from triggering false positives. Idle slope ≈ 0; sustained attack > 0.
func computeCFER(norms []float64) float64 {
	n := len(norms)
	if n < 3 {
		return 0
	}
	var sumX, sumY, sumXY, sumX2 float64
	fn := float64(n)
	for i, v := range norms {
		x := float64(i)
		sumX += x
		sumY += v
		sumXY += x * v
		sumX2 += x * x
	}
	denom := fn*sumX2 - sumX*sumX
	if denom == 0 {
		return 0
	}
	return (fn*sumXY - sumX*sumY) / denom
}

// computeTurbulence computes the variance of ||F_t|| over the full window.
//
//	Turbulence = Var(||F||) = E[||F||²] - E[||F||]²
func computeTurbulence(norms []float64) float64 {
	if len(norms) == 0 {
		return 0
	}
	var sum, sumSq float64
	for _, v := range norms {
		sum += v
		sumSq += v * v
	}
	n := float64(len(norms))
	mean := sum / n
	return (sumSq / n) - (mean * mean)
}

// computeShockwave computes the second derivative of ||F_t|| at the latest point.
//
//	Shock = ||F_{t}|| - 2·||F_{t-1}|| + ||F_{t-2}||
//
// Positive = sudden acceleration (attack onset). Negative = decelerating
// (burst subsiding) — clamped to 0 in the detector before scoring.
func computeShockwave(norms []float64) float64 {
	n := len(norms)
	if n < 3 {
		return 0
	}
	return norms[n-1] - 2*norms[n-2] + norms[n-3]
}

// computeEntropy computes Shannon entropy of the node intensity distribution
// in the latest snapshot.
//
//	H = -Σ p(x) · log₂(p(x))    where p(x) = intensity(x) / Σ intensities
//
// High entropy = influence spread across many nodes (ransomware pattern).
// Low entropy  = localised activity (benign heavy-I/O process).
func computeEntropy(snap field.Snapshot) float64 {
	if len(snap.Intensities) == 0 || snap.Norm == 0 {
		return 0
	}
	var total float64
	for _, v := range snap.Intensities {
		total += v
	}
	if total == 0 {
		return 0
	}
	var h float64
	for _, v := range snap.Intensities {
		p := v / total
		if p > 0 {
			h -= p * math.Log2(p)
		}
	}
	return h
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func extractNorms(window []field.Snapshot) []float64 {
	norms := make([]float64, len(window))
	for i, s := range window {
		norms[i] = s.Norm
	}
	return norms
}

// topPID finds the PID of the process behind the highest-intensity node.
func topPID(snap field.Snapshot) uint32 {
	var topNode string
	var topVal float64
	for node, v := range snap.Intensities {
		if v > topVal {
			topVal = v
			topNode = node
		}
	}
	if !strings.HasPrefix(topNode, "proc:") {
		return 0
	}
	comm := strings.TrimPrefix(topNode, "proc:")
	return findPIDByComm(comm)
}

// findPIDByComm scans /proc for a process with the given comm name.
func findPIDByComm(comm string) uint32 {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return 0
	}
	for _, e := range entries {
		pid, err := strconv.ParseUint(e.Name(), 10, 32)
		if err != nil {
			continue
		}
		data, err := os.ReadFile(fmt.Sprintf("/proc/%d/comm", pid))
		if err != nil {
			continue
		}
		if strings.TrimSpace(string(data)) == comm {
			return uint32(pid)
		}
	}
	return 0
}

// parentPIDForPID reads the parent PID of the given process from /proc/<pid>/status.
func parentPIDForPID(pid uint32) uint32 {
	if pid == 0 {
		return 0
	}
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/status", pid))
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "PPid:") {
			fields := strings.Fields(line)
			if len(fields) < 2 {
				return 0
			}
			ppid, err := strconv.ParseUint(fields[1], 10, 32)
			if err != nil {
				return 0
			}
			return uint32(ppid)
		}
	}
	return 0
}
