module github.com/jninng/observ/adapters/zaplog

go 1.21

require (
	github.com/jninng/observ v0.0.0
	go.uber.org/zap v1.27.0
)

require go.uber.org/multierr v1.10.0 // indirect

replace github.com/jninng/observ => ../.. // 发布（打 tag）前移除，改用正式版本号
