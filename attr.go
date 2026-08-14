package observ

// 属性命名约定（对齐 OTel semantic conventions 子集）：
//   - OTel 原名带点号的保留点号（error.type），其余 snake_case；
//   - 带单位的属性统一用 _seconds/_bytes 后缀，禁止 _s 等简写变体；
//   - 各库复用同名键，保证用户侧跨库聚合口径一致；
//   - 基数红线：属性值只允许枚举值与标识符，禁止业务 payload 进基数。
const (
	// AttrRunID 一次执行/运行的标识符。
	AttrRunID = "run_id"
	// AttrWorkflow 工作流/任务名。
	AttrWorkflow = "workflow"
	// AttrStatus 结果状态枚举（succeeded/failed/...）。
	AttrStatus = "status"
	// AttrErrorType OTel 语义约定原样保留点号。
	AttrErrorType = "error.type"
	// AttrDurationSeconds 耗时，单位秒（与 _seconds 指标口径一致）。
	AttrDurationSeconds = "duration_seconds"
	// AttrQueueName 队列标识。
	AttrQueueName = "queue_name"
	// AttrQueueDepth 队列深度（资源状态量）。
	AttrQueueDepth = "queue_depth"
)

// 指标命名约定：snake_case，计数器带 _total、耗时带 _seconds、
// 队列/资源状态量带 _depth。业务库产出的指标名应匹配
// [a-z_][a-z0-9_]*（软约束，不做契约测试）。
