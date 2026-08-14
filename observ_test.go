package observ_test

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"testing"

	"github.com/jninng/observ"
)

func TestNoopMeterZeroAlloc(t *testing.T) {
	c := observ.NoopMeter.NewCounter("x_total", "h")
	n := testing.AllocsPerRun(100, func() { c.Inc() })
	if n != 0 {
		t.Fatalf("NoopMeter counter Inc allocs = %v, want 0", n)
	}
}

func TestNoopMeterEqualityGate(t *testing.T) {
	if observ.NoopMeter != observ.Meter(noopMeterPub()) {
		t.Fatal("NoopMeter must stay comparable/equal as exported value")
	}
	m := observ.Meter(observ.NoopMeter)
	if m != observ.NoopMeter {
		t.Fatal("meter == observ.NoopMeter gating broken")
	}
}

func noopMeterPub() observ.Meter { return observ.NoopMeter }

func TestNoopMeterRepeatedNew(t *testing.T) {
	m := observ.NoopMeter
	for i := 0; i < 3; i++ {
		m.NewCounter("a_total", "h").Add(1)
		m.NewGauge("g", "h").Set(-1)
		m.NewHistogram("h_seconds", "h", []float64{1, 2}).Observe(0.5)
	}
}

func TestNoopMeterConcurrent(t *testing.T) {
	m := observ.NoopMeter
	c := m.NewCounter("c_total", "h")
	g := m.NewGauge("g", "h")
	h := m.NewHistogram("h_seconds", "h", []float64{1})
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				c.Inc()
				c.Add(1)
				g.Set(1)
				g.Add(-1)
				h.Observe(0.1)
			}
		}()
	}
	wg.Wait()
}

func TestNoopLogger(t *testing.T) {
	if observ.NoopLogger.Enabled(slog.LevelError) {
		t.Fatal("NoopLogger.Enabled must be false")
	}
	observ.NoopLogger.Log(slog.LevelError, "m", slog.String("k", "v")) // 不得 panic
	n := testing.AllocsPerRun(100, func() { observ.NoopLogger.Log(slog.LevelInfo, "m") })
	if n != 0 {
		t.Fatalf("NoopLogger.Log no-attr allocs = %v, want 0", n)
	}
}

func TestDefaultLoggerSemantics(t *testing.T) {
	if observ.DefaultLogger() != observ.NoopLogger {
		t.Fatal("DefaultLogger must start as NoopLogger")
	}
	sl := observ.NewSlogLogger(slog.New(slog.NewTextHandler(io.Discard, nil)))
	old := observ.SetDefaultLogger(sl)
	if old != observ.NoopLogger {
		t.Fatalf("SetDefaultLogger old = %v, want NoopLogger", old)
	}
	if observ.DefaultLogger() != sl {
		t.Fatal("DefaultLogger did not swap")
	}
	old2 := observ.SetDefaultLogger(nil)
	if old2 != sl || observ.DefaultLogger() != observ.NoopLogger {
		t.Fatal("SetDefaultLogger(nil) must reset to NoopLogger")
	}
}

func TestDefaultLoggerConcurrentSwap(t *testing.T) {
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				if l := observ.DefaultLogger(); l == nil {
					t.Error("DefaultLogger returned nil")
					return
				}
				observ.SetDefaultLogger(nil)
			}
		}()
	}
	wg.Wait()
	// 恢复
	observ.SetDefaultLogger(nil)
}

func TestSlogLoggerBridge(t *testing.T) {
	h := &recSlogHandler{}
	sl := observ.NewSlogLogger(slog.New(h))
	if !sl.Enabled(slog.LevelError) {
		t.Fatal("slog bridge Enabled(Error) should be true for text-like handler")
	}
	sl.Log(slog.LevelWarn, "hello", slog.String("run_id", "r1"))
	if len(h.recs) != 1 || h.recs[0].Level != slog.LevelWarn || h.recs[0].Message != "hello" {
		t.Fatalf("unexpected records: %+v", h.recs)
	}
}

type recSlogHandler struct{ recs []slog.Record }

func (h *recSlogHandler) Enabled(ctx context.Context, l slog.Level) bool { return true }
func (h *recSlogHandler) Handle(ctx context.Context, r slog.Record) error {
	h.recs = append(h.recs, r)
	return nil
}
func (h *recSlogHandler) WithAttrs(attrs []slog.Attr) slog.Handler { return h }
func (h *recSlogHandler) WithGroup(name string) slog.Handler       { return h }
