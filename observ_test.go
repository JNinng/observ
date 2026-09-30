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
	if observ.NoopLogger.Enabled(context.Background(), slog.LevelError) {
		t.Fatal("NoopLogger.Enabled must be false")
	}
	observ.NoopLogger.Log(context.Background(), slog.LevelError, "m", slog.String("k", "v")) // 不得 panic
	n := testing.AllocsPerRun(100, func() {
		observ.NoopLogger.Log(context.Background(), slog.LevelInfo, "m")
	})
	if n != 0 {
		t.Fatalf("NoopLogger.Log no-attr allocs = %v, want 0", n)
	}
}

func TestDefaultMeterSemantics(t *testing.T) {
	if observ.DefaultMeter() != observ.NoopMeter {
		t.Fatal("DefaultMeter must start as NoopMeter")
	}
	m := wrapMeter{observ.NoopMeter}
	old := observ.SetDefaultMeter(m)
	if old != observ.NoopMeter {
		t.Fatalf("SetDefaultMeter old = %v, want NoopMeter", old)
	}
	if observ.DefaultMeter() != m {
		t.Fatal("DefaultMeter did not swap")
	}
	old2 := observ.SetDefaultMeter(nil)
	if old2 != m || observ.DefaultMeter() != observ.NoopMeter {
		t.Fatal("SetDefaultMeter(nil) must reset to NoopMeter")
	}
}

// wrapMeter 以嵌入制造与 NoopMeter 相异的动态类型（方法全部可用），
// 供默认值替换断言区分身份。
type wrapMeter struct{ observ.Meter }

func TestDefaultMeterConcurrentSwap(t *testing.T) {
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				if m := observ.DefaultMeter(); m == nil {
					t.Error("DefaultMeter returned nil")
					return
				}
				observ.SetDefaultMeter(wrapMeter{observ.NoopMeter})
			}
		}()
	}
	wg.Wait()
	// 恢复
	observ.SetDefaultMeter(nil)
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
	h := newRecSlogHandler()
	sl := observ.NewSlogLogger(slog.New(h))
	if !sl.Enabled(context.Background(), slog.LevelError) {
		t.Fatal("slog bridge Enabled(Error) should be true for text-like handler")
	}
	type ctxKey struct{}
	ctx := context.WithValue(context.Background(), ctxKey{}, "v1")
	sl.Log(ctx, slog.LevelWarn, "hello", slog.String("run_id", "r1"))
	if len(h.sink.recs) != 1 || h.sink.recs[0].Level != slog.LevelWarn || h.sink.recs[0].Message != "hello" {
		t.Fatalf("unexpected records: %+v", h.sink.recs)
	}
	if got := h.sink.ctxs[0].Value(ctxKey{}); got != "v1" {
		t.Fatalf("slog bridge dropped caller ctx, got %v", got)
	}
}

func TestNoopWithCtxCapabilities(t *testing.T) {
	m := observ.NoopMeter
	ctx := context.Background()

	cc, ok := m.NewCounter("c_total", "h").(observ.CounterWithCtx)
	if !ok {
		t.Fatal("Noop counter must implement CounterWithCtx")
	}
	cc.IncCtx(ctx)
	cc.AddCtx(ctx, 1)

	gc, ok := m.NewGauge("g", "h").(observ.GaugeWithCtx)
	if !ok {
		t.Fatal("Noop gauge must implement GaugeWithCtx")
	}
	gc.SetCtx(ctx, 1)
	gc.AddCtx(ctx, -1)

	hc, ok := m.NewHistogram("h_seconds", "h", []float64{1}).(observ.HistogramWithCtx)
	if !ok {
		t.Fatal("Noop histogram must implement HistogramWithCtx")
	}
	hc.ObserveCtx(ctx, 0.5)

	n := testing.AllocsPerRun(100, func() { cc.AddCtx(ctx, 1) })
	if n != 0 {
		t.Fatalf("NoopMeter counter AddCtx allocs = %v, want 0", n)
	}
}

func TestNoopLoggerWithAttrs(t *testing.T) {
	la, ok := observ.NoopLogger.(observ.LoggerWithAttrs)
	if !ok {
		t.Fatal("NoopLogger must implement LoggerWithAttrs")
	}
	if got := la.WithAttrs(slog.String("k", "v")); got != observ.NoopLogger {
		t.Fatal("NoopLogger.WithAttrs must return NoopLogger")
	}
	la.WithAttrs().Log(context.Background(), slog.LevelError, "m") // 不得 panic
}

func TestSlogLoggerWithAttrs(t *testing.T) {
	h := newRecSlogHandler()
	sl := observ.NewSlogLogger(slog.New(h))
	la, ok := sl.(observ.LoggerWithAttrs)
	if !ok {
		t.Fatal("slog bridge must implement LoggerWithAttrs")
	}
	bound := la.WithAttrs(slog.String("component", "cache"))
	// 能力保留:派生 Logger 仍实现 LoggerWithAttrs,可叠加(先绑定者靠前)。
	b2, ok := bound.(observ.LoggerWithAttrs)
	if !ok {
		t.Fatal("slog bridge WithAttrs must preserve capability")
	}
	base := []slog.Attr{slog.String("run_id", "r1")}
	bound = b2.WithAttrs(base...)
	// 所有权:传参后修改调用方切片,输出不受影响。
	base[0] = slog.String("run_id", "mutated")

	type ctxKey struct{}
	ctx := context.WithValue(context.Background(), ctxKey{}, "v1")
	bound.Log(ctx, slog.LevelWarn, "hello", slog.String("status", "ok"))

	if len(h.sink.recs) != 1 {
		t.Fatalf("records = %d, want 1", len(h.sink.recs))
	}
	var keys, vals []string
	h.sink.recs[0].Attrs(func(a slog.Attr) bool {
		keys = append(keys, a.Key)
		vals = append(vals, a.Value.String())
		return true
	})
	if len(keys) != 3 || keys[0] != "component" || keys[1] != "run_id" || keys[2] != "status" ||
		vals[0] != "cache" || vals[1] != "r1" {
		t.Fatalf("attr order = %v/%v, want component=cache, run_id=r1, status", keys, vals)
	}
	if got := h.sink.ctxs[0].Value(ctxKey{}); got != "v1" {
		t.Fatalf("WithAttrs-derived logger dropped caller ctx, got %v", got)
	}
}

// recSlogHandler 记录型 handler,模拟真实 handler 的 WithAttrs 前缀语义:
// 绑定属性在记录属性之前;派生实例经 sink 共享记录。
type recSlogHandler struct {
	sink  *recSink
	attrs []slog.Attr
}

type recSink struct {
	recs []slog.Record
	ctxs []context.Context
}

func newRecSlogHandler() *recSlogHandler { return &recSlogHandler{sink: &recSink{}} }

func (h *recSlogHandler) Enabled(_ context.Context, _ slog.Level) bool { return true }

func (h *recSlogHandler) Handle(ctx context.Context, r slog.Record) error {
	nr := slog.NewRecord(r.Time, r.Level, r.Message, r.PC)
	nr.AddAttrs(h.attrs...)
	r.Attrs(func(a slog.Attr) bool { nr.AddAttrs(a); return true })
	h.sink.recs = append(h.sink.recs, nr)
	h.sink.ctxs = append(h.sink.ctxs, ctx)
	return nil
}

// WithAttrs 拷贝入参(所有权:调用方传参后的修改不影响输出)。
func (h *recSlogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	next := append([]slog.Attr(nil), h.attrs...)
	next = append(next, attrs...)
	return &recSlogHandler{sink: h.sink, attrs: next}
}

func (h *recSlogHandler) WithGroup(string) slog.Handler { return h }
