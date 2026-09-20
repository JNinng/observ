module github.com/jninng/observ/adapters/zaplog

go 1.21

require (
	github.com/jninng/observ v0.1.1
	go.uber.org/zap v1.27.0
)

require go.uber.org/multierr v1.10.0 // indirect

// 临时指向未发布的 observ v0.2.0（Logger 带 ctx）；根模块发 tag 后移除并 bump 正式版
replace github.com/jninng/observ => ../..
