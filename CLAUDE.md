# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is a cross-platform Go library for creating and managing system services/daemons. It provides a unified API to install, remove, start, stop, and query services across macOS (launchd), Linux (systemd/upstart/systemv), FreeBSD, and Windows (Service Control Manager).

The library also includes an HTTP server engine built on Gin with automatic HTTPS/TLS support via Let's Encrypt (ACME autocert), TUS resumable file upload support, and graceful shutdown capabilities.

## Build and Test Commands

```bash
# Build the project
go build ./...

# Run tests (if any are added)
go test ./...

# Build example service
go build -o example-service ./examples/service

# Build example HTTP server
go build -o example-server ./examples/server

# Generate Swagger docs for server example
cd examples/server
swag init

# Run the example service (requires root/admin on most platforms)
sudo ./example-service install --args="arg1 arg2"
sudo ./example-service start
sudo ./example-service status
sudo ./example-service stop
sudo ./example-service remove
```

## Architecture

### Core Components

**Service Layer** (`service.go`):
- High-level `Service` struct wrapping the platform-specific daemon implementations
- Provides `Console()` method for CLI-based service management (install/remove/start/stop/status)
- Includes `Graceful()` for signal-based shutdown and crash handling utilities (`PanicFile`, `RedirectLog`)

**Engine Layer** (`engine.go`):
- HTTP server wrapper around Gin framework
- `Start(addr)`: Start HTTP server with auto-restart on failures
- `StartTLS(addr, hosts...)`: Start HTTPS with Let's Encrypt autocert, automatic HTTP→HTTPS redirect
- `TUSFileComposer(path)`: Configure TUS resumable upload storage
- `TUSHandle(basePath, composer)`: Mount TUS upload handlers
- `Static(relativePath, root)`: Serve static files with COOP/COEP headers for cross-origin isolation
- `Graceful()`: Wait for interrupt signal and gracefully shutdown with 5s timeout

**Platform Implementations** (`internal/daemon/`):
- `daemon.go`: Common interface (`Daemon` and `Executable`) and factory function `New()`
- `daemon_darwin.go`: macOS launchd (UserAgent, GlobalAgent, GlobalDaemon)
- `daemon_linux_systemd.go`: Linux systemd services
- `daemon_linux_systemv.go`: Linux System V init scripts
- `daemon_linux_upstart.go`: Linux Upstart services
- `daemon_freebsd.go`: FreeBSD rc.d services
- `daemon_windows.go`: Windows Service Control Manager with recovery actions

Each platform implementation provides:
- `Install(args...)`: Creates service configuration files and registers with system
- `Remove()`: Unregisters and removes service files
- `Start()`: Starts the service via platform-specific commands
- `Stop()`: Stops the running service
- `Status()`: Returns current service state (running/stopped/not installed)
- `Run(e Executable)`: Executes the service with proper lifecycle management

### Platform-Specific Details

**macOS (launchd)**:
- Service configs written to `~/Library/LaunchAgents/` (UserAgent), `/Library/LaunchAgents/` (GlobalAgent), or `/Library/LaunchDaemons/` (GlobalDaemon)
- Uses `launchctl` commands to load/unload/query services
- Logs default to `/usr/local/var/log/{name}.log` and `.err`

**Linux (systemd)**:
- Service files written to `/etc/systemd/system/{name}.service`
- Uses `systemctl` for daemon-reload, enable, start, stop, status
- Supports dependencies via `Requires=` and `After=` directives
- Auto-restart on failure configured by default

**Windows**:
- Registers via Windows Service Control Manager API (`golang.org/x/sys/windows/svc/mgr`)
- Automatic recovery actions: restart after 5s (first 3 times), then after 1m
- Reset period: 24 hours
- Detects if running as service or interactive console mode

### Service Lifecycle

1. **Creation**: `NewService(name, description, dependencies...)` creates appropriate daemon type based on OS
2. **Installation**: `service.Install(args...)` writes config and registers with system
3. **Management**: Use `Start()`, `Stop()`, `Status()` or `Console()` for CLI-based control
4. **Running**: Implement `Executable` interface with `Start()`, `Stop()`, `Run()` methods
5. **Cleanup**: `Remove()` uninstalls service from system

### HTTP Server Pattern

The `Engine` wrapper provides a batteries-included HTTP server setup:

```go
engine := daemon.NewEngine()
// Configure routes, middleware, etc.
engine.Start(":8080")              // HTTP only
// OR
engine.StartTLS(":443", "example.com")  // HTTPS with autocert + HTTP redirect
engine.Graceful()                  // Block until interrupt signal
```

TLS mode automatically:
- Obtains certificates from Let's Encrypt for specified hosts
- Stores certs in `./certs/` directory next to executable
- Sets up HTTP→HTTPS redirect server
- Implements automatic restart on server errors

## Key Dependencies

- `github.com/gin-gonic/gin`: HTTP web framework
- `github.com/tus/tusd/v2`: Resumable file upload protocol (TUS)
- `golang.org/x/crypto/acme/autocert`: Automatic HTTPS certificates from Let's Encrypt
- `golang.org/x/sys`: Platform-specific system APIs (Windows services, etc.)
- `github.com/swaggo/swag`: Swagger/OpenAPI documentation generation

## Development Notes

- The `internal/daemon` package is forked/adapted from a third-party daemon library (see copyright notices)
- Platform selection happens at compile-time via build tags and `runtime.GOOS` checks
- Service names are automatically sanitized (spaces replaced with underscores)
- Most service operations require elevated privileges (root/admin) except macOS UserAgent
- HTTP server implements automatic restart loops on errors (5s backoff)
- ReadHeaderTimeout set to 3s to mitigate Slowloris attacks
