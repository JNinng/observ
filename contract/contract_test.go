package contract_test

import (
	"context"
	"log/slog"
	"sync"
	"testing"

	"github.com/jninng/observ"
	"github.com/jninng/observ/contract"
)

func TestNoopMeterContract(t *testing.T) {
	contract.RunMeterContract(t, func() observ.Meter { return observ.NoopMeter })
}

func TestNoopLoggerContract(t *testing.T) {
	contract.RunLoggerContract(t, func() observ.Logger { return observ.NoopLogger })
}

// fakeMeter 实现读回能力接口与 WithCtx 能力接口，启用深度档与
// CtxCapability 断言。
type fakeMeter struct{}

func (fakeMeter) NewCounter(name, help string) observ.Counter {
	return &fakeCounter{}
}
func (fakeMeter) NewGauge(name, help string) observ.Gauge { return &fakeGauge{} }
func (fakeMeter) NewHistogram(name, help string, b []float64) observ.Histogram {
	return &fakeHist{bounds: append([]float64(nil), b...)}
}

type fakeCounter struct {
	mu sync.Mutex
	v  float64
}

func (c *fakeCounter) Inc() { c.Add(1) }
func (c *fakeCounter) Add(v float64) {
	c.mu.Lock()
	c.v += v
	c.mu.Unlock()
}

// Ctx 变体走与基础方法相同的累积路径(验证"计入同一产物"契约)。
func (c *fakeCounter) IncCtx(ctx context.Context)            { c.Inc() }
func (c *fakeCounter) AddCtx(ctx context.Context, v float64) { c.Add(v) }
func (c *fakeCounter) Value() float64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.v
}

type fakeGauge struct {
	mu sync.Mutex
	v  float64
}

func (g *fakeGauge) Set(v float64) { g.mu.Lock(); g.v = v; g.mu.Unlock() }
func (g *fakeGauge) Add(v float64) { g.mu.Lock(); g.v += v; g.mu.Unlock() }

func (g *fakeGauge) SetCtx(ctx context.Context, v float64) { g.Set(v) }
func (g *fakeGauge) AddCtx(ctx context.Context, v float64) { g.Add(v) }
func (g *fakeGauge) Value() float64 {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.v
}

type fakeHist struct {
	mu     sync.Mutex
	count  uint64
	sum    float64
	bounds []float64
}

func (h *fakeHist) Observe(v float64) {
	h.mu.Lock()
	h.count++
	h.sum += v
	h.mu.Unlock()
}
func (h *fakeHist) ObserveCtx(ctx context.Context, v float64) { h.Observe(v) }
func (h *fakeHist) Count() uint64 {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.count
}
func (h *fakeHist) Sum() float64 {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.sum
}
func (h *fakeHist) Bounds() []float64 { return h.bounds }

func TestFakeMeterContract(t *testing.T) {
	contract.RunMeterContract(t, func() observ.Meter { return fakeMeter{} })
}

// fakeLogger 实现读回能力接口与 LoggerWithAttrs，启用深度档与
// WithAttrs 断言。
type fakeLogger struct {
	mu    sync.Mutex
	Recs  []contract.Record
	offAt slog.Level  // 该级别及以上 Enabled=false
	base  []slog.Attr // WithAttrs 绑定属性(前缀语义)
}

func (l *fakeLogger) Enabled(_ context.Context, level slog.Level) bool { return level < l.offAt }
func (l *fakeLogger) Log(_ context.Context, level slog.Level, msg string, attrs ...slog.Attr) {
	if !l.Enabled(context.Background(), level) {
		return
	}
	combined := make([]slog.Attr, 0, len(l.base)+len(attrs))
	combined = append(combined, l.base...)
	combined = append(combined, attrs...)
	l.mu.Lock()
	l.Recs = append(l.Recs, contract.Record{Level: level, Msg: msg, Attrs: combined})
	l.mu.Unlock()
}

// WithAttrs 拷贝入参满足所有权契约(传参后调用方切片的修改不影响输出),
// 叠加顺序:先绑定者更靠前。
func (l *fakeLogger) WithAttrs(attrs ...slog.Attr) observ.Logger {
	base := make([]slog.Attr, 0, len(l.base)+len(attrs))
	base = append(base, l.base...)
	base = append(base, attrs...)
	return &fakeLogger{offAt: l.offAt, base: base}
}
func (l *fakeLogger) Records() []contract.Record {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]contract.Record(nil), l.Recs...)
}

func TestFakeLoggerContract(t *testing.T) {
	contract.RunLoggerContract(t, func() observ.Logger { return &fakeLogger{offAt: slog.LevelError + 4} })
}
