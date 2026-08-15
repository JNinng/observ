// zaplog 适配器使用示例：用 zap 构造生产级 Logger，包装为
// observ.Logger 并设为包级默认，业务库经默认快照输出日志。
package main

import (
	"log/slog"

	"github.com/jninng/observ"
	"github.com/jninng/observ/adapters/zaplog"
	"go.uber.org/zap"
)

// service 演示业务库的标准接入形态：构造函数读一次
// DefaultLogger() 并固定（构造期快照语义）。
type service struct {
	logger observ.Logger
}

func newService() *service {
	return &service{logger: observ.DefaultLogger()}
}

func (s *service) run(runID string) {
	// 属性键复用 attr.go 约定的统一命名，保证跨库聚合口径一致。
	if !s.logger.Enabled(slog.LevelInfo) {
		return
	}
	s.logger.Log(slog.LevelInfo, "service run started",
		slog.String(observ.AttrRunID, runID))

	s.logger.Log(slog.LevelWarn, "task retried",
		slog.String(observ.AttrRunID, runID),
		slog.Int("attempt", 2),
		// _seconds 属性统一用 Float64 秒：slog.Duration 的渲染格式随
		// handler 而变（300ms/300000000/0.3），会破坏跨库聚合口径。
		slog.Float64(observ.AttrDurationSeconds, 0.3))

	s.logger.Log(slog.LevelError, "task failed",
		slog.String(observ.AttrRunID, runID),
		slog.String(observ.AttrStatus, "failed"),
		slog.String(observ.AttrErrorType, "timeout"))

	// 组属性会被以点号前缀展平（如 http.method）。
	s.logger.Log(slog.LevelInfo, "request done",
		slog.Group("http",
			slog.String("method", "GET"),
			slog.Int("status_code", 200),
		))
}

func main() {
	// 生产配置示例：JSON 输出、Info 级别。
	zl, err := zap.NewProduction()
	if err != nil {
		panic(err)
	}
	defer zl.Sync()

	// 包装为 observ.Logger 并设为包级默认；返回的旧值可在测试中恢复。
	old := observ.SetDefaultLogger(zaplog.New(zl))
	_ = old

	newService().run("run-42")

	// 未接入任何实现时保持零输出、零开销：
	_ = observ.SetDefaultLogger(nil)
	newService().run("run-43")

	// main 正常返回以执行 defer zl.Sync()（os.Exit 会跳过 defer，
	// 有缓冲 core 时会丢日志）。
}
