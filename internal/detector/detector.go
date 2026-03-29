// Package detector evaluates feature vectors and raises alerts.
//
// Two detection modes (selectable at runtime):
//  1. Threshold — fast, deterministic, no model required. Good for initial
//     deployment and as a pre-filter before ML inference.
//  2. ML inference — pluggable interface; implement Classifier to attach any
//     model (ONNX, TensorFlow Lite, custom scoring function).
//
// The detector is the last stage of the pipeline before alerting.
package detector

import (
	"context"
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
	At        time.Time
	Severity  Severity
	Score     float64          // composite [0, 1]
	Vector    features.Vector  // the feature vector that triggered this
	Reason    string           // human-readable trigger explanation
}

// Classifier is the interface for pluggable ML models.
// Implement this to attach ONNX Runtime, a random forest, or any scorer.
type Classifier interface {
	// Score returns a ransomware probability in [0, 1].
	Score(v features.Vector) (float64, error)
}

// Config holds detector parameters.
type Config struct {
	// Threshold-mode parameters
	CFERThreshold       float64 // CFER above this triggers WARNING
	TurbulenceThreshold float64
	ShockwaveThreshold  float64
	EntropyThreshold    float64

	// Composite score thresholds
	WarningScore float64 // composite score → WARNING
	AlertScore   float64 // composite score → ALERT

	// Weights for composite threshold score (must sum to 1.0)
	CFERWeight       float64
	TurbulenceWeight float64
	ShockwaveWeight  float64
	EntropyWeight    float64

	// UseMLClassifier: if true and a Classifier is attached, use its score
	// instead of the threshold composite.
	UseMLClassifier bool
}

func DefaultConfig() Config {
	return Config{
		CFERThreshold:       1.5,
		TurbulenceThreshold: 0.5,
		ShockwaveThreshold:  0.8,
		EntropyThreshold:    3.0, // bits

		WarningScore: 0.35,
		AlertScore:   0.65,

		CFERWeight:       0.30,
		TurbulenceWeight: 0.25,
		ShockwaveWeight:  0.30,
		EntropyWeight:    0.15,

		UseMLClassifier: false,
	}
}

// Detector evaluates feature vectors and emits Detection events.
type Detector struct {
	cfg        Config
	log        *zap.Logger
	classifier Classifier // nil if UseMLClassifier is false
}

func New(cfg Config, log *zap.Logger, classifier Classifier) *Detector {
	return &Detector{cfg: cfg, log: log, classifier: classifier}
}

// Run reads feature vectors from in and sends Detections to out when triggered.
// Returns when ctx is cancelled.
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

// evaluate decides whether a feature vector should raise a Detection.
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

// thresholdScore computes a weighted composite score in [0, 1] by normalising
// each feature against its threshold.
func (d *Detector) thresholdScore(v features.Vector) float64 {
	cferN  := clamp01(v.CFER / max(d.cfg.CFERThreshold, 1e-9))
	turbN  := clamp01(v.Turbulence / max(d.cfg.TurbulenceThreshold, 1e-9))
	shockN := clamp01(v.Shockwave / max(d.cfg.ShockwaveThreshold, 1e-9))
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
	// TODO: generate structured reason listing which features exceeded thresholds.
	// For now return a summary string.
	return "composite score " + formatScore(score) +
		" | CFER=" + formatScore(v.CFER) +
		" turb=" + formatScore(v.Turbulence) +
		" shock=" + formatScore(v.Shockwave) +
		" entropy=" + formatScore(v.Entropy)
}

// ---------------------------------------------------------------------------
// Stub ML classifier — replace with real model integration
// ---------------------------------------------------------------------------

// ThresholdClassifier is the default stub that simply wraps the threshold logic.
// Replace with an ONNX or custom model implementation.
type ThresholdClassifier struct {
	cfg Config
}

func NewThresholdClassifier(cfg Config) *ThresholdClassifier {
	return &ThresholdClassifier{cfg: cfg}
}

func (tc *ThresholdClassifier) Score(v features.Vector) (float64, error) {
	// TODO: load and run a real model (e.g. via onnxruntime-go).
	// Placeholder: delegate to threshold composite.
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
	// Simple fixed-point formatting without fmt (avoids import cycle risk).
	// Caller can swap for strconv.FormatFloat if preferred.
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
