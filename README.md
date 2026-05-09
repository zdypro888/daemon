# daemon

跨平台 Go 系统服务库 + 内置 Gin HTTP/HTTPS 服务器。一个二进制程序通过这个库
能装成 macOS launchd / Linux systemd / FreeBSD rc.d / Windows Service 跑起来,
同时提供生产级 HTTPS (autocert)、TUS 大文件断点续传、graceful shutdown 等开箱能力。

## 安装

```bash
go get github.com/zdypro888/daemon
```

## 一、Service 部分 (跨平台服务管理)

```go
package main

import (
    "errors"
    "log"

    "github.com/zdypro888/daemon"
)

func main() {
    service, err := daemon.NewService("my-app", "my application")
    if err != nil {
        log.Fatal(err)
    }
    if err := service.Console(); err != nil {
        if errors.Is(err, daemon.ErrNoCommand) {
            service.Usage()
            return
        }
        log.Fatal(err)
    }
    service.Graceful()
}
```

支持的子命令:

```bash
sudo ./my-app install --args="--port=8080"
sudo ./my-app start
sudo ./my-app status
sudo ./my-app stop
sudo ./my-app remove
```

平台具体细节见 `internal/daemon/`。

## 二、Engine 部分 (HTTP/HTTPS 服务器)

### 默认配置

```go
engine := daemon.NewEngine()
engine.GET("/ping", func(c *gin.Context) { c.String(200, "pong") })
engine.Start(":8080")
engine.Graceful()
```

`NewEngine()` 自带:
- gin Logger + Recovery 中间件
- gzip 压缩 (排除 `.png/.jpg/.ttf/.mp4/.zip` 等已压缩格式, 见 `DefaultGzipExcludedExtensions`)
- HTTP→HTTPS 重定向 (StartTLS 时)

### 自定义 (推荐)

```go
engine := daemon.NewEngineWithOptions(daemon.EngineOptions{
    GinMode:      gin.ReleaseMode,
    AccessLog:    false,
    Recovery:     true,
    EnableGzip:   true,
    WriteTimeout: 5 * time.Minute,  // 慢链路 / 大静态文件传输
    HSTS:         true,              // HTTPS 自动加 Strict-Transport-Security
    // DisableHTTPRedirect: true,    // StartTLS 时关掉 :80 redirect (反代场景)
})
```

| 字段 | 默认 | 说明 |
|---|---|---|
| `ReadHeaderTimeout` | 3s | 慢攻击防护 |
| `ReadTimeout` | 15s | 请求 body 读取上限 — TUS 大文件上传需调大 |
| `WriteTimeout` | 15s | 响应写入上限 — **慢链路 / 大文件下载需调大** |
| `IdleTimeout` | 60s | keep-alive 空闲连接 |
| `ShutdownTimeout` | 5s | Graceful 关停时给 in-flight 请求的等待时间 |
| `DisableHTTPRedirect` | false | 默认开启 StartTLS 时的 :80 → :443 重定向; 设 true 跳过 (反代场景) |
| `CertsDir` | 可执行文件旁 `./certs` | autocert 缓存路径,容器化场景常需指定 |
| `HSTS` | false | HTTPS 响应自动加 `Strict-Transport-Security` |
| `HSTSMaxAge` | 15552000 (180 天) | HSTS max-age 秒数 |
| `GzipExcludedExtensions` | `DefaultGzipExcludedExtensions` | 不压缩的扩展名 |

### HTTPS (Let's Encrypt 自动证书)

```go
if err := engine.StartTLS(":443", "example.com", "www.example.com"); err != nil {
    log.Fatal(err)
}
engine.Graceful()
```

证书走 ACME (Let's Encrypt),首次访问自动签发并缓存到 `CertsDir`。

### TLS (自带证书)

```go
cfg := &tls.Config{Certificates: []tls.Certificate{cert}}
engine.StartTLSWithConfig(":443", cfg)
```

### TUS 大文件断点续传

```go
composer := engine.TUSFileComposer("/var/uploads")
if err := engine.TUSHandle("/files/", composer); err != nil {
    log.Fatal(err)
}

// 订阅 "上传完成" 事件
go func() {
    for ev := range engine.TUSCompleteUploads() {
        log.Printf("upload finished: %s (%d bytes)", ev.Upload.ID, ev.Upload.Size)
    }
}()
```

### Cross-Origin Isolation

需要 SharedArrayBuffer / WebAssembly Threads 时, 静态目录加 COOP/COEP 头:

```go
engine.IsolatedStatic("/wasm", "./public/wasm")
// 自动响应:
//   Cross-Origin-Embedder-Policy: require-corp
//   Cross-Origin-Opener-Policy: same-origin
```

(原 `engine.Static` 名字误导且实现有 bug, 已重命名为 `IsolatedStatic`,旧名作为 deprecated 别名保留。)

### 全局超时变量

包级 `var ReadTimeout / WriteTimeout / ...` 仅作为 `EngineOptions` 没指定时的 fallback 默认值。**优先用 `EngineOptions` 的字段, 不要修改全局 var** (会影响进程内所有 daemon.Engine 实例)。

## 三、Graceful 关停

```go
engine.Graceful()  // 阻塞直到 SIGINT/SIGTERM, 然后 graceful shutdown
// 或主动触发:
engine.Shutdown()
```

`ShutdownTimeout` (默认 5s) 内未完成的请求会被强切。

## 已知行为

- **autocert 路径**: 默认创建 `./certs` (相对可执行文件), 需要写权限。容器只读 fs 时通过 `EngineOptions.CertsDir` 指定可写目录。
- **listen 重试**: 端口被占 / 配置错时,内部按指数退避重试最多 10 次后放弃, 不再无限刷屏。
- **gin.DefaultWriter 进程全局**: `NewEngineWithOptions` 会设 `gin.DefaultWriter / DefaultErrorWriter` — 同进程多个 Engine 共享。

## 依赖

- `github.com/gin-gonic/gin` — HTTP framework
- `github.com/gin-contrib/gzip` — 响应压缩
- `github.com/tus/tusd/v2` — TUS resumable upload
- `golang.org/x/crypto/acme/autocert` — Let's Encrypt 自动证书
- `github.com/zdypro888/crash` — panic / log 重定向

## 开发

```bash
go build ./...
go test ./...

# 跑示例 service
cd examples/service && go build .

# 跑示例 server (Swagger)
cd examples/server && swag init && go run .
```
