package daemon

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path"
	"strings"
	"syscall"
	"time"

	"github.com/kardianos/osext"
	"github.com/takama/daemon"
	"github.com/zdypro888/crash"
)

//Run 运行
func Run(name, description string, dependencies ...string) bool {
	var flagInstall, flagRemove, flagStart, flagStop, flagStatus bool
	flag.BoolVar(&flagInstall, "install", false, "install services")
	flag.BoolVar(&flagRemove, "remove", false, "remove services")
	flag.BoolVar(&flagStart, "start", false, "start services")
	flag.BoolVar(&flagStop, "stop", false, "stop services")
	flag.BoolVar(&flagStatus, "status", false, "show services status")
	var flagArgs string
	flag.StringVar(&flagArgs, "args", "", "args for services")
	flag.Parse()
	if flagInstall || flagRemove || flagStart || flagStop || flagStatus {
		service, err := daemon.New(name, description, daemon.SystemDaemon, dependencies...)
		if err != nil {
			log.Printf("Init service faild: %v", err)
		} else if flagInstall {
			var args []string
			if flagArgs != "" {
				args = strings.Split(flagArgs, " ")
			}
			if result, err := service.Install(args...); err != nil {
				log.Printf("Install faild(%s): %v", result, err)
			} else {
				log.Printf("Install success: %s", result)
			}
		} else if flagRemove {
			if result, err := service.Remove(); err != nil {
				log.Printf("Remove faild(%s): %v", result, err)
			} else {
				log.Printf("Remove success: %s", result)
			}
		} else if flagStart {
			if result, err := service.Start(); err != nil {
				log.Printf("Start faild(%s): %v", result, err)
			} else {
				log.Printf("Start success: %s", result)
			}
		} else if flagStop {
			if result, err := service.Stop(); err != nil {
				log.Printf("Stop faild(%s): %v", result, err)
			} else {
				log.Printf("Stop success: %s", result)
			}
		} else if flagStatus {
			if result, err := service.Status(); err != nil {
				log.Printf("Get status faild(%s): %v", result, err)
			} else {
				log.Printf("%s", result)
			}
		}
		return false
	}
	return true
}

//RunWithConsole 运行
func RunWithConsole(name, description string, dependencies ...string) bool {
	var flagConsole bool
	flag.BoolVar(&flagConsole, "console", false, "with console output")
	if !Run(name, description, dependencies...) {
		return false
	}
	if !flagConsole {
		folder, err := osext.ExecutableFolder()
		if err != nil {
			log.Printf("get executable folder faild: %v", err)
			return false
		}
		if err = crash.InitPanicFile(path.Join(folder, fmt.Sprintf("%s_crash_%v.log", name, time.Now().Format("20060102")))); err != nil {
			log.Printf("open crash file faild: %v", err)
			return false
		}
		if err = crash.RedirectLog(path.Join(folder, fmt.Sprintf("%s.log", name))); err != nil {
			log.Printf("open log file faild: %v", err)
			return false
		}
	}
	return true
}

//WaitNotify 等待信号量
func WaitNotify() {
	interrupt := make(chan os.Signal, 1)
	defer close(interrupt)
	signal.Notify(interrupt, os.Interrupt, syscall.SIGTERM)
	signal := <-interrupt
	log.Printf("go system signal: %d", signal)
}
