// Package features computes the CCF detection feature vector from the
// temporal window of field snapshots.
//
// Features implemented (vorticity excluded per design decision):
//   - CFER  : Capability Field Expansion Rate — how fast total influence grows
//   - Turbulence : variance of ||F_t|| over the window — behavioural instability
//   - Shockwave  : second derivative of ||F_t|| — sudden onset detection
//   - Entropy    : Shannon entropy of node intensity distribution — spread indicator
package features

import (
	"math"
	"time"

	"github.com/ccf-agent/internal/field"
)

// Vector is the feature vector fed into the ML inference stage.
type Vector struct {
	ComputedAt  time.Time
	CFER        float64 // expansion rate, units: intensity/second
	Turbulence  float64 // variance of norm over window
	Shockwave   float64 // second derivative of norm (Δ²||F||/Δt²)
	Entropy     float64 // Shannon entropy H = -Σ p log p
	ActiveNodes int     // number of nodes with non-zero intensity
}

// Extractor computes a feature Vector from a snapshot window.
// It is stateless and goroutine-safe.
type Extractor struct{}

func New() *Extractor { return &Extractor{} }

// Compute derives a feature Vector from the provided snapshot window.
// Returns false if the window is too short for meaningful computation
// (minimum 3 snapshots needed for the second derivative).
func (e *Extractor) Compute(window []field.Snapshot) (Vector, bool) {
	if len(window) < 3 {
		return Vector{}, false
	}

	norms := extractNorms(window)

	return Vector{
		ComputedAt:  time.Now(),
		CFER:        computeCFER(window, norms),
		Turbulence:  computeTurbulence(norms),
		Shockwave:   computeShockwave(norms),
		Entropy:     computeEntropy(window[len(window)-1]),
		ActiveNodes: len(window[len(window)-1].Intensities),
	}, true
}

// ---------------------------------------------------------------------------
// Individual feature computations
// ---------------------------------------------------------------------------

// computeCFER computes the Capability Field Expansion Rate:
//
//	CFER = (||F_t|| - ||F_{t-1}||) / dt
//
// Uses the last two snapshots for the first derivative.
// Units: intensity per second.
func computeCFER(window []field.Snapshot, norms []float64) float64 {
	n := len(norms)
	if n < 2 {
		return 0
	}
	dt := window[n-1].At.Sub(window[n-2].At).Seconds()
	if dt <= 0 {
		return 0
	}
	return (norms[n-1] - norms[n-2]) / dt
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
//	Shock = (||F_{t}|| - 2·||F_{t-1}|| + ||F_{t-2}||) / dt²
//
// A large positive value indicates sudden field acceleration — attack onset.
func computeShockwave(norms []float64) float64 {
	n := len(norms)
	if n < 3 {
		return 0
	}
	// Use uniform time spacing assumption (dt = 1 normalised unit) for simplicity.
	// TODO: use actual snapshot timestamps for variable-rate accuracy.
	d2 := norms[n-1] - 2*norms[n-2] + norms[n-3]
	return d2 // positive = accelerating expansion (shockwave)
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

	// Sum of intensities (L1 norm for probability normalisation).
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

// extractNorms pulls the pre-computed ||F|| value from each snapshot.
func extractNorms(window []field.Snapshot) []float64 {
	norms := make([]float64, len(window))
	for i, s := range window {
		norms[i] = s.Norm
	}
	return norms
}
