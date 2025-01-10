package daemon

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/zdypro888/crash"
	takama "github.com/zdypro888/daemon/internal/daemon"
)

// Service represents a service
type Service struct {
	takama.Daemon
}

// NewService create a new service
func NewService(name, description string, dependencies ...string) (*Service, error) {
	td, err := takama.New(name, description, takama.SystemDaemon, dependencies...)
	if err != nil {
		return nil, err
	}
	return &Service{
		Daemon: td,
	}, nil
}

// Usage print usage information
func (service *Service) Usage() {
	fmt.Println("Usage: command <install|remove|start|stop|status> [flags]")
}

// Console parse command line arguments and execute an action
func (service *Service) Console() error {
	if len(os.Args) < 2 {
		return errors.New("no command specified")
	}
	command := os.Args[1]
	var err error
	switch command {
	case "install":
		installCmd := flag.NewFlagSet("install", flag.ExitOnError)
		args := installCmd.String("args", "", "Arguments for the service")
		_ = installCmd.Parse(os.Args[2:])
		err = service.Install(strings.Fields(*args)...)
	case "remove":
		err = service.Remove()
	case "start":
		err = service.Start()
	case "stop":
		err = service.Stop()
	case "status":
		var result string
		if result, err = service.Status(); err == nil {
			fmt.Println(result)
		}
	default:
		return fmt.Errorf("unknown command: %s", command)
	}
	return err
}

// PanicFile redirect panic output to a file
func (service *Service) PanicFile(filepath string) error {
	return crash.InitPanicFile(filepath)
}

// RedirectLog redirect log output to a file
func (service *Service) RedirectLog(filepath string) error {
	return crash.RedirectLog(filepath)
}

// Graceful wait for a signal to notify the service to stop
func (service *Service) Graceful() os.Signal {
	interrupt := make(chan os.Signal, 1)
	defer close(interrupt)
	signal.Notify(interrupt, os.Interrupt, syscall.SIGTERM)
	return <-interrupt
}
