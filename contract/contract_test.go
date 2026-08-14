package contract_test

import (
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

// fakeMeter 实现读回能力接口，启用深度档断言。
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

// fakeLogger 实现读回能力接口，启用深度档断言。
type fakeLogger struct {
	mu    sync.Mutex
	Recs  []contract.Record
	offAt slog.Level // 该级别及以上 Enabled=false
}

func (l *fakeLogger) Enabled(level slog.Level) bool { return level < l.offAt }
func (l *fakeLogger) Log(level slog.Level, msg string, attrs ...slog.Attr) {
	if !l.Enabled(level) {
		return
	}
	l.mu.Lock()
	l.Recs = append(l.Recs, contract.Record{Level: level, Msg: msg, Attrs: attrs})
	l.mu.Unlock()
}
func (l *fakeLogger) Records() []contract.Record {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]contract.Record(nil), l.Recs...)
}

func TestFakeLoggerContract(t *testing.T) {
	contract.RunLoggerContract(t, func() observ.Logger { return &fakeLogger{offAt: slog.LevelError + 4} })
}
