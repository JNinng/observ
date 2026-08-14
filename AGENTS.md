# AGENTS.md

## 仓库用途

`observ` 是基础库共享的可观测规范包：定义统一的 `Meter`/`Logger` 小接口与各业务库接入规范。根模块只定契约（含 Noop 实现与 slog 桥），不提供任何聚合/导出实现。

## 仓库结构（单仓多 Go module）

- 根模块 `github.com/jninng/observ`：`meter.go`、`logger.go`、`attr.go`、`doc.go`、`observ_test.go`
- `contract/`：`RunMeterContract`/`RunLoggerContract` 契约测试助手（基线档 + 深度档读回接口）
- `adapters/prom/`：独立 module，用 client_golang 实现 `observ.Meter`
- `adapters/zaplog/`：独立 module，用 zap 实现 `observ.Logger`
- `docs/observability-design.md`：设计文档，改根模块 API 或适配器前必读

## 常用命令（Windows / Git Bash）

```bash
go test ./...                     # 根模块测试
go test -race ./...               # 契约要求并发安全，测试须过 -race
cd adapters/prom && go test ./...
cd adapters/zaplog && go test ./...
```

多 module 通过根目录 `go.work` 联编（`go 1.26.2` 工具链，各 module 最低 `go 1.21`）。

## 依赖规则（硬约束）

- 根模块**零第三方依赖**：`go.mod` 的 require 块必须为空，只准用 stdlib（`log/slog` 属 stdlib）。
- 依赖单向：业务库 → 根模块；适配器 → 根模块 + 各自的第三方库。根模块**禁止 import `adapters/*`**（会成环）；适配器之间互不依赖。
- `contract` 包是唯一例外：公开导出、import `testing`（stdlib，明示取舍），供跨 module 适配器复用契约测试。

## API 约定

- 不自定义 Level 类型：日志级别直接复用 `slog.Level`/`slog.Attr`。
- `DefaultLogger()` 为构造期快照语义：业务库构造函数读一次并固定；经 `atomic.Pointer` 原子替换，初始 Noop。
- `NoopMeter` 是可比较零大小值类型，业务库用 `meter == observ.NoopMeter` 整体跳过埋点。
- 指标命名：attr/指标名 snake_case，对齐 OTel 语义约定（见 `attr.go`）；核心包指标只有无 label 与枚举拆名两种形态。
- 各业务库的 Observer 事件不进 observ 根模块（规范手册见 `doc.go` 与设计文档第 5 节）：事件按值传递、禁 map/指针、回调必须快速返回且由业务库 recover panic。

## 文档与注释惯例

- 代码注释与文档使用中文；公共 API 必须有说明注释。
- 版本策略：根模块与各适配器 module 独立打 tag（`v1.2.3` 与 `adapters/prom/v0.1.0` 风格）。
