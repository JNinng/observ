# observ — 基础库共享可观测规范包 设计文档

日期：2026-08-14

## 1. 背景与问题

工程资产中有一组基础库接入用户项目时都面临同一个问题：

- **日志库不可知**：用户项目可能用 slog、zap、zerolog，库内不能硬编码
  `log.Printf`。
- **指标出口不可知**：指标可能输出到控制台、文件或数据库，不一定是
  Prometheus，不能强制依赖 client_golang。
- **性能敏感**：上观测手段不能引入可感知的开销。

本规范定义统一的共享观测接入规范包 `observ`，并规定各基础库的接入方式：
所有基础库按同一模式接入，用户项目只需要实现/选用一次桥接。

## 2. 规范约定

以下为硬性约定，observ 实现方与各业务库必须遵守：

| 约定   | 规范内容                                                                                                       |
|------|------------------------------------------------------------------------------------------------------------|
| 统一接入 | 各业务库统一引用 observ 根模块；用户侧只实现/选用一次桥接，全线复用                                                                     |
| 事件形态 | 类型化 Observer 接口：每类事件一个固定结构体，方法 `OnXxx(XxxEvent)` 按值传递；无反射、无装箱、无变参切片                                        |
| 指标接口 | 自定义小接口，签名对齐 prometheus/otel 子集；根模块零第三方依赖                                                                   |
| 日志桥接 | 根模块定义 Logger 接口（2 方法，复用 stdlib `slog.Level`/`slog.Attr`），任意日志库直接实现即接入；slog 经根模块自带桥，zap 经 `adapters/zaplog` |
| 默认回落 | 业务库选项未注入 Logger 时，构造期取 `observ.DefaultLogger()`（原子替换、初始 Noop、构造期快照），见 4.4                                  |
| 仓库形态 | 单 Git 仓库多 Go module（根模块 + adapters/* 子模块）；根模块 `go.mod` 的 require 块必须为空                                     |
| 最低版本 | 全线最低 Go 1.21（`log/slog` 进 stdlib 的版本），根模块与各适配器/业务库统一                                                       |

分层规则：

- 热路径只做指标埋点（计数器累加、直方图观测），不写日志。
- 日志仅用于低频生命周期事件：失败、恢复、队列满等。

## 3. 仓库结构与依赖规则（硬约束）

```
github.com/jninng/observ            ← Git 仓库
├── go.mod                          ← module github.com/jninng/observ（根模块，go 1.21）
├── meter.go                        // Meter / Counter / Gauge / Histogram + NoopMeter
├── logger.go                       // Logger（对接任意日志库）+ NoopLogger + NewSlogLogger
├── attr.go                         // 属性命名约定（snake_case，对齐 OTel 语义约定）
├── doc.go                          // 各业务库定义 Observer 的规范手册（见第 5 节）
├── contract/
│   └── contract.go                 // RunMeterContract / RunLoggerContract 契约测试助手 + 读回能力接口（见第 6 节）
├── observ_test.go                  // Noop 并发安全（-race）等根模块测试
└── adapters/
    ├── prom/                       ← module github.com/jninng/observ/adapters/prom
    │   ├── go.mod                  // require client_golang + github.com/jninng/observ
    │   └── prom.go                 // 用 client_golang 实现 observ.Meter
    └── zaplog/                     ← module github.com/jninng/observ/adapters/zaplog
        ├── go.mod                  // require go.uber.org/zap + github.com/jninng/observ
        └── zaplog.go               // 用 zap 实现 observ.Logger（见 5.2）
```

依赖规则（单向，硬约束）：

```text
业务库 ──→ observ 根模块 ──→ (仅 stdlib，含 log/slog)
用户项目 ──→ observ/adapters/prom ──→ observ 根模块 + client_golang
用户项目 ──→ observ/adapters/zaplog ──→ observ 根模块 + go.uber.org/zap
```

- 根模块**零第三方依赖**，`go.mod` 的 require 块必须为空（Go 1.21+
  下 `log/slog` 属 stdlib，引入 Logger 接口不破坏此约束）。
- 根模块**禁止 import `adapters/*`**（否则形成环）。
- 适配器之间互不依赖；一个适配器一个第三方目标库。
- 业务库默认只依赖根模块。只有当业务库想给用户"开箱即用的 prom 埋点"
  时，才在自己的 adapter 子包里引 `observ/adapters/prom`，核心包仍然不引。

版本策略：根模块与各适配器模块独立打 tag（`v1.2.3` 与
`adapters/prom/v0.1.0` 风格），互不绑架升级。

## 4. 根模块 API

### 4.1 Level（不自定义）

根模块**不定义自己的 Level 类型**，日志级别直接复用 stdlib
`slog.Level`（含介于两级间的自定义级别）：

- 事件不携带级别——各业务库的日志观察者按事件类型硬编码级别
  （如恢复失败事件固定 `slog.LevelError`），用户侧无需再判断。
- Meter 不需要级别；各日志库的级别映射（`slog.Level` → zapcore /
  zerolog 等）由 Logger 实现方负责（见 4.4）。

### 4.2 Meter（指标）

```go
// 签名刻意与 prometheus/otel 对齐，转化零阻力
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
```

- **无 label 参数**：带 label 的指标由 Meter 实现方在适配层组合
  （见 5.3），业务库埋点处不拼 label slice。
- 不设独立 `Timer` 接口：耗时指标统一用 `Histogram`，单位固定为秒
  （对齐 prometheus `_seconds` 惯例）。
- `NoopMeter`：**可比较的零大小值类型**（`type noopMeter struct{}`
  值接收者空方法体），以 `var NoopMeter Meter = noopMeter{}` 导出；
  可内联，单次调用 ~1ns；重复 `New*` 恒正常返回、恒不 panic。
  业务库判断 `meter == observ.NoopMeter` 时可整体跳过埋点代码块。
  门控为**尽力优化（best-effort）**：仅对 observ 自身导出的 Noop
  值命中；用户传入自定义 noop 时不命中，走空方法体内联路径，
  开销可忽略。正确性不依赖门控命中，实现方不得以包装类型破坏
  该相等性。
- 根模块不提供任何聚合/导出实现——导出由适配器与用户侧负责。

Meter 契约（硬性，按"可测语义 / 使用规范"两类约束，契约测试
按第 6 节基线/深度两档断言）：

可测语义（契约测试断言）：

- **并发安全**：`Meter` 及其产出的 Counter/Gauge/Histogram 必须支持
  并发使用；契约测试在 `-race` 下验证。
- **同名重复 `New*` 与负值 `Counter.Add`**：仅允许两种结局——
  正常返回或 panic，不得挂死、不得静默破坏后续调用（若选择正常
  返回，后续接口调用必须仍可用）。实现可自行选择 panic
  （对齐 client_golang）或容忍；`NoopMeter` 恒不 panic。
  Counter 负值语义上仍为编程错误，业务库负责保证非负。
- **`buckets` 所有权**：`NewHistogram` 返回后 buckets 归实现所有，
  调用方不得再修改。实现提供读回能力（见 §6 深度档）时由契约
  测试验证，否则为文档化约定。

使用规范（不可测，由文档与评审约束）：

- **`New*` 调用时机**：仅允许在构造期（包 init / 工厂函数）调用，
  禁止热路径调用（prom 实现在 `New*` 时注册 registry）。
- 调用方应以包级变量持有 `New*` 产物与 buckets。

### 4.3 命名约定（attr.go / doc.go）

- **指标名**：`snake_case`，带单位后缀（`_total`、`_seconds`、
  `_bytes`）。
- **属性键**：对齐 OTel semantic conventions 子集——OTel 原名带
  点号的保留点号（`error.type`），其余为 `snake_case`（`run_id`、
  `workflow`）；带单位的属性统一用 `_seconds`/`_bytes` 后缀
  （如 `duration_seconds`），与指标单位口径一致，禁止 `_s` 等
  简写变体。各库复用同名键，保证用户侧跨库聚合口径一致。
- **名称字符集（软约束，不做契约测试）**：业务库产出的指标名
  匹配 `[a-z_][a-z0-9_]*`；属性键每段同前，段间允许 OTel 点号
  （`error.type`）。适配器遇到非法名称时的处置（报错或规范化）
  由实现方文档写明，**推荐报错而非静默改名**——prom 原生会静默
  清洗非法字符，适配器若直接透传，同一指标名经不同适配器会产出
  不同的实际名称，与"跨库聚合口径一致"目标（见上条）冲突。
- **基数红线**：label/属性值只允许枚举值与标识符（run_id、工作流名），
  禁止业务 payload 进基数。

### 4.4 Logger（日志，对接任意日志库）

```go
// Logger 是日志侧的统一落点：任意日志库实现两个方法即可接入，
type Logger interface {
Enabled(level slog.Level) bool
Log(level slog.Level, msg string, attrs ...slog.Attr)
}

var NoopLogger Logger = noopLogger{}   // Enabled 恒 false

// slog/std 用户：根模块自带桥（slog 属 stdlib，零依赖不变）
func NewSlogLogger(l *slog.Logger) Logger

// 包级默认 Logger：原子读写、永不返回 nil、初始为 NoopLogger
func DefaultLogger() Logger
func SetDefaultLogger(l Logger) (old Logger) // 传 nil 等价重置为 Noop
```

Logger 契约（硬性；可测项由契约测试按基线/深度两档断言，
见第 6 节）：

- **属性载体**：复用 stdlib `slog.Attr` 及 `slog.String` 等构造器，
  键名遵循 4.3 命名约定；实现按 `slog.Value.Kind` 显式分支处理，
  仅 `KindAny` 走反射兜底。
- **Enabled 门控**：调用方先判 `Enabled` 再构造 attrs，未启用时零
  构造成本；无 attrs 调用零分配。日志本就限定低频事件（§2 分层
  规则），变参切片开销可接受。
- **并发安全**：实现必须支持并发调用；`attrs` 切片传给 `Log` 后
  归实现所有，调用方不得复用/修改。
- **纯旁路**：`Log` 不得 panic、不得阻塞业务（落盘/网络 IO 由实现
  自行异步化），违反即实现方 bug。
- **级别映射**：由实现方负责（如 `slog.Level` ↔ zapcore.Level 四级
  一一对应、中间级别就近向下取整），映射表写入实现方 doc。
- **包级默认（DefaultLogger）**：经 `atomic.Pointer` 读写，永不返回
  nil，初始为 NoopLogger（不设置任何东西的用户保持零输出、零开销）；
  `SetDefaultLogger` 原子替换并返回旧值（供测试恢复）。**构造期快照
  语义**：业务库在构造函数中读取一次并固定，运行期替换仅影响之后
  构造的组件。业务库选项未注入 Logger 时回落到它（见 5.1）；
  observ 不提供其他任何包级可变全局。

zap 经 `adapters/zaplog` 直接实现 Logger（见 5.2）；其余任意日志库
（zerolog、logrus、自研等）由用户实现这两个方法即接入，无需
slog Handler，也无需 observ 新增适配器。

## 5. 业务库接入规范（各库照此执行）

### 5.1 类型化 Observer

每个业务库在自己的包内定义事件与接口（不进 observ 根模块——事件
天然是库 API 的一部分）：

```go
// 以某业务库为例（仅示意，事件与接口由各库在自身包内定义）
type Observer interface {
OnRunStarted(ev RunStartedEvent) // ev: RunID, Workflow
OnRunCompleted(ev RunCompletedEvent) // ev: RunID, Status, Duration
OnRunFailed(ev RunFailedEvent)       // ev: RunID, Error
OnRecoverFailed(ev RecoverFailedEvent) // ev: RunID, Error
OnQueueFull(ev QueueFullEvent)         // ev: QueueName, Depth
}
type NoopObserver struct{} // 空方法体，内联，~1-2ns/次
```

规范要点：

- 方法名 `On<Event>`，参数为**按值传递的事件结构体**。字段为值类型、
  string、error，或**已存在且此后不可变**的 slice（传递不逃逸即零
  分配）；禁止 map/指针，禁止在事件分发点构造/拷贝容器。批量类事件
  （如一批任务失败）允许携带不可变 slice 字段。
- 注入用各库既有的 option 模式：`WithObserver(obs)` / `WithMeter(m)`
  默认 Noop；`WithLogger(l)` 未注入时构造期取
  `observ.DefaultLogger()` 并固定（快照语义，见 4.4）。
- 回调 panic 必须被业务库 recover，绝不影响主流程。事件均为低频，
  业务库在事件分发点用 `defer` + `recover` 兜底并降级为内部计数即可，
  不引入每次调用的闭包包装。
- **回调时效契约**：回调在调用方 goroutine 同步执行，必须快速返回
  （微秒级）；落盘、网络等 IO 由用户侧 Handler/Exporter 自行异步化，
  业务库不提供异步分发。
- **接口演进（非破坏性）**：新增事件不改核心接口，走**可选扩展接口**
    + 分发点类型断言：

  ```go
  type Observer interface {          // 核心接口，稳定
      OnRunStarted(ev RunStartedEvent)
      OnRunCompleted(ev RunCompletedEvent)
  }
  type ObserverV2 interface {        // 扩展接口，按需追加事件
      Observer
      OnRunRetried(ev RunRetriedEvent)
  }

  // 业务库分发点：
  switch o := obs.(type) {
  case ObserverV2:
      o.OnRunRetried(ev)
  }
  ```

  未实现扩展接口的观察者（含以嵌入核心接口方式实现的用户代码）自动
  跳过新事件，非破坏性变更。修改/删除既有事件字段与方法仍为破坏性
  变更，走 major 版本；观测点显式入契约。
- 观测是**纯旁路**：事件回调与指标写入不得改变业务执行语义、
  持久化内容或对外行为。

### 5.2 日志桥接（→ observ.Logger，对接任意日志库）

每个业务库自带一个 `obs_bridge.go`：`NewLogObserver(l observ.Logger)
Observer`，内部是**显式字段映射**（按事件类型固定级别，无反射）：

```go
func (o logObserver) OnRecoverFailed(ev RecoverFailedEvent) {
if !o.log.Enabled(slog.LevelError) {
return
}
o.log.Log(slog.LevelError, "recover failed",
slog.String("run_id", ev.RunID),
slog.Any("error", ev.Err))
}
```

各日志库落地路径（业务库自身不为任何第三方日志库维护代码）：

| 日志库                   | 做法                                                           |
|-----------------------|--------------------------------------------------------------|
| slog / stdlib         | `observ.NewSlogLogger(logger)`（根模块自带桥）                       |
| zap                   | `zaplog.New(zapLogger)` 返回 `observ.Logger`（见下）               |
| zerolog、logrus、自研等    | 用户实现 `observ.Logger` 两个方法即接入，无需 slog Handler、无需 observ 新增适配器 |
| 已全站 slog.Handler 化的项目 | `observ.NewSlogLogger(slog.New(handler))` 同样适用               |

#### zaplog 适配器（zap 落地）

`adapters/zaplog` 用 zap 直接实现 `observ.Logger`：

```go
func New(zl *zap.Logger) observ.Logger
```

- **级别映射**：slog 四级与 zapcore 四级一一对应；介于两级间的
  自定义级别就近向下（更严重）取整。
- **属性编码**：`slog.Attr` 逐个显式转 zap Field；键名按 4.3 的
  snake_case 原样透传；`LogValuer` 在编码前解析（对齐 slog handler
  语义）；组属性以点号前缀展平，空名组内联；除 `KindAny` 兜底外
  无反射。
- **存在理由**：zap 官方桥 `go.uber.org/zap/exp/zapslog` 位于实验性
  `exp/` 目录、无 API 稳定承诺，且方向是 slog.Handler 适配；zaplog
  直连 observ.Logger，由本仓库锁定语义、独立打 tag。

典型用法：

```go
obs := lib.NewLogObserver(zaplog.New(zapLogger))
```

### 5.3 指标埋点

业务库核心包（只依赖根模块）经 `WithMeter` 注入的 Meter 埋点，指标名
遵循 4.3 命名约定：计数器带 `_total`、耗时带 `_seconds`、队列/资源
状态量带 `_depth`。受根接口无 label 约束，核心包指标只有两种形态：

1. **无 label 指标**：`queue_depth`、`execute_duration_seconds`。
2. **枚举拆名**：枚举值入指标名，如 `runs_completed_total`、
   `runs_failed_total`（替代 `runs_total{status}` 风格）。

带 label 的指标（`runs_total{status}` 风格）只出现在以下位置，
**核心包不得产出**：

1. **用户侧预构建**：用户在 Meter 实现层用事件字段选择/缓存带维度
   的 Counter 实例。
2. **业务库 adapter 子包**：依赖 `observ/adapters/prom` 的变体 API
   （如 `NewCounterVec` 包装），随"开箱即用 prom 埋点"一起提供。

label 变体 API 的具体形态在 prom 适配器实施时定稿，根模块接口不变。

用户指标出口路径：

| 出口         | 做法                                            |
|------------|-----------------------------------------------|
| Prometheus | 用户引 `observ/adapters/prom`（或业务库自带 adapter 子包） |
| 控制台/文件     | 用户自行实现 4 接口 + 定期 dump（根模块不放导出器）               |
| 数据库        | 同上，实现 Meter 写表；高频场景实现内部做批量/异步                 |

## 6. 测试策略

- **根模块**：Noop 并发安全（`-race`）；Meter/Logger 契约测试以
  **公开助手**形式提供，位于根模块 `contract/` 子包：
  `contract.RunMeterContract(t *testing.T, new func() Meter)` 与
  `contract.RunLoggerContract(t *testing.T, new func() Logger)`。
  import "testing" 为**明示取舍**：testing 属 stdlib，不破坏零第三方
  依赖；`_test.go` 无法跨 module 复用、`internal/` 跨 module
  不可见，故契约助手必须公开。鉴于 `Meter`/`Logger` 接口本身
  **无读回能力**（接口上没有取值方法），契约测试分两档，避免
  断言超出接口能力：
    - **基线档（全部实现必测）**：并发安全（`-race`）；同名重复
      `New*` 与负值 `Add` 的两结局断言（正常返回或 panic，见 4.2，
      经 recover 验证且后续调用仍可用）；Logger 不 panic、并发安全、
      `NoopLogger.Enabled` 恒 false、`Log` 为空操作。
    - **深度档（可选，实现附加读回能力接口后自动启用）**：契约包
      定义读回能力接口——Counter/Gauge 附加 `Value() float64`，
      Histogram 附加 `Count() uint64; Sum() float64;
    Bounds() []float64`，Logger 附加 `Records() []Record`
      （`Record` 含 level/msg/attrs）。`New*` 返回值或被测 Logger
      实现了对应接口时，追加断言：调用序列 golden 值（Inc/Add/
      Set/Observe 后的 Value/Count/Sum）、buckets 所有权（传入后
      修改调用方切片，观察结果与 Bounds 不受影响）、`Enabled`
      门控下零输出、级别透传与 attrs 内容。
      `New*` 调用时机与 NoopMeter 门控属使用规范，不可测（见 4.2）。
      DefaultLogger 的原子替换、永不 nil、初始 Noop、构造期快照语义
      在根模块测试中直接断言（`-race`，含 `SetDefaultLogger`
      旧值恢复）。
- **适配器**：prom 复用根模块 `contract` 契约测试——适配层为产物
  包装读回接口，启用深度档 golden 断言——另加 registry 语义断言
  （指标名/类型/help）；zaplog 复用 `contract` 的 Logger 契约
  测试——测试侧用记录型 zapcore Core 实现 `Records()` 启用深度
  档——另加内存 Core 断言字段/级别映射。
- **业务库**：FakeObserver 记录事件序列做断言；性能敏感业务库
  加基准测试证明 Noop 路径零分配（`testing.AllocsPerRun`）。

## 7. 实施范围与顺序

本期（新仓库 github.com/jninng/observ）：

1. 根模块：Meter/Counter/Gauge/Histogram、NoopMeter、
   命名约定文档、`contract` 契约测试助手（Go 1.21）。
2. `adapters/prom`（如需要可同期）：client_golang 实现 Meter。
3. `adapters/zaplog`：zap 实现 observ.Logger（复用 Logger 契约测试）。
4. 其余 **Meter** 适配器（otel、文件/控制台 dump、数据库等）按真实
   需求出现再建，同构复制；日志侧不再新增适配器（见第 8 节）。

后续（各业务库各自排期，本文档只约束模式）：

5. 各业务库接入：将内部直出日志（如 `log.Printf`）替换为类型化事件 +
   observ.Logger 桥接 + Meter 埋点，观测保持纯旁路（见 5.1）。
6. 热更新管理器、请求折叠等库照第 5 节规范接入。

## 8. 范围外（明确不做）

- 不做 tracing（OpenTelemetry trace）——等有跨库链路追踪需求再议。
  **已知负债（记录在案）**：当前事件方法无 ctx 参数、事件结构无
  span 关联字段，`Logger.Log` 同样无 ctx（日志侧届时无法关联
  span/从 ctx 取请求级信息）。演进路径：事件侧届时扩展事件签名
  或引入带 ctx 的分发变体，可能构成各业务库的破坏性变更（走
  major）；Logger 侧走可选扩展接口（如 `LogCtx(ctx, ...)`）+
  分发点类型断言，与 5.1 Observer 模式一致，不破坏现有两方法
  实现。此为接受的取舍。
- 根模块核心接口（`Meter`/`Counter`/`Gauge`/`Histogram`、
  `Logger`）视为冻结：小版本演进一律走**可选能力接口/扩展接口**
  ——先例为 §6 读回能力接口，分发模式同 §5.1 类型断言；仅当修改
  方法签名或语义时走 major。不以未导出方法封锁接口——用户自行
  实现 Meter/Logger 是受支持的用法（见 5.3 出口路径表）。
- 根模块不做任何日志格式化、聚合、导出实现。
- 除 `DefaultLogger`（见 4.4，原子、初始 Noop、构造期快照）外，
  不引入任何包级可变全局状态；业务库同样不得自建包级可变全局。
