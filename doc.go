// Package observ 是基础库共享可观测规范包。
//
// 各业务库接入规范（手册）：
//
//  1. 类型化 Observer：每个业务库在自己的包内定义事件与接口
//     （不进 observ 根模块）。方法名 On<Event>，参数为按值传递的
//     事件结构体；字段为值类型、string、error 或已存在且此后不可变
//     的 slice；禁止 map/指针、禁止在分发点构造/拷贝容器。
//  2. 注入用 option 模式：WithObserver(obs)/WithMeter(m) 默认 Noop；
//     WithLogger(l) 未注入时构造期取 observ.DefaultLogger() 并固定
//     （快照语义）。
//  3. 回调 panic 必须被业务库 recover；回调在调用方 goroutine 同步
//     执行，必须快速返回（微秒级）；业务库不提供异步分发。
//  4. 接口演进走可选扩展接口（ObserverV2 嵌入 Observer）+ 分发点
//     类型断言，非破坏性。
//  5. 分层规则：热路径只做指标埋点；日志仅用于低频生命周期事件。
//  6. 观测是纯旁路：事件回调与指标写入不得改变业务执行语义。
//  7. 核心包指标只有两种形态：无 label 指标、枚举拆名（枚举值入
//     指标名）。带 label 指标只出现在用户侧预构建或业务库 adapter
//     子包。
package observ
