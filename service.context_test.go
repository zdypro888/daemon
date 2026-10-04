package daemon

import (
	"context"
	"errors"
	takama "github.com/zdypro888/daemon/internal/daemon"
	"sync/atomic"
	"testing"
	"time"
)

type contextDaemonStub struct {
	takama.Daemon
	dispatch func(takama.Executable) error
}

func (daemon *contextDaemonStub) Run(task takama.Executable) error { return daemon.dispatch(task) }

func TestRunContextForegroundAndFailure(t *testing.T) {
	cause := errors.New("worker initialization failed")
	service := &Service{Daemon: &contextDaemonStub{dispatch: func(task takama.Executable) error { task.Run(); return nil }}}
	if err := service.RunContext(context.Background(), func(context.Context) error { return cause }); !errors.Is(err, cause) {
		t.Fatalf("worker failure lost: %v", err)
	}
	if err := service.RunContext(context.Background(), func(context.Context) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if err := service.RunContext(nil, func(context.Context) error { return nil }); err == nil {
		t.Fatal("nil context accepted")
	}
	if err := service.RunContext(context.Background(), nil); err == nil {
		t.Fatal("nil worker accepted")
	}
}

func TestServiceStopWaitsForResourceRelease(t *testing.T) {
	entered := make(chan struct{})
	canceled := make(chan struct{})
	release := make(chan struct{})
	finished := make(chan struct{})
	task := newContextExecution(context.Background(), func(ctx context.Context) error {
		close(entered)
		<-ctx.Done()
		close(canceled)
		<-release
		close(finished)
		return ctx.Err()
	})
	task.Start()
	<-entered
	stopped := make(chan struct{})
	go func() { task.Stop(); close(stopped) }()
	select {
	case <-canceled:
	case <-time.After(time.Second):
		t.Fatal("stop did not cancel worker")
	}
	select {
	case <-stopped:
		t.Fatal("stop returned while worker still owned resources")
	default:
	}
	close(release)
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("stop did not finish")
	}
	select {
	case <-finished:
	default:
		t.Fatal("resource cleanup missing")
	}
	task.Stop()
	if task.Err() != nil {
		t.Fatalf("normal shutdown treated as failure: %v", task.Err())
	}
}

func TestContextExecutionStartsOnlyOnceAndKeepsParent(t *testing.T) {
	type key struct{}
	parent, cancel := context.WithCancel(context.WithValue(context.Background(), key{}, "owner"))
	defer cancel()
	var count atomic.Int32
	entered := make(chan struct{})
	task := newContextExecution(parent, func(ctx context.Context) error {
		count.Add(1)
		if ctx.Value(key{}) != "owner" {
			t.Error("parent metadata lost")
		}
		close(entered)
		<-ctx.Done()
		return ctx.Err()
	})
	task.Start()
	task.Start()
	<-entered
	cancel()
	task.Stop()
	if count.Load() != 1 || task.Err() != nil {
		t.Fatalf("count=%d err=%v", count.Load(), task.Err())
	}
}

func TestCanceledServiceNeverStartsWorker(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	task := newContextExecution(ctx, func(context.Context) error { t.Error("canceled task initialized resources"); return nil })
	task.Run()
	if task.Err() != nil {
		t.Fatal(task.Err())
	}
}

func TestDispatcherFailureDoesNotStartWorker(t *testing.T) {
	cause := errors.New("service dispatcher unavailable")
	service := &Service{Daemon: &contextDaemonStub{dispatch: func(takama.Executable) error { return cause }}}
	if err := service.RunContext(context.Background(), func(context.Context) error { t.Error("failed dispatcher started worker"); return nil }); !errors.Is(err, cause) {
		t.Fatal(err)
	}
}
