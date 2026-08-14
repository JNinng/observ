package observ

// Meter 是指标侧统一小接口，签名刻意与 prometheus/otel 对齐。
// 根模块不提供任何聚合/导出实现。

type Counter interface {
	Inc()
	Add(v float64)
}

type Gauge interface {
	Set(v float64)
	Add(v float64) // 可为负
}

type Histogram interface {
	Observe(v float64)
}

type Meter interface {
	NewCounter(name, help string) Counter
	NewGauge(name, help string) Gauge
	NewHistogram(name, help string, buckets []float64) Histogram
}

// noopMeter 是可比较的零大小值类型；NoopMeter 供业务库以
// meter == observ.NoopMeter 整体跳过埋点代码块（best-effort 门控）。
type noopMeter struct{}

func (noopMeter) NewCounter(name, help string) Counter { return noopCounter{} }
func (noopMeter) NewGauge(name, help string) Gauge     { return noopGauge{} }
func (noopMeter) NewHistogram(name, help string, b []float64) Histogram {
	return noopHistogram{}
}

type noopCounter struct{}

func (noopCounter) Inc()          {}
func (noopCounter) Add(v float64) {}

type noopGauge struct{}

func (noopGauge) Set(v float64) {}
func (noopGauge) Add(v float64) {}

type noopHistogram struct{}

func (noopHistogram) Observe(v float64) {}

// NoopMeter 恒不 panic；重复 New* 恒正常返回。
var NoopMeter Meter = noopMeter{}
