// prom 适配器使用示例：注册指标、暴露 /metrics 端点并演示业务库
// 经 observ.Meter 小接口埋点的典型形态。
package main

import (
	"log"
	"net/http"

	"github.com/jninng/observ"
	"github.com/jninng/observ/adapters/prom"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// worker 演示业务库的标准接入形态：构造函数接收 observ.Meter，
// 构造期创建指标（New* 禁止热路径），并固定默认 Logger 快照。
type worker struct {
	tasksTotal  observ.Counter
	taskSeconds observ.Histogram
	queueDepth  observ.Gauge
}

func newWorker(m observ.Meter) *worker {
	return &worker{
		// 计数器带 _total、耗时带 _seconds，命名对齐 attr.go 约定。
		tasksTotal: m.NewCounter("worker_tasks_total", "已处理任务总数"),
		taskSeconds: m.NewHistogram("worker_task_seconds", "单任务处理耗时",
			[]float64{0.005, 0.01, 0.05, 0.1, 0.5, 1}),
		queueDepth: m.NewGauge("worker_queue_depth", "待处理队列深度"),
	}
}

func (w *worker) handle() {
	w.queueDepth.Add(-1)
	w.tasksTotal.Inc()
	w.taskSeconds.Observe(0.042)
}

func main() {
	// reg 为 nil 时使用 prometheus.DefaultRegisterer；
	// 生产中建议传入独立的 prometheus.NewRegistry()。
	m := prom.New(nil)
	w := newWorker(m)

	w.queueDepth.Set(10)
	w.handle()

	http.Handle("/metrics", promhttp.Handler())
	log.Println("metrics 监听于 :9090/metrics")
	if err := http.ListenAndServe(":9090", nil); err != nil {
		log.Fatal(err)
	}
}
