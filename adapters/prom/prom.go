// Package prom 用 client_golang 实现 observ.Meter。
//
// 语义：
//   - New* 即向传入的 Registerer 注册（构造期调用，禁止热路径）。
//   - 非法指标名：随 client_golang 在注册期 panic（对齐"报错而非
//     静默改名"，不透传 prom 的静默清洗）。
//   - 同名重复 New*：MustRegister panic（契约两结局之一）。
//   - NewHistogram 传入的 buckets 会被拷贝，返回后归实现所有。
package prom

import (
	"github.com/prometheus/client_golang/prometheus"

	"github.com/jninng/observ"
)

// New 返回把指标注册到 reg 的 observ.Meter。reg 为 nil 时使用
// prometheus.DefaultRegisterer。
func New(reg prometheus.Registerer) observ.Meter {
	if reg == nil {
		reg = prometheus.DefaultRegisterer
	}
	return meter{reg}
}

type meter struct{ reg prometheus.Registerer }

func (m meter) NewCounter(name, help string) observ.Counter {
	c := prometheus.NewCounter(prometheus.CounterOpts{Name: name, Help: help})
	m.reg.MustRegister(c)
	return counter{c}
}

func (m meter) NewGauge(name, help string) observ.Gauge {
	g := prometheus.NewGauge(prometheus.GaugeOpts{Name: name, Help: help})
	m.reg.MustRegister(g)
	return gauge{g}
}

func (m meter) NewHistogram(name, help string, buckets []float64) observ.Histogram {
	// 拷贝 buckets：调用方传入后不得再影响实现（契约 §4.2 所有权）。
	bs := append([]float64(nil), buckets...)
	h := prometheus.NewHistogram(prometheus.HistogramOpts{Name: name, Help: help, Buckets: bs})
	m.reg.MustRegister(h)
	return histogram{h}
}

// counter 内嵌 prometheus.Counter（含 Describe/Collect），使包装类型
// 仍可被断言回 prometheus 指标做读回。
type counter struct{ prometheus.Counter }

type gauge struct{ prometheus.Gauge }

type histogram struct{ prometheus.Histogram }
