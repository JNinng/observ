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
// 属性编码：slog.Attr 逐个显式转 zap Field，键名原样透传；
// 除 KindAny 兜底外无反射；组属性以点号前缀展平。
package zaplog

import (
	"log/slog"

	"github.com/jninng/observ"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// New 返回包装 zl 的 observ.Logger。
func New(zl *zap.Logger) observ.Logger { return logger{zl} }

type logger struct{ zl *zap.Logger }

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

func (l logger) Enabled(level slog.Level) bool {
	return l.zl.Core().Enabled(mapLevel(level))
}

func (l logger) Log(level slog.Level, msg string, attrs ...slog.Attr) {
	zl := mapLevel(level)
	if !l.zl.Core().Enabled(zl) {
		return
	}
	fields := make([]zap.Field, 0, len(attrs))
	for _, a := range attrs {
		fields = appendAttr(fields, a, "")
	}
	if ce := l.zl.Check(zl, msg); ce != nil {
		ce.Write(fields...)
	}
}

func appendAttr(fields []zap.Field, a slog.Attr, prefix string) []zap.Field {
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
		for _, ga := range a.Value.Group() {
			fields = appendAttr(fields, ga, key)
		}
		return fields
	default: // 含 KindAny：反射兜底
		return append(fields, zap.Any(key, a.Value.Any()))
	}
}
