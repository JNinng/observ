// Package zaplog 用 zap 直接实现 observ.Logger。
//
// 级别映射：
//
//	slog.LevelDebug ↔ zapcore.DebugLevel
//	slog.LevelInfo  ↔ zapcore.InfoLevel
//	slog.LevelWarn  ↔ zapcore.WarnLevel
//	slog.LevelError ↔ zapcore.ErrorLevel
//	中间自定义级别就近向下（更严重）取整。
//
// ctx 形参参与编码：WithCtxAttrs 注入提取器时，每次 Log 前从 ctx 提取
// 属性（链路注入 trace_id/span_id/request_id 等）追加在调用方属性之后。
// 注入做在适配层内部——调用面无需装饰层，zap caller 定位不受封装层数
// 影响；提取器须并发安全且快速返回。
//
// 动态实例：NewDynamic 包装"取当前实例的函数"而非固定实例——热更重建
// 换新后桥自动跟随，桥身份恒定、可安全长持（observ 默认日志器只装一次）。
// caller skip 由实例侧烘焙（zap.AddCallerSkip），本适配层自身恒为一帧。
//
// 属性编码：slog.Attr 逐个显式转 zap Field，键名原样透传；
// LogValuer 在编码前解析（对齐 slog handler 语义）；除 KindAny
// 兜底外无反射；组属性以点号前缀展平，空名组内联。
package zaplog

import (
	"context"
	"log/slog"

	"github.com/jninng/observ"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Option 构造选项。
type Option func(*config)

type config struct {
	ctxAttrs func(context.Context) []slog.Attr
}

// WithCtxAttrs 注入 ctx 属性提取器：每次 Log 前调用，返回的属性追加在
// 调用方属性之后（zap 对同名键双写不覆盖——调用方显式传过的键应避免在
// 提取器中重复产出）。nil 返回值合法（零属性差异）。
func WithCtxAttrs(fn func(context.Context) []slog.Attr) Option {
	return func(c *config) { c.ctxAttrs = fn }
}

// New 返回包装 zl 的 observ.Logger。
func New(zl *zap.Logger, opts ...Option) observ.Logger {
	return NewDynamic(func() *zap.Logger { return zl }, opts...)
}

// NewDynamic 返回包装动态实例的 observ.Logger：current 每次调用取当前
// 生效实例（须并发安全；热更重建换新后自动跟随）。桥身份恒定——
// 经 observ.SetDefaultLogger 安装后无需随实例重建重装。
func NewDynamic(current func() *zap.Logger, opts ...Option) observ.Logger {
	var c config
	for _, o := range opts {
		o(&c)
	}
	return logger{current: current, ctxAttrs: c.ctxAttrs}
}

type logger struct {
	current  func() *zap.Logger
	ctxAttrs func(context.Context) []slog.Attr
}

func mapLevel(l slog.Level) zapcore.Level {
	switch {
	case l <= slog.LevelDebug:
		return zapcore.DebugLevel
	case l <= slog.LevelInfo:
		return zapcore.InfoLevel
	case l <= slog.LevelWarn:
		return zapcore.WarnLevel
	default:
		return zapcore.ErrorLevel
	}
}

func (l logger) Enabled(ctx context.Context, level slog.Level) bool {
	return l.current().Core().Enabled(mapLevel(level))
}

func (l logger) Log(ctx context.Context, level slog.Level, msg string, attrs ...slog.Attr) {
	zl := mapLevel(level)
	inst := l.current()
	if !inst.Core().Enabled(zl) {
		return
	}
	fields := make([]zap.Field, 0, len(attrs))
	for _, a := range attrs {
		fields = appendAttr(fields, a, "")
	}
	if l.ctxAttrs != nil {
		for _, a := range l.ctxAttrs(ctx) {
			fields = appendAttr(fields, a, "")
		}
	}
	if ce := inst.Check(zl, msg); ce != nil {
		ce.Write(fields...)
	}
}

func appendAttr(fields []zap.Field, a slog.Attr, prefix string) []zap.Field {
	// 对齐 slog handler 语义：格式化前解析 LogValuer（解析可能改变
	// Kind，包括解析为组值）。
	a.Value = a.Value.Resolve()
	key := a.Key
	if prefix != "" {
		key = prefix + "." + key
	}
	switch a.Value.Kind() {
	case slog.KindString:
		return append(fields, zap.String(key, a.Value.String()))
	case slog.KindInt64:
		return append(fields, zap.Int64(key, a.Value.Int64()))
	case slog.KindUint64:
		return append(fields, zap.Uint64(key, a.Value.Uint64()))
	case slog.KindFloat64:
		return append(fields, zap.Float64(key, a.Value.Float64()))
	case slog.KindBool:
		return append(fields, zap.Bool(key, a.Value.Bool()))
	case slog.KindDuration:
		return append(fields, zap.Duration(key, a.Value.Duration()))
	case slog.KindTime:
		return append(fields, zap.Time(key, a.Value.Time()))
	case slog.KindGroup:
		// 空名组内联（对齐 slog）：子属性沿用父前缀，避免产生
		// "http..key" 双点号键。
		child := key
		if a.Key == "" {
			child = prefix
		}
		for _, ga := range a.Value.Group() {
			fields = appendAttr(fields, ga, child)
		}
		return fields
	default: // 含 KindAny：反射兜底
		return append(fields, zap.Any(key, a.Value.Any()))
	}
}
