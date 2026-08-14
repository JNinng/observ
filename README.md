# observ

基础库共享可观测规范包。为 Go 基础库定义统一的 **Meter / Logger** 小接口与接入规范：日志库不可知、指标出口不可知、性能敏感场景下可安全埋点。

## 特性

- **零第三方依赖**：根模块只用 stdlib，`go.mod` 的 require 块为空。
- **日志桥接**：`Logger` 接口仅 2 个方法，任意日志库直接实现即接入；slog 经根模块自带桥，zap 经 `adapters/zaplog`。
- **指标小接口**：`Meter` 签名对齐 prometheus/otel 子集，根模块不提供聚合/导出实现。
- **契约测试**：`contract` 包提供 `RunMeterContract` / `RunLoggerContract`，适配器实现一套测试即可验证并发安全与语义。
- **Noop 默认回落**：未注入选项时零输出、零开销。

## 安装

```bash
go get github.com/jninng/observ
```

适配器（独立 module，独立版本）：

```bash
go get github.com/jninng/observ/adapters/prom
go get github.com/jninng/observ/adapters/zaplog
```

要求 Go 1.21+。

## 快速上手

### 日志

```go
logger := observ.NewSlogLogger(slog.Default())
logger.Log(slog.LevelInfo, "service started", slog.String("component", "cache"))

// 或设置包级默认（原子替换，业务库构造期读取一次并固定）
old := observ.SetDefaultLogger(logger)
defer observ.SetDefaultLogger(old)
```

### 指标

```go
var meter observ.Meter = observ.NoopMeter

counter := meter.NewCounter("requests_total", "Total requests processed")
counter.Inc()
counter.Add(2.5)

histogram := meter.NewHistogram("latency_seconds", "Request latency", []float64{0.5, 1, 2.5})
histogram.Observe(0.42)
```

### 接入 Prometheus / zap

```go
import (
    "github.com/jninng/observ/adapters/prom"
    "github.com/jninng/observ/adapters/zaplog"
)

meter := prom.New(registry)             // 实现 observ.Meter
logger := zaplog.New(zapLogger)         // 实现 observ.Logger
```

### 契约测试（适配器作者）

```go
func TestMeterContract(t *testing.T) {
    contract.RunMeterContract(t, func() observ.Meter {
        return prom.New(prometheus.NewRegistry())
    })
}

func TestLoggerContract(t *testing.T) {
    contract.RunLoggerContract(t, func() observ.Logger {
        return zaplog.New(zap.NewNop())
    })
}
```

## 业务库接入规范（摘要）

1. **类型化 Observer**：每类事件一个固定结构体，方法 `OnXxx(XxxEvent)` 按值传递；无反射、无装箱、无变参切片。
2. **option 模式注入**：`WithObserver(obs)`/`WithMeter(m)` 默认 Noop；`WithLogger(l)` 未注入时构造期取 `observ.DefaultLogger()` 并固定（快照语义）。
3. **回调纪律**：回调在调用方 goroutine 同步执行，必须快速返回（微秒级）；panic 必须被业务库 recover。
4. **接口演进**：走可选扩展接口（`ObserverV2` 嵌入 `Observer`）+ 分发点类型断言，非破坏性。
5. **分层规则**：热路径只做指标埋点，日志仅用于低频生命周期事件。
6. **纯旁路**：事件回调与指标写入不得改变业务执行语义。
7. **指标形态**：核心包只有无 label 指标与枚举拆名两种；带 label 指标只出现在用户侧预构建或业务库 adapter 子包。

完整规范见 [doc.go](doc.go) 与 [docs/observability-design.md](docs/observability-design.md)。

## 仓库结构

```
github.com/jninng/observ
├── meter.go / logger.go / attr.go   // Meter / Logger 契约 + Noop + slog 桥
├── contract/                        // 契约测试助手
├── adapters/prom/                   // client_golang 实现 observ.Meter
├── adapters/zaplog/                 // zap 实现 observ.Logger
└── docs/observability-design.md     // 设计文档
```

## 测试

```bash
go test -race ./...
cd adapters/prom && go test ./...
cd adapters/zaplog && go test ./...
```

## License

[MIT](LICENSE)
