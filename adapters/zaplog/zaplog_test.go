package zaplog_test

import (
	"log/slog"
	"math"
	"sync"
	"testing"
	"time"

	"github.com/jninng/observ"
	"github.com/jninng/observ/adapters/zaplog"
	"github.com/jninng/observ/contract"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// recCore 记录型 zapcore.Core，支撑 Records() 深度档。
type recCore struct {
	mu     sync.Mutex
	levels []zapcore.Level
	msgs   []string
	fields [][]zapcore.Field
	min    zapcore.Level
}

func (c *recCore) Enabled(l zapcore.Level) bool             { return l >= c.min }
func (c *recCore) With(fields []zapcore.Field) zapcore.Core { return c }
func (c *recCore) Check(e zapcore.Entry, ce *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	if c.Enabled(e.Level) {
		return ce.AddCore(e, c)
	}
	return ce
}
func (c *recCore) Write(e zapcore.Entry, fields []zapcore.Field) error {
	c.mu.Lock()
	c.levels = append(c.levels, e.Level)
	c.msgs = append(c.msgs, e.Message)
	c.fields = append(c.fields, append([]zapcore.Field(nil), fields...))
	c.mu.Unlock()
	return nil
}
func (c *recCore) Sync() error { return nil }

type recLogger struct {
	observ.Logger
	core *recCore
}

func (l recLogger) Records() []contract.Record {
	l.core.mu.Lock()
	defer l.core.mu.Unlock()
	var out []contract.Record
	for i := range l.core.msgs {
		var attrs []slog.Attr
		for _, f := range l.core.fields[i] {
			switch f.Type {
			case zapcore.StringType:
				attrs = append(attrs, slog.String(f.Key, f.String))
			case zapcore.Int64Type:
				attrs = append(attrs, slog.Int64(f.Key, f.Integer))
			case zapcore.Float64Type:
				attrs = append(attrs, slog.Float64(f.Key, math.Float64frombits(uint64(f.Integer))))
			case zapcore.BoolType:
				attrs = append(attrs, slog.Bool(f.Key, f.Integer == 1))
			case zapcore.DurationType:
				attrs = append(attrs, slog.Int64(f.Key, f.Integer))
			default:
				attrs = append(attrs, slog.Any(f.Key, f.Interface))
			}
		}
		out = append(out, contract.Record{
			Level: unmapLevel(l.core.levels[i]),
			Msg:   l.core.msgs[i],
			Attrs: attrs,
		})
	}
	return out
}

func unmapLevel(z zapcore.Level) slog.Level {
	switch z {
	case zapcore.DebugLevel:
		return slog.LevelDebug
	case zapcore.InfoLevel:
		return slog.LevelInfo
	case zapcore.WarnLevel:
		return slog.LevelWarn
	default:
		return slog.LevelError
	}
}

func newRec() recLogger {
	core := &recCore{min: zapcore.DebugLevel}
	return recLogger{Logger: zaplog.New(zap.New(core)), core: core}
}

func TestZapLoggerContract(t *testing.T) {
	contract.RunLoggerContract(t, func() observ.Logger { return newRec() })
}

func TestZapLevelMapping(t *testing.T) {
	core := &recCore{min: zapcore.DebugLevel}
	l := zaplog.New(zap.New(core))
	cases := []struct {
		in   slog.Level
		want zapcore.Level
	}{
		{slog.LevelDebug, zapcore.DebugLevel},
		{slog.LevelInfo, zapcore.InfoLevel},
		{slog.LevelWarn, zapcore.WarnLevel},
		{slog.LevelError, zapcore.ErrorLevel},
		{slog.LevelWarn - 2, zapcore.WarnLevel}, // 中间级别就近向下（更严重）
		{slog.LevelInfo + 2, zapcore.WarnLevel},
	}
	for _, tc := range cases {
		l.Log(tc.in, "m")
		if got := core.levels[len(core.levels)-1]; got != tc.want {
			t.Fatalf("level %v mapped to %v, want %v", tc.in, got, tc.want)
		}
	}
}

// stringValuer 验证 slog.LogValuer 在编码前被解析（对齐 slog handler 语义）。
type stringValuer struct{}

func (stringValuer) LogValue() slog.Value { return slog.StringValue("resolved") }

func TestZapLogValuerResolved(t *testing.T) {
	core := &recCore{min: zapcore.DebugLevel}
	l := zaplog.New(zap.New(core))
	l.Log(slog.LevelInfo, "m", slog.Any("v", stringValuer{}))
	fs := core.fields[len(core.fields)-1]
	if len(fs) != 1 {
		t.Fatalf("fields = %d, want 1", len(fs))
	}
	if fs[0].Type != zapcore.StringType || fs[0].String != "resolved" {
		t.Fatalf("LogValuer field = %+v, want string \"resolved\"", fs[0])
	}
}

func TestZapGroupFlattening(t *testing.T) {
	core := &recCore{min: zapcore.DebugLevel}
	l := zaplog.New(zap.New(core))
	l.Log(slog.LevelInfo, "m",
		slog.String("top", "1"),
		slog.Group("", slog.String("inlined", "2")),                    // 顶层空名组：内联
		slog.Group("http", slog.Group("", slog.String("nested", "3"))), // 嵌套空名组：沿用父前缀
		slog.Group("rpc", slog.String("method", "call")),               // 常规嵌套组
	)
	fs := core.fields[len(core.fields)-1]
	want := []string{"top", "inlined", "http.nested", "rpc.method"}
	if len(fs) != len(want) {
		t.Fatalf("fields = %d, want %d (%+v)", len(fs), len(want), fs)
	}
	for i, k := range want {
		if fs[i].Key != k {
			t.Fatalf("field[%d].Key = %q, want %q", i, fs[i].Key, k)
		}
	}
}

func TestZapAttrEncoding(t *testing.T) {
	core := &recCore{min: zapcore.DebugLevel}
	l := zaplog.New(zap.New(core))
	l.Log(slog.LevelInfo, "m",
		slog.String("run_id", "r1"),
		slog.Int64("n", 3),
		slog.Uint64("u", 7),
		slog.Float64("f", 1.5),
		slog.Bool("b", true),
		slog.Time("ts", time.Unix(0, 1)),
		slog.Duration("duration_seconds", 1500*time.Millisecond),
	)
	fs := core.fields[len(core.fields)-1]
	if len(fs) != 7 {
		t.Fatalf("fields = %d, want 7", len(fs))
	}
	check := func(i int, key string) {
		t.Helper()
		if fs[i].Key != key {
			t.Fatalf("field[%d].Key = %q, want %q", i, fs[i].Key, key)
		}
	}
	check(0, "run_id")
	if fs[0].String != "r1" {
		t.Fatalf("run_id = %q", fs[0].String)
	}
	check(1, "n")
	if fs[1].Integer != 3 {
		t.Fatalf("n = %d, want 3", fs[1].Integer)
	}
	check(2, "u")
	if fs[2].Integer != 7 {
		t.Fatalf("u = %d, want 7", fs[2].Integer)
	}
	check(3, "f")
	if math.Float64frombits(uint64(fs[3].Integer)) != 1.5 {
		t.Fatalf("f = %v, want 1.5", math.Float64frombits(uint64(fs[3].Integer)))
	}
	check(4, "b")
	if fs[4].Type != zapcore.BoolType {
		t.Fatalf("b type = %v, want %v", fs[4].Type, zapcore.BoolType)
	}
	check(5, "ts")
	if fs[5].Type != zapcore.TimeType {
		t.Fatalf("ts type = %v, want %v", fs[5].Type, zapcore.TimeType)
	}
	check(6, "duration_seconds")
	if fs[6].Type != zapcore.DurationType {
		t.Fatalf("duration type = %v, want %v", fs[6].Type, zapcore.DurationType)
	}
	if fs[6].Integer != int64(1500*time.Millisecond) {
		t.Fatalf("duration = %d, want %d", fs[6].Integer, int64(1500*time.Millisecond))
	}
}
