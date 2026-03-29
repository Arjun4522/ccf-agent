// cmd/agent — CCF Agent entrypoint.
//
// Wires all pipeline stages together and runs them concurrently:
//
//	Collector → (rawCh) → Mapper → (mappedCh) → TemporalEngine
//	                                                   ↓ (tick)
//	                                            Extractor → (vecCh) → Detector → (detCh) → output
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ccf-agent/internal/collector"
	"github.com/ccf-agent/internal/detector"
	"github.com/ccf-agent/internal/features"
	"github.com/ccf-agent/internal/field"
	"github.com/ccf-agent/internal/mapper"
	"github.com/ccf-agent/internal/responder"
	"github.com/ccf-agent/pkg/event"
	"go.uber.org/zap"
)

// ---------------------------------------------------------------------------
// CLI flags
// ---------------------------------------------------------------------------

var (
	flagDebug            = flag.Bool("debug", false, "enable debug logging")
	flagDecayRate        = flag.Float64("decay-rate", 0.85, "field decay rate per tick (0–1)")
	flagWindowSize       = flag.Int("window-size", 30, "temporal window size (snapshots); 30 × 500 ms = 15 s")
	flagSnapshotInterval = flag.Duration("snapshot-interval", 500*time.Millisecond, "snapshot interval")
	flagAlertScore       = flag.Float64("alert-score", 0.65, "composite score threshold for ALERT")
	flagWarningScore     = flag.Float64("warning-score", 0.40, "composite score threshold for WARNING")
	flagCooldownTicks    = flag.Int("cooldown-ticks", 6, "ticks to hold min WARNING after ALERT fires (hysteresis)")
	flagOutputJSON       = flag.Bool("json", false, "output detections as JSON lines")
	flagDryRun           = flag.Bool("dry-run", false, "log blocking actions without executing them")
	flagKillOnAlert      = flag.Bool("kill-on-alert", true, "SIGKILL offending process on ALERT")
	flagPauseOnWarn      = flag.Bool("pause-on-warning", true, "SIGSTOP offending process on WARNING")
	flagQuarantine       = flag.String("quarantine-dir", "/var/lib/ccf-agent/quarantine", "directory for quarantined files")
)

func main() {
	flag.Parse()

	log := buildLogger(*flagDebug)
	defer log.Sync() //nolint:errcheck

	ctx, cancel := signal.NotifyContext(context.Background(),
		syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if err := run(ctx, log); err != nil {
		log.Fatal("agent exited with error", zap.Error(err))
	}
	log.Info("agent stopped cleanly")
}

// ---------------------------------------------------------------------------
// Pipeline wiring
// ---------------------------------------------------------------------------

func run(ctx context.Context, log *zap.Logger) error {
	// ── configs ──────────────────────────────────────────────────────────────
	collCfg := collector.DefaultConfig()
	mapCfg := mapper.DefaultConfig()

	fieldCfg := field.DefaultConfig()
	fieldCfg.DecayRate = *flagDecayRate
	fieldCfg.WindowSize = *flagWindowSize
	fieldCfg.SnapshotInterval = *flagSnapshotInterval

	detCfg := detector.DefaultConfig()
	detCfg.AlertScore = *flagAlertScore
	detCfg.WarningScore = *flagWarningScore
	detCfg.AlertCooldownTicks = *flagCooldownTicks

	// ── channels ─────────────────────────────────────────────────────────────
	rawCh := make(chan event.RawEvent, 4096)
	mappedCh := make(chan event.MappedEvent, 4096)
	vecCh := make(chan features.Vector, 256)
	detCh := make(chan detector.Detection, 64)
	respCh := make(chan detector.Detection, 64)

	// ── stage construction ───────────────────────────────────────────────────
	coll, err := collector.New(collCfg, log.Named("collector"))
	if err != nil {
		return fmt.Errorf("init collector: %w", err)
	}
	defer coll.Close()

	mpr := mapper.New(mapCfg, log.Named("mapper"))
	f := field.NewField(fieldCfg)
	te := field.NewTemporalEngine(f, fieldCfg, log.Named("temporal"))
	ext := features.New()
	det := detector.New(detCfg, log.Named("detector"),
		detector.NewThresholdClassifier(detCfg))

	respCfg := responder.DefaultConfig()
	respCfg.DryRun = *flagDryRun
	respCfg.KillOnAlert = *flagKillOnAlert
	respCfg.PauseOnWarning = *flagPauseOnWarn
	respCfg.QuarantineDir = *flagQuarantine

	resp := responder.New(respCfg, log.Named("responder"))

	go resp.Run(ctx, respCh)

	// ── feature extraction ticker ─────────────────────────────────────────────
	go func() {
		ticker := time.NewTicker(fieldCfg.SnapshotInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				window := te.Window()
				vec, ok := ext.Compute(window)
				if !ok {
					continue
				}
				select {
				case vecCh <- vec:
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	// ── stage goroutines ─────────────────────────────────────────────────────
	go func() {
		if err := coll.Run(ctx, rawCh); err != nil {
			log.Error("collector error", zap.Error(err))
		}
	}()

	go mpr.Run(ctx, rawCh, mappedCh)
	go te.Run(ctx, mappedCh)
	go det.Run(ctx, vecCh, detCh)

	// ── output loop ───────────────────────────────────────────────────────────
	log.Info("ccf agent running",
		zap.Float64("alert_score", *flagAlertScore),
		zap.Float64("warning_score", *flagWarningScore),
		zap.Float64("decay_rate", *flagDecayRate),
		zap.Int("window_size", *flagWindowSize),
		zap.Int("cooldown_ticks", *flagCooldownTicks),
	)

	for {
		select {
		case <-ctx.Done():
			return nil
		case d := <-detCh:
			printDetection(d, *flagOutputJSON)
			select {
			case respCh <- d:
			default:
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Output
// ---------------------------------------------------------------------------

func printDetection(d detector.Detection, asJSON bool) {
	if asJSON {
		type jsonDet struct {
			At       string  `json:"at"`
			Severity string  `json:"severity"`
			Score    float64 `json:"score"`
			CFER     float64 `json:"cfer"`
			Turb     float64 `json:"turbulence"`
			Shock    float64 `json:"shockwave"`
			Entropy  float64 `json:"entropy"`
			Nodes    int     `json:"active_nodes"`
			Reason   string  `json:"reason"`
		}
		enc := json.NewEncoder(os.Stdout)
		_ = enc.Encode(jsonDet{
			At:       d.At.Format(time.RFC3339Nano),
			Severity: d.Severity.String(),
			Score:    d.Score,
			CFER:     d.Vector.CFER,
			Turb:     d.Vector.Turbulence,
			Shock:    d.Vector.Shockwave,
			Entropy:  d.Vector.Entropy,
			Nodes:    d.Vector.ActiveNodes,
			Reason:   d.Reason,
		})
		return
	}
	fmt.Printf("[%s] %s  score=%.3f  cfer=%.3f  turb=%.3f  shock=%.3f  entropy=%.3f  nodes=%d\n",
		d.At.Format("15:04:05.000"),
		d.Severity,
		d.Score,
		d.Vector.CFER,
		d.Vector.Turbulence,
		d.Vector.Shockwave,
		d.Vector.Entropy,
		d.Vector.ActiveNodes,
	)
}

// ---------------------------------------------------------------------------
// Logger
// ---------------------------------------------------------------------------

func buildLogger(debug bool) *zap.Logger {
	var cfg zap.Config
	if debug {
		cfg = zap.NewDevelopmentConfig()
	} else {
		cfg = zap.NewProductionConfig()
	}
	log, err := cfg.Build()
	if err != nil {
		panic(err)
	}
	return log
}
