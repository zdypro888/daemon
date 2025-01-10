// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by
// license that can be found in the LICENSE file.

package daemon

import (
	"errors"
	"runtime"
	"strings"
)

// Status constants.
const (
	statNotInstalled = "Service not installed"
)

// Daemon interface has a standard set of methods/commands
type Daemon interface {
	// GetTemplate - gets service config template
	GetTemplate() string

	// SetTemplate - sets service config template
	SetTemplate(string) error

	// Install the service into the system
	Install(args ...string) error

	// Remove the service and all corresponding files from the system
	Remove() error

	// Start the service
	Start() error

	// Stop the service
	Stop() error

	// Status - check the service status
	Status() (string, error)

	// Run - run executable service
	Run(e Executable) error
}

// Executable interface defines controlling methods of executable service
type Executable interface {
	// Start - non-blocking start service
	Start()
	// Stop - non-blocking stop service
	Stop()
	// Run - blocking run service
	Run()
}

// Kind is type of the daemon
type Kind string

const (
	// UserAgent is a user daemon that runs as the currently logged in user and
	// stores its property list in the user’s individual LaunchAgents directory.
	// In other words, per-user agents provided by the user. Valid for macOS only.
	UserAgent Kind = "UserAgent"

	// GlobalAgent is a user daemon that runs as the currently logged in user and
	// stores its property list in the users' global LaunchAgents directory. In
	// other words, per-user agents provided by the administrator. Valid for macOS
	// only.
	GlobalAgent Kind = "GlobalAgent"

	// GlobalDaemon is a system daemon that runs as the root user and stores its
	// property list in the global LaunchDaemons directory. In other words,
	// system-wide daemons provided by the administrator. Valid for macOS only.
	GlobalDaemon Kind = "GlobalDaemon"

	// SystemDaemon is a system daemon that runs as the root user. In other words,
	// system-wide daemons provided by the administrator. Valid for FreeBSD, Linux
	// and Windows only.
	SystemDaemon Kind = "SystemDaemon"
)

// New - Create a new daemon
//
// name: name of the service
//
// description: any explanation, what is the service, its purpose
//
// kind: what kind of daemon to create
func New(name, description string, kind Kind, dependencies ...string) (Daemon, error) {
	switch runtime.GOOS {
	case "darwin":
		if kind == SystemDaemon {
			return nil, errors.New("invalid daemon kind specified")
		}
	case "freebsd":
		if kind != SystemDaemon {
			return nil, errors.New("invalid daemon kind specified")
		}
	case "linux":
		if kind != SystemDaemon {
			return nil, errors.New("invalid daemon kind specified")
		}
	case "windows":
		if kind != SystemDaemon {
			return nil, errors.New("invalid daemon kind specified")
		}
	}

	return newDaemon(strings.Join(strings.Fields(name), "_"), description, kind, dependencies)
}
