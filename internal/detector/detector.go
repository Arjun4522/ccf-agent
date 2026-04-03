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
	CFERThreshold       float64
	TurbulenceThreshold float64
	ShockwaveThreshold  float64
	EntropyThreshold    float64

	WarningScore float64
	AlertScore   float64

	CFERWeight       float64
	TurbulenceWeight float64
	ShockwaveWeight  float64
	EntropyWeight    float64

	AlertCooldownTicks int

	UseMLClassifier bool

	FastWindowSize    int
	SlowWindowSize    int
	MinDataPoints     int
	FastThreshold     float64
	ConfirmMultiplier float64
}

func DefaultConfig() Config {
	return Config{
		CFERThreshold:       0.3,
		TurbulenceThreshold: 8.0,
		ShockwaveThreshold:  2.0,
		EntropyThreshold:    3.0,

		WarningScore: 0.40,
		AlertScore:   0.65,

		CFERWeight:       0.45,
		TurbulenceWeight: 0.15,
		ShockwaveWeight:  0.30,
		EntropyWeight:    0.10,

		AlertCooldownTicks: 60,

		UseMLClassifier: false,

		FastWindowSize:    10,
		SlowWindowSize:    30,
		MinDataPoints:     5,
		FastThreshold:     0.30,
		ConfirmMultiplier: 1.5,
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

	sev := d.severityMultiScale(score)

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
	shockSqrt := math.Sqrt(shock)
	shockThreshSqrt := math.Sqrt(max(d.cfg.ShockwaveThreshold, 1e-9))

	cferN := clamp01(cfer / max(d.cfg.CFERThreshold, 1e-9))
	turbN := clamp01(v.Turbulence / max(d.cfg.TurbulenceThreshold, 1e-9))
	shockN := clamp01(shockSqrt / shockThreshSqrt)
	entrN := clamp01(v.Entropy / max(d.cfg.EntropyThreshold, 1e-9))

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

func (d *Detector) severityMultiScale(score float64) Severity {
	if score >= d.cfg.AlertScore {
		return SeverityAlert
	}
	if score >= d.cfg.FastThreshold {
		confirmThreshold := d.cfg.FastThreshold * d.cfg.ConfirmMultiplier
		if score >= confirmThreshold {
			return SeverityAlert
		}
		return SeverityWarning
	}
	return SeverityNone
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
