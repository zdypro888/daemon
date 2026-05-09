// Sample service that registers itself with the OS service manager and survives restarts.
//
// 用法:
//
//	go build -o example-service .
//	sudo ./example-service install --args="arg1 arg2"
//	sudo ./example-service start
//	sudo ./example-service status
//	sudo ./example-service stop
//	sudo ./example-service remove
package main

import (
	"errors"
	"log"

	"github.com/zdypro888/daemon"
)

func main() {
	service, err := daemon.NewService("example-service", "sample daemon service")
	if err != nil {
		log.Fatalf("create service: %v", err)
	}

	// 把 stderr / log 重定向到文件, 便于服务化跑起来时排错。失败不致命, 继续跑。
	if err := service.PanicFile("panic.log"); err != nil {
		log.Printf("init panic file: %v", err)
	}
	if err := service.RedirectLog("service.log"); err != nil {
		log.Printf("init log file: %v", err)
	}

	if err := service.Console(); err != nil {
		// 没传子命令时打印 usage 而不是 panic — 之前 example panic 看着像 bug。
		if errors.Is(err, daemon.ErrNoCommand) {
			service.Usage()
			return
		}
		log.Fatalf("service command failed: %v", err)
	}

	// 阻塞直到收到 SIGINT/SIGTERM
	service.Graceful()
}
