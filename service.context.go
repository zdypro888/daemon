package daemon

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
)

// RunContext 在前台或系统服务中运行同一任务；停止服务时取消上下文并等待资源退出。
// 信号由调用者绑定到 ctx，Windows SCM 的 stop/shutdown 则由服务回调取消。
func (service *Service) RunContext(ctx context.Context, run func(context.Context) error) error {
	if ctx == nil || run == nil {
		return errors.New("service context and runner are required")
	}
	execution := newContextExecution(ctx, run)
	defer execution.Stop()
	err := service.Run(execution)
	execution.Stop()
	return errors.Join(err, execution.Err())
}

type contextExecution struct {
	ctx     context.Context
	cancel  context.CancelFunc
	run     func(context.Context) error
	start   sync.Once
	started atomic.Bool
	done    chan struct{}
	err     error // 关闭 done 后才读取，避免服务回调与任务退出竞争。
}

func newContextExecution(ctx context.Context, run func(context.Context) error) *contextExecution {
	ctx, cancel := context.WithCancel(ctx)
	return &contextExecution{ctx: ctx, cancel: cancel, run: run, done: make(chan struct{})}
}

func (execution *contextExecution) Start() {
	execution.start.Do(func() {
		execution.started.Store(true)
		go func() {
			defer close(execution.done)
			if execution.ctx.Err() != nil {
				return
			}
			execution.err = execution.run(execution.ctx)
			if errors.Is(execution.err, context.Canceled) && execution.ctx.Err() != nil {
				execution.err = nil
			}
		}()
	})
}

func (execution *contextExecution) Stop() {
	execution.cancel()
	if execution.started.Load() {
		<-execution.done
	}
}

func (execution *contextExecution) Run() {
	execution.Start()
	<-execution.done
}

func (execution *contextExecution) Done() <-chan struct{} { return execution.done }

func (execution *contextExecution) Err() error {
	select {
	case <-execution.done:
		return execution.err
	default:
		return nil
	}
}
