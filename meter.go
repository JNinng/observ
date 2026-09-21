package observ

import "sync/atomic"

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

// defaultMeter 经 atomic.Pointer 读写，永不返回 nil，初始为 NoopMeter。
var defaultMeter atomic.Pointer[Meter]

func init() {
	m := Meter(NoopMeter)
	defaultMeter.Store(&m)
}

// DefaultMeter 返回包级默认 Meter（构造期快照语义：业务库在构造函数
// 中读取一次并固定）。未设置任何东西的用户保持零开销 Noop——返回值
// 即 NoopMeter 值，`m == observ.NoopMeter` 门控照常命中。
// 注意 New* 产物绑定构造时刻的 Meter：替换默认只影响之后构造的组件，
// 已建仪表不迁移（含未设置期构造的 Noop 仪表，设置不追溯）。
func DefaultMeter() Meter {
	return *defaultMeter.Load()
}

// SetDefaultMeter 原子替换默认 Meter 并返回旧值（供测试恢复）。
// 传 nil 等价重置为 NoopMeter。
func SetDefaultMeter(m Meter) (old Meter) {
	if m == nil {
		m = NoopMeter
	}
	p := defaultMeter.Swap(&m)
	return *p
}
