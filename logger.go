package observ

import (
	"context"
	"log/slog"
	"sync/atomic"
)

// Logger 是日志侧统一落点：任意日志库实现两个方法即可接入。
// 级别直接复用 stdlib slog.Level，不做自定义 Level 类型。
type Logger interface {
	Enabled(level slog.Level) bool
	Log(level slog.Level, msg string, attrs ...slog.Attr)
}

type noopLogger struct{}

func (noopLogger) Enabled(level slog.Level) bool {
	return false
}
func (noopLogger) Log(level slog.Level, msg string, attrs ...slog.Attr) {}

// NoopLogger 的 Enabled 恒 false，Log 为空操作。
var NoopLogger Logger = noopLogger{}

// NewSlogLogger 返回包装 *slog.Logger 的 Logger（slog 属 stdlib，零依赖不变）。
func NewSlogLogger(l *slog.Logger) Logger { return slogLogger{l} }

type slogLogger struct{ l *slog.Logger }

func (s slogLogger) Enabled(level slog.Level) bool {
	return s.l.Enabled(context.Background(), level)
}

func (s slogLogger) Log(level slog.Level, msg string, attrs ...slog.Attr) {
	if !s.l.Enabled(context.Background(), level) {
		return
	}
	s.l.LogAttrs(context.Background(), level, msg, attrs...)
}

// defaultLogger 经 atomic.Pointer 读写，永不返回 nil，初始为 NoopLogger。
var defaultLogger atomic.Pointer[Logger]

func init() {
	nl := Logger(NoopLogger)
	defaultLogger.Store(&nl)
}

// DefaultLogger 返回包级默认 Logger（构造期快照语义：业务库在构造函数
// 中读取一次并固定）。未设置任何东西的用户保持零输出、零开销。
func DefaultLogger() Logger {
	return *defaultLogger.Load()
}

// SetDefaultLogger 原子替换默认 Logger 并返回旧值（供测试恢复）。
// 传 nil 等价重置为 NoopLogger。
func SetDefaultLogger(l Logger) (old Logger) {
	if l == nil {
		l = NoopLogger
	}
	p := defaultLogger.Swap(&l)
	return *p
}
