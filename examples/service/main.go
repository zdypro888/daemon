package main

import (
	"github.com/zdypro888/daemon"
	_ "github.com/zdypro888/daemon/examples/server/docs"
)

func main() {
	service, err := daemon.NewService("service", "sample service")
	if err != nil {
		panic(err)
	}
	service.Usage()
	service.PanicFile("panic.log")
	service.RedirectLog("service.log")
	if err := service.Console(); err != nil {
		panic(err)
	}
	service.Graceful()
}
