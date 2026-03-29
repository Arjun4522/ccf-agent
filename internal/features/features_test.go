package features_test

import (
	"math"
	"testing"
	"time"

	"github.com/ccf-agent/internal/features"
	"github.com/ccf-agent/internal/field"
)

func makeSnap(norm float64, intensities map[string]float64) field.Snapshot {
	return field.Snapshot{
		At:          time.Now(),
		Intensities: intensities,
		Norm:        norm,
	}
}

// ── CFER ─────────────────────────────────────────────────────────────────────

func TestCFER_Growing(t *testing.T) {
	window := []field.Snapshot{
		makeSnap(1.0, map[string]float64{"/home/a": 1.0}),
		makeSnap(2.0, map[string]float64{"/home/a": 2.0}),
		makeSnap(3.0, map[string]float64{"/home/a": 3.0}),
	}
	ext := features.New()
	vec, ok := ext.Compute(window)
	if !ok {
		t.Fatal("Compute returned not-ok for 3-snapshot window")
	}
	if vec.CFER <= 0 {
		t.Errorf("expected positive CFER for growing field, got %.4f", vec.CFER)
	}
}

func TestCFER_Stable(t *testing.T) {
	window := []field.Snapshot{
		makeSnap(2.0, map[string]float64{"/home/a": 2.0}),
		makeSnap(2.0, map[string]float64{"/home/a": 2.0}),
		makeSnap(2.0, map[string]float64{"/home/a": 2.0}),
	}
	ext := features.New()
	vec, ok := ext.Compute(window)
	if !ok {
		t.Fatal("not-ok")
	}
	if math.Abs(vec.CFER) > 0.01 {
		t.Errorf("expected near-zero CFER for stable field, got %.4f", vec.CFER)
	}
}

func TestCFER_Oscillating(t *testing.T) {
	// Symmetric oscillation — regression slope should cancel to near zero.
	// This is the idle desktop pattern that used to cause false positives
	// when CFER was computed as a simple point derivative.
	window := []field.Snapshot{
		makeSnap(1.0, nil),
		makeSnap(3.0, nil),
		makeSnap(1.0, nil),
		makeSnap(3.0, nil),
		makeSnap(1.0, nil),
	}
	ext := features.New()
	vec, ok := ext.Compute(window)
	if !ok {
		t.Fatal("not-ok")
	}
	if math.Abs(vec.CFER) > 0.1 {
		t.Errorf("expected near-zero CFER for symmetric oscillation, got %.4f", vec.CFER)
	}
}

// ── Turbulence ────────────────────────────────────────────────────────────────

func TestTurbulence_Volatile(t *testing.T) {
	window := []field.Snapshot{
		makeSnap(0.1, nil),
		makeSnap(9.9, nil),
		makeSnap(0.1, nil),
		makeSnap(9.9, nil),
		makeSnap(0.1, nil),
	}
	ext := features.New()
	vec, ok := ext.Compute(window)
	if !ok {
		t.Fatal("not-ok")
	}
	if vec.Turbulence < 10 {
		t.Errorf("expected high turbulence, got %.4f", vec.Turbulence)
	}
}

func TestTurbulence_Stable(t *testing.T) {
	window := []field.Snapshot{
		makeSnap(5.0, nil),
		makeSnap(5.0, nil),
		makeSnap(5.0, nil),
	}
	ext := features.New()
	vec, ok := ext.Compute(window)
	if !ok {
		t.Fatal("not-ok")
	}
	if vec.Turbulence > 0.001 {
		t.Errorf("expected near-zero turbulence for constant norms, got %.4f", vec.Turbulence)
	}
}

// ── Shockwave ─────────────────────────────────────────────────────────────────

func TestShockwave_Acceleration(t *testing.T) {
	// Norms: 1, 1, 5 — sudden spike. Second derivative = 5 - 2*1 + 1 = 4.
	window := []field.Snapshot{
		makeSnap(1.0, nil),
		makeSnap(1.0, nil),
		makeSnap(5.0, nil),
	}
	ext := features.New()
	vec, ok := ext.Compute(window)
	if !ok {
		t.Fatal("not-ok")
	}
	if vec.Shockwave < 3.5 {
		t.Errorf("expected high shockwave for spike, got %.4f", vec.Shockwave)
	}
}

func TestShockwave_Deceleration(t *testing.T) {
	// Norms: 5, 5, 1 — burst subsiding. Second derivative = 1 - 10 + 5 = -4.
	window := []field.Snapshot{
		makeSnap(5.0, nil),
		makeSnap(5.0, nil),
		makeSnap(1.0, nil),
	}
	ext := features.New()
	vec, ok := ext.Compute(window)
	if !ok {
		t.Fatal("not-ok")
	}
	if vec.Shockwave >= 0 {
		t.Errorf("expected negative shockwave for deceleration, got %.4f", vec.Shockwave)
	}
}

// ── Entropy ───────────────────────────────────────────────────────────────────

func TestEntropy_Concentrated(t *testing.T) {
	window := []field.Snapshot{
		makeSnap(5.0, nil),
		makeSnap(5.0, nil),
		makeSnap(5.0, map[string]float64{"/home/a": 5.0}),
	}
	ext := features.New()
	vec, ok := ext.Compute(window)
	if !ok {
		t.Fatal("not-ok")
	}
	if vec.Entropy > 0.01 {
		t.Errorf("expected near-zero entropy for single node, got %.4f", vec.Entropy)
	}
}

func TestEntropy_Spread(t *testing.T) {
	// Equal intensity across 8 nodes → max entropy = log2(8) = 3.0.
	nodes := map[string]float64{
		"/home/a": 1, "/home/b": 1, "/home/c": 1, "/home/d": 1,
		"/home/e": 1, "/home/f": 1, "/home/g": 1, "/home/h": 1,
	}
	window := []field.Snapshot{
		makeSnap(2.83, nil),
		makeSnap(2.83, nil),
		makeSnap(2.83, nodes),
	}
	ext := features.New()
	vec, ok := ext.Compute(window)
	if !ok {
		t.Fatal("not-ok")
	}
	expected := math.Log2(8)
	if math.Abs(vec.Entropy-expected) > 0.01 {
		t.Errorf("entropy: got %.4f want %.4f", vec.Entropy, expected)
	}
}

// ── Window size ───────────────────────────────────────────────────────────────

func TestTooShortWindow(t *testing.T) {
	window := []field.Snapshot{
		makeSnap(1.0, nil),
		makeSnap(2.0, nil),
	}
	ext := features.New()
	_, ok := ext.Compute(window)
	if ok {
		t.Error("expected not-ok for 2-snapshot window (min 3 required)")
	}
}

func TestMinimumWindow(t *testing.T) {
	window := []field.Snapshot{
		makeSnap(1.0, map[string]float64{"/tmp/a": 1.0}),
		makeSnap(2.0, map[string]float64{"/tmp/a": 2.0}),
		makeSnap(3.0, map[string]float64{"/tmp/a": 3.0}),
	}
	ext := features.New()
	_, ok := ext.Compute(window)
	if !ok {
		t.Error("expected ok for 3-snapshot window")
	}
}