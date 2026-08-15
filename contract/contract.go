// Package contract 提供 Meter/Logger 契约测试助手（公开形式，
// 供根模块与跨 module 的适配器复用；import "testing" 为明示取舍，
// testing 属 stdlib，不破坏根模块零第三方依赖）。
//
// 契约分两档：
//   - 基线档（全部实现必测）：并发安全（-race）；同名重复 New* 与
//     负值 Add 的两结局断言（正常返回或 panic，经 recover 验证且
//     后续调用仍可用）；Logger 不 panic、并发安全、Enabled 门控
//     下零输出。
//   - 深度档（可选）：New* 返回值或被测 Logger 实现读回能力接口时
//     自动启用，追加 golden 值、buckets 所有权、级别透传与 attrs
//     内容断言。
package contract

import (
	"log/slog"
	"sync"
	"testing"

	"github.com/jninng/observ"
)

// 深度档读回能力接口（实现附加后自动启用深度断言）。

type CounterValue interface{ Value() float64 }
type GaugeValue interface{ Value() float64 }

type HistogramStats interface {
	Count() uint64
	Sum() float64
	Bounds() []float64
}

type Record struct {
	Level slog.Level
	Msg   string
	Attrs []slog.Attr
}

type LoggerRecords interface{ Records() []Record }

// RunMeterContract 对 new 产出的 Meter 运行契约测试。
func RunMeterContract(t *testing.T, new func() observ.Meter) {
	t.Helper()

	t.Run("Concurrent", func(t *testing.T) {
		m := new()
		c := m.NewCounter("contract_c_total", "h")
		g := m.NewGauge("contract_g", "h")
		h := m.NewHistogram("contract_h_seconds", "h", []float64{0.5, 1})
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
	})

	t.Run("DuplicateNewTwoOutcomes", func(t *testing.T) {
		m := new()
		c1 := m.NewCounter("contract_dup_total", "h")
		outcome := -1 // 0=正常返回, 1=panic
		for i := 0; i < 2; i++ {
			panicked, val := tolerate(func() observ.Counter { return m.NewCounter("contract_dup_total", "h") })
			if panicked {
				if outcome == 0 {
					t.Fatal("重复 New* 结局必须一致（先正常后 panic）")
				}
				outcome = 1
			} else {
				if val == nil {
					t.Fatal("未 panic 时 New* 必须返回非 nil 产物")
				}
				if outcome == 1 {
					t.Fatal("重复 New* 结局必须一致（先 panic 后正常）")
				}
				outcome = 0
			}
		}
		// 后续接口调用必须仍可用。
		c1.Inc()
		c1.Add(1)
	})

	t.Run("NegativeAddTwoOutcomes", func(t *testing.T) {
		m := new()
		c := m.NewCounter("contract_neg_total", "h")
		p2, _ := tolerate(func() bool { c.Add(-1); return true })
		_ = p2 // 正常返回或 panic 皆可；若 panic，recover 后下面继续
		c.Inc()
	})

	t.Run("DeepReadback", func(t *testing.T) {
		m := new()
		c := m.NewCounter("contract_deep_c_total", "h")
		cv, cok := c.(CounterValue)
		g := m.NewGauge("contract_deep_g", "h")
		gv, gok := g.(GaugeValue)
		buckets := []float64{0.5, 1, 2.5}
		h := m.NewHistogram("contract_deep_h_seconds", "h", buckets)
		hs, hok := h.(HistogramStats)
		if !cok && !gok && !hok {
			t.Skip("无读回能力接口，深度档跳过")
		}
		if cok {
			c.Inc()
			c.Add(2.5)
			if got := cv.Value(); got != 3.5 {
				t.Fatalf("counter Value = %v, want 3.5", got)
			}
		}
		if gok {
			g.Set(3)
			if got := gv.Value(); got != 3 {
				t.Fatalf("gauge Value after Set = %v, want 3", got)
			}
			g.Add(-1.5)
			if got := gv.Value(); got != 1.5 {
				t.Fatalf("gauge Value after Add = %v, want 1.5", got)
			}
		}
		if hok {
			// buckets 所有权：传入后修改调用方切片，观察结果与 Bounds 不受影响。
			buckets[0] = 99
			h.Observe(0.5)
			h.Observe(2)
			if got, want := hs.Count(), uint64(2); got != want {
				t.Fatalf("hist Count = %v, want %v", got, want)
			}
			if got, want := hs.Sum(), 2.5; got != want {
				t.Fatalf("hist Sum = %v, want %v", got, want)
			}
			b := hs.Bounds()
			if len(b) != 3 || b[0] != 0.5 || b[2] != 2.5 {
				t.Fatalf("hist Bounds = %v, want [0.5 1 2.5]", b)
			}
		}
	})
}

// RunLoggerContract 对 new 产出的 Logger 运行契约测试。
func RunLoggerContract(t *testing.T, new func() observ.Logger) {
	t.Helper()

	t.Run("NoPanicConcurrent", func(t *testing.T) {
		l := new()
		_ = l.Enabled(slog.LevelInfo)
		var wg sync.WaitGroup
		for i := 0; i < 8; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for j := 0; j < 100; j++ {
					l.Enabled(slog.LevelInfo)
					l.Log(slog.LevelInfo, "m", slog.String("k", "v"))
					l.Log(slog.LevelError, "e")
				}
			}()
		}
		wg.Wait()
	})

	t.Run("DeepRecords", func(t *testing.T) {
		l := new()
		lr, ok := l.(LoggerRecords)
		if !ok {
			t.Skip("无 Records 读回接口，深度档跳过")
		}
		attrs := []slog.Attr{slog.String("run_id", "r1"), slog.Int64("n", 3)}
		l.Log(slog.LevelWarn, "hello", attrs...)
		recs := lr.Records()
		if len(recs) != 1 {
			t.Fatalf("records = %d, want 1", len(recs))
		}
		r := recs[0]
		if r.Level != slog.LevelWarn || r.Msg != "hello" {
			t.Fatalf("record level/msg = %v/%q", r.Level, r.Msg)
		}
		if len(r.Attrs) != 2 || r.Attrs[0].Key != "run_id" || r.Attrs[0].Value.String() != "r1" ||
			r.Attrs[1].Key != "n" || r.Attrs[1].Value.Int64() != 3 {
			t.Fatalf("record attrs = %+v", r.Attrs)
		}
		// Enabled 门控下零输出。
		for lvl := slog.LevelDebug; lvl <= slog.LevelError+8; lvl += 4 {
			if !l.Enabled(lvl) {
				before := len(lr.Records())
				l.Log(lvl, "gated")
				if after := len(lr.Records()); after != before {
					t.Fatalf("Log at disabled level %v produced output", lvl)
				}
			}
		}
	})
}

// tolerate 执行 f：返回是否 panic 及 f 的返回值（panic 时为零值）。
func tolerate[T any](f func() T) (panicked bool, val T) {
	defer func() {
		if r := recover(); r != nil {
			panicked = true
		}
	}()
	val = f()
	return
}
