// 示例：同一工作循环支持前台信号与系统服务停止。
package main

import (
	"context"
	"errors"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/zdypro888/daemon"
)

func main() {
	service, err := daemon.NewService("example-service", "sample daemon service")
	if err != nil {
		log.Fatal(err)
	}
	// 管理子命令完成后退出；无子命令才进入实际工作循环。
	if err = service.Console(); err == nil {
		return
	} else if !errors.Is(err, daemon.ErrNoCommand) {
		log.Fatal(err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err = service.RunContext(ctx, func(ctx context.Context) error {
		log.Print("service started")
		<-ctx.Done()
		// 在这里关闭连接、落盘并归还锁；回调返回后服务才算停止。
		log.Print("service stopped")
		return nil
	}); err != nil {
		log.Fatal(err)
	}
}
