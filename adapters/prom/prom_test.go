package prom_test

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"

	"github.com/jninng/observ"
	"github.com/jninng/observ/adapters/prom"
	"github.com/jninng/observ/contract"
)

// 读回包装：为产物附加 Value/Count/Sum/Bounds，启用契约深度档。
type wrapMeter struct{ inner observ.Meter }

func (w wrapMeter) NewCounter(name, help string) observ.Counter {
	return wrapCounter{w.inner.NewCounter(name, help)}
}
func (w wrapMeter) NewGauge(name, help string) observ.Gauge {
	return wrapGauge{w.inner.NewGauge(name, help)}
}
func (w wrapMeter) NewHistogram(name, help string, b []float64) observ.Histogram {
	return wrapHist{w.inner.NewHistogram(name, help, b)}
}

func dtoOf(m prometheus.Metric) *dto.Metric {
	d := &dto.Metric{}
	if err := m.Write(d); err != nil {
		panic(err)
	}
	return d
}

type wrapCounter struct{ c observ.Counter }

func (w wrapCounter) Inc()          { w.c.Inc() }
func (w wrapCounter) Add(v float64) { w.c.Add(v) }
func (w wrapCounter) Value() float64 {
	return dtoOf(w.c.(prometheus.Counter)).GetCounter().GetValue()
}

type wrapGauge struct{ g observ.Gauge }

func (w wrapGauge) Set(v float64) { w.g.Set(v) }
func (w wrapGauge) Add(v float64) { w.g.Add(v) }
func (w wrapGauge) Value() float64 {
	return dtoOf(w.g.(prometheus.Gauge)).GetGauge().GetValue()
}

type wrapHist struct{ h observ.Histogram }

func (w wrapHist) Observe(v float64) { w.h.Observe(v) }
func (w wrapHist) Count() uint64 {
	return dtoOf(w.h.(prometheus.Histogram)).GetHistogram().GetSampleCount()
}
func (w wrapHist) Sum() float64 {
	return dtoOf(w.h.(prometheus.Histogram)).GetHistogram().GetSampleSum()
}
func (w wrapHist) Bounds() []float64 {
	var b []float64
	for _, bk := range dtoOf(w.h.(prometheus.Histogram)).GetHistogram().Bucket {
		b = append(b, bk.GetUpperBound())
	}
	return b
}

func newRegistryMeter() observ.Meter {
	return prom.New(prometheus.NewRegistry())
}

func TestPromMeterContract(t *testing.T) {
	contract.RunMeterContract(t, func() observ.Meter {
		return wrapMeter{newRegistryMeter()}
	})
}

func TestPromRegistrySemantics(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := prom.New(reg)
	c := m.NewCounter("runs_completed_total", "completed runs")
	g := m.NewGauge("queue_depth", "queue depth")
	h := m.NewHistogram("execute_duration_seconds", "exec", []float64{0.1, 1})

	c.Inc()
	g.Set(42)
	h.Observe(0.5)

	mfs, err := reg.Gather()
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"runs_completed_total":     "counter:completed runs",
		"queue_depth":              "gauge:queue depth",
		"execute_duration_seconds": "histogram:exec",
	}
	got := map[string]string{}
	for _, mf := range mfs {
		var typ string
		switch mf.GetType() {
		case dto.MetricType_COUNTER:
			typ = "counter"
		case dto.MetricType_GAUGE:
			typ = "gauge"
		case dto.MetricType_HISTOGRAM:
			typ = "histogram"
		}
		got[mf.GetName()] = typ + ":" + mf.GetHelp()
	}
	for name, w := range want {
		if got[name] != w {
			t.Fatalf("metric %q = %q, want %q (got all: %v)", name, got[name], w, got)
		}
	}
}

func TestPromDuplicateNewPanics(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := prom.New(reg)
	m.NewCounter("dup_total", "h")
	panicked := func() (p bool) {
		defer func() {
			if recover() != nil {
				p = true
			}
		}()
		m.NewCounter("dup_total", "h")
		return
	}()
	if !panicked {
		t.Skip("实现选择容忍重复注册（两结局均合法）")
	}
	// panic 后既有产物仍可用。
	m2c := m.NewCounter("other_total", "h")
	m2c.Inc()
}
