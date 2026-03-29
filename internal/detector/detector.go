// Package detector evaluates feature vectors and raises alerts.
//
// Two detection modes (selectable at runtime):
//  1. Threshold — fast, deterministic, no model required.
//  2. ML inference — pluggable Classifier interface.
package detector

import (
	"context"
	"math"
	"time"

	"github.com/ccf-agent/internal/features"
	"go.uber.org/zap"
)

// Severity classifies how confident the detector is that an attack is underway.
type Severity int

const (
	SeverityNone    Severity = iota
	SeverityWarning          // elevated — monitor closely
	SeverityAlert            // high confidence attack pattern
)

func (s Severity) String() string {
	switch s {
	case SeverityWarning:
		return "WARNING"
	case SeverityAlert:
		return "ALERT"
	default:
		return "NONE"
	}
}

// Detection is raised when the detector triggers.
type Detection struct {
	At       time.Time
	Severity Severity
	Score    float64         // composite [0, 1]
	Vector   features.Vector // the feature vector that triggered this
	Reason   string          // human-readable trigger explanation
}

// Classifier is the interface for pluggable ML models.
type Classifier interface {
	Score(v features.Vector) (float64, error)
}

// Config holds detector parameters.
type Config struct {
	// Per-feature normalisation thresholds.
	// The composite score is computed as a weighted sum of
	// clamp01(feature / threshold) for each feature.
	CFERThreshold       float64
	TurbulenceThreshold float64
	ShockwaveThreshold  float64 // applied after sqrt compression
	EntropyThreshold    float64

	// Composite score thresholds.
	WarningScore float64
	AlertScore   float64

	// Feature weights (should sum to 1.0).
	CFERWeight       float64
	TurbulenceWeight float64
	ShockwaveWeight  float64
	EntropyWeight    float64

	// AlertCooldownTicks: after an ALERT fires, hold minimum WARNING severity
	// for this many 500 ms ticks. Prevents negative-shockwave oscillations
	// from silencing the detector mid-attack.
	AlertCooldownTicks int

	UseMLClassifier bool
}

func DefaultConfig() Config {
	return Config{
		// --- Thresholds ---
		// These are tuned so that the attack values you saw (CFER≈0.5,
		// turb≈20, shock≈10, entropy≈3.3) normalise well above 1.0 and
		// clamp to 1.0, while idle values stay well below 1.0.
		//
		// CFER: idle regression slope ≈ 0.05–0.15; attack ≈ 0.3–0.6
		CFERThreshold: 0.3,
		// Turbulence: idle variance ≈ 1–4; attack ≈ 15–20
		TurbulenceThreshold: 8.0,
		// Shockwave threshold is in sqrt-space (raw shock is sqrt'd before
		// dividing). sqrt(10) ≈ 3.16 so a raw spike of 10 maps to ~1.0.
		// sqrt(2.0) ≈ 1.41 is the threshold — meaning raw shock of 2.0 = 1.0 normalised.
		ShockwaveThreshold: 2.0,
		// Entropy: idle ≈ 2.2–2.5 bits; attack ≈ 3.2–3.5 bits
		EntropyThreshold: 3.0,

		// --- Score thresholds ---
		// Idle composite in your logs was ≈ 0.37 (before this fix).
		// With new thresholds, idle CFER≈0 contributes 0, turb small,
		// shock oscillates to 0, entropy ≈ 2.2/3.0 * 0.10 ≈ 0.07.
		// Total idle ≈ 0.10–0.20. Attack should hit 0.7+.
		WarningScore: 0.40,
		AlertScore:   0.65,

		// --- Weights ---
		// CFER is the primary signal — regression slope is very clean.
		// Shockwave is secondary — good at detecting onset.
		// Turbulence and entropy are supporting signals.
		CFERWeight:       0.45,
		TurbulenceWeight: 0.15,
		ShockwaveWeight:  0.30,
		EntropyWeight:    0.10,

		// Hold WARNING for 3 s after ALERT to suppress post-burst oscillation.
		AlertCooldownTicks: 6,

		UseMLClassifier: false,
	}
}

// Detector evaluates feature vectors and emits Detection events.
type Detector struct {
	cfg           Config
	log           *zap.Logger
	classifier    Classifier
	alertCooldown int // ticks remaining at elevated minimum severity
}

func New(cfg Config, log *zap.Logger, classifier Classifier) *Detector {
	return &Detector{cfg: cfg, log: log, classifier: classifier}
}

// Run reads feature vectors from in and sends Detections to out when triggered.
func (d *Detector) Run(ctx context.Context, in <-chan features.Vector, out chan<- Detection) {
	for {
		select {
		case <-ctx.Done():
			return
		case vec, ok := <-in:
			if !ok {
				return
			}
			det, triggered := d.evaluate(vec)
			if triggered {
				d.log.Warn("detection",
					zap.String("severity", det.Severity.String()),
					zap.Float64("score", det.Score),
					zap.String("reason", det.Reason),
					zap.Float64("cfer", vec.CFER),
					zap.Float64("turbulence", vec.Turbulence),
					zap.Float64("shockwave", vec.Shockwave),
					zap.Float64("entropy", vec.Entropy),
				)
				select {
				case out <- det:
				case <-ctx.Done():
					return
				}
			}
		}
	}
}

// SetClassifier replaces the pluggable ML classifier at runtime.
func (d *Detector) SetClassifier(c Classifier) {
	d.classifier = c
}

// ---------------------------------------------------------------------------
// Evaluation logic
// ---------------------------------------------------------------------------

func (d *Detector) evaluate(v features.Vector) (Detection, bool) {
	var score float64
	var err error

	if d.cfg.UseMLClassifier && d.classifier != nil {
		score, err = d.classifier.Score(v)
		if err != nil {
			d.log.Error("classifier error, falling back to threshold", zap.Error(err))
			score = d.thresholdScore(v)
		}
	} else {
		score = d.thresholdScore(v)
	}

	sev := d.severity(score)

	// Hysteresis: once ALERT fires, hold minimum WARNING for cooldown ticks.
	// This prevents negative-shockwave ticks from silencing the detector
	// while the field is still highly elevated post-burst.
	if sev == SeverityAlert {
		d.alertCooldown = d.cfg.AlertCooldownTicks
	} else if d.alertCooldown > 0 {
		d.alertCooldown--
		if sev == SeverityNone {
			sev = SeverityWarning
		}
	}

	if sev == SeverityNone {
		return Detection{}, false
	}

	return Detection{
		At:       v.ComputedAt,
		Severity: sev,
		Score:    score,
		Vector:   v,
		Reason:   d.buildReason(v, score),
	}, true
}

// thresholdScore computes a weighted composite score in [0, 1].
//
// Shockwave is sqrt-compressed before normalisation:
//   - Dampens the extreme spike (raw 10 → sqrt 3.16) so it doesn't
//     dominate the composite and mask the other signals.
//   - Negative shockwave (field decelerating) is clamped to 0 — that's
//     a benign signal (burst subsiding), not a ransomware indicator.
func (d *Detector) thresholdScore(v features.Vector) float64 {
	cfer := v.CFER
	if cfer < 0 {
		cfer = 0
	}

	shock := v.Shockwave
	if shock < 0 {
		shock = 0
	}
	// sqrt-compress shockwave to reduce oscillation sensitivity.
	shockSqrt      := math.Sqrt(shock)
	shockThreshSqrt := math.Sqrt(max(d.cfg.ShockwaveThreshold, 1e-9))

	cferN  := clamp01(cfer / max(d.cfg.CFERThreshold, 1e-9))
	turbN  := clamp01(v.Turbulence / max(d.cfg.TurbulenceThreshold, 1e-9))
	shockN := clamp01(shockSqrt / shockThreshSqrt)
	entrN  := clamp01(v.Entropy / max(d.cfg.EntropyThreshold, 1e-9))

	return d.cfg.CFERWeight*cferN +
		d.cfg.TurbulenceWeight*turbN +
		d.cfg.ShockwaveWeight*shockN +
		d.cfg.EntropyWeight*entrN
}

func (d *Detector) severity(score float64) Severity {
	switch {
	case score >= d.cfg.AlertScore:
		return SeverityAlert
	case score >= d.cfg.WarningScore:
		return SeverityWarning
	default:
		return SeverityNone
	}
}

func (d *Detector) buildReason(v features.Vector, score float64) string {
	return "composite score " + formatScore(score) +
		" | CFER=" + formatScore(v.CFER) +
		" turb=" + formatScore(v.Turbulence) +
		" shock=" + formatScore(v.Shockwave) +
		" entropy=" + formatScore(v.Entropy)
}

// ---------------------------------------------------------------------------
// ThresholdClassifier stub
// ---------------------------------------------------------------------------

type ThresholdClassifier struct {
	cfg Config
}

func NewThresholdClassifier(cfg Config) *ThresholdClassifier {
	return &ThresholdClassifier{cfg: cfg}
}

func (tc *ThresholdClassifier) Score(v features.Vector) (float64, error) {
	d := &Detector{cfg: tc.cfg}
	return d.thresholdScore(v), nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func max(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func formatScore(f float64) string {
	const precision = 1000
	i := int(f*precision + 0.5)
	return itoa(i/precision) + "." + pad3(i%precision)
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	b := [20]byte{}
	pos := len(b)
	for i > 0 {
		pos--
		b[pos] = byte('0' + i%10)
		i /= 10
	}
	return string(b[pos:])
}

func pad3(i int) string {
	s := itoa(i)
	for len(s) < 3 {
		s = "0" + s
	}
	return s
}