package daemon

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gin-contrib/gzip"
	"github.com/gin-gonic/gin"
	"github.com/tus/tusd/v2/pkg/filelocker"
	"github.com/tus/tusd/v2/pkg/filestore"
	tusd "github.com/tus/tusd/v2/pkg/handler"
	"golang.org/x/crypto/acme/autocert"
)

// 默认超时。改成 var (历史是 const) 让外部能在 NewEngine 之前覆盖, 避免重新发版。
// 主要场景:
//   - WriteTimeout 默认 15s 在窄带链路上传输大静态文件 (3MB+ JS bundle, 视频 chunk 等)
//     会被切断 → 客户端报 "Network connection was lost"。
//   - ReadTimeout 默认 15s 让 TUS 大文件上传超时 (本库自己集成了 TUS, 自相矛盾)。
// 推荐改 EngineOptions 里的字段 (NewEngineWithOptions); 全局 var 仅作为 fallback。
var (
	ReadHeaderTimeout = 3 * time.Second
	ReadTimeout       = 15 * time.Second
	WriteTimeout      = 15 * time.Second
	IdleTimeout       = 60 * time.Second
)

// 默认排除的不可压缩扩展名 — 已经压缩过的二进制再 gzip 不仅没收益, 还可能因 deflate
// 头开销让响应变大。可以通过 EngineOptions.GzipExcludedExtensions 覆盖。
var DefaultGzipExcludedExtensions = []string{
	".png", ".jpg", ".jpeg", ".gif", ".webp", ".ico", ".svgz",
	".ttf", ".woff", ".woff2", ".otf",
	".mp3", ".mp4", ".webm", ".ogg", ".m4a",
	".zip", ".gz", ".br", ".bz2", ".7z", ".rar",
	".pdf",
}

type Engine struct {
	*gin.Engine
	httpsrv  *http.Server
	httpsrvs *http.Server

	TUSFileStore filestore.FileStore
	TUSHandler   *tusd.Handler

	opts        EngineOptions
	autocertMgr *autocert.Manager
	closeOnce   sync.Once
}

// EngineOptions controls gin mode, middleware defaults, and HTTP server timeouts.
//
// 字段为零值时回退到包级默认 (DefaultGzipExcludedExtensions / ReadTimeout / 等)。
type EngineOptions struct {
	GinMode      string
	AccessLog    bool
	Recovery     bool
	AccessWriter io.Writer
	ErrorWriter  io.Writer
	EnableGzip   bool

	// gzip 排除的扩展名 (覆盖 DefaultGzipExcludedExtensions)。仅 EnableGzip=true 生效。
	GzipExcludedExtensions []string

	// HTTP 服务器超时。零值回退到 ReadHeaderTimeout / ReadTimeout / WriteTimeout / IdleTimeout 包级默认。
	ReadHeaderTimeout time.Duration
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration

	// Graceful() 关停时给 in-flight 请求的最大等待时间, 默认 5s。
	ShutdownTimeout time.Duration

	// StartTLS / StartTLSWithConfig 默认会同时开启 :http (80 端口) 做 HTTP→HTTPS 301 重定向。
	// 这里设 true 可以禁用 — 适合反代场景 (前端有 nginx) 或者只想要 HTTPS。
	// 历史行为是无条件开 redirect, 用零值 (false) 维持兼容。
	DisableHTTPRedirect bool

	// TLS 证书 / autocert 缓存目录。空 = 可执行文件同级 ./certs。
	CertsDir string

	// 是否给 HTTPS 响应自动加 HSTS 头 (Strict-Transport-Security)。默认 false。
	HSTS bool
	// HSTS max-age, 默认 180 天 (15552000s)。仅 HSTS=true 时生效。
	HSTSMaxAge int
}

func (opts *EngineOptions) effectiveReadHeaderTimeout() time.Duration {
	if opts.ReadHeaderTimeout > 0 {
		return opts.ReadHeaderTimeout
	}
	return ReadHeaderTimeout
}
func (opts *EngineOptions) effectiveReadTimeout() time.Duration {
	if opts.ReadTimeout > 0 {
		return opts.ReadTimeout
	}
	return ReadTimeout
}
func (opts *EngineOptions) effectiveWriteTimeout() time.Duration {
	if opts.WriteTimeout > 0 {
		return opts.WriteTimeout
	}
	return WriteTimeout
}
func (opts *EngineOptions) effectiveIdleTimeout() time.Duration {
	if opts.IdleTimeout > 0 {
		return opts.IdleTimeout
	}
	return IdleTimeout
}
func (opts *EngineOptions) effectiveShutdownTimeout() time.Duration {
	if opts.ShutdownTimeout > 0 {
		return opts.ShutdownTimeout
	}
	return 5 * time.Second
}
func (opts *EngineOptions) effectiveGzipExcluded() []string {
	if opts.GzipExcludedExtensions != nil {
		return opts.GzipExcludedExtensions
	}
	return DefaultGzipExcludedExtensions
}

func NewEngine() *Engine {
	return NewEngineWithOptions(EngineOptions{
		GinMode:    gin.ReleaseMode,
		AccessLog:  true,
		Recovery:   true,
		EnableGzip: true,
		// HTTP→HTTPS redirect 默认开 (DisableHTTPRedirect 零值 false)
	})
}

func NewEngineWithOptions(opts EngineOptions) *Engine {
	if opts.GinMode == "" {
		opts.GinMode = gin.ReleaseMode
	}
	gin.SetMode(opts.GinMode)

	if opts.AccessWriter != nil {
		gin.DefaultWriter = opts.AccessWriter
	} else {
		gin.DefaultWriter = os.Stdout
	}
	if opts.ErrorWriter != nil {
		gin.DefaultErrorWriter = opts.ErrorWriter
	} else {
		gin.DefaultErrorWriter = os.Stderr
	}

	router := gin.New()
	if opts.AccessLog {
		router.Use(gin.Logger())
	}
	if opts.Recovery {
		router.Use(gin.Recovery())
	}
	if opts.EnableGzip {
		router.Use(gzip.Gzip(gzip.DefaultCompression, gzip.WithExcludedExtensions(opts.effectiveGzipExcluded())))
	}
	if opts.HSTS {
		maxAge := opts.HSTSMaxAge
		if maxAge <= 0 {
			maxAge = 15552000 // 180 天
		}
		hsts := "max-age=" + strconv.Itoa(maxAge) + "; includeSubDomains"
		router.Use(func(ctx *gin.Context) {
			// 仅给 HTTPS 请求加 HSTS, HTTP→HTTPS redirect 路径不需要
			if ctx.Request.TLS != nil {
				ctx.Header("Strict-Transport-Security", hsts)
			}
			ctx.Next()
		})
	}

	return &Engine{
		Engine:   router,
		httpsrv:  nil,
		httpsrvs: nil,
		opts:     opts,
	}
}

func (engine *Engine) Start(addr string) {
	if addr == "" {
		addr = ":http"
	}
	engine.httpsrv = &http.Server{
		Addr:              addr,
		Handler:           engine.Engine,
		ReadHeaderTimeout: engine.opts.effectiveReadHeaderTimeout(),
		ReadTimeout:       engine.opts.effectiveReadTimeout(),
		WriteTimeout:      engine.opts.effectiveWriteTimeout(),
		IdleTimeout:       engine.opts.effectiveIdleTimeout(),
	}
	go engine.listenLoop(engine.httpsrv, false)
}

func (engine *Engine) StartTLS(addr string, hosts ...string) error {
	if len(hosts) == 0 {
		return errors.New("at least one host must be specified for TLS autocert")
	}

	certPath := engine.opts.CertsDir
	if certPath == "" {
		ex, err := os.Executable()
		if err != nil {
			return err
		}
		certPath = path.Join(filepath.Dir(ex), "certs")
	}
	if _, err := os.Stat(certPath); os.IsNotExist(err) {
		if err := os.MkdirAll(certPath, 0700); err != nil {
			return err
		}
	}
	if addr == "" {
		addr = ":https"
	}

	engine.autocertMgr = &autocert.Manager{
		Prompt:     autocert.AcceptTOS,
		HostPolicy: autocert.HostWhitelist(hosts...),
		Cache:      autocert.DirCache(certPath),
	}

	if !engine.opts.DisableHTTPRedirect {
		engine.httpsrv = &http.Server{
			Addr: ":http",
			Handler: engine.autocertMgr.HTTPHandler(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				target := "https://" + req.Host
				if addr != ":443" && addr != ":https" { // 非标准 HTTPS 端口
					_, port, err := net.SplitHostPort(addr)
					if err == nil && port != "" {
						target = "https://" + req.Host + ":" + port
					}
				}
				target += req.RequestURI
				http.Redirect(w, req, target, http.StatusMovedPermanently)
			})),
			ReadHeaderTimeout: engine.opts.effectiveReadHeaderTimeout(),
			ReadTimeout:       engine.opts.effectiveReadTimeout(),
			WriteTimeout:      engine.opts.effectiveWriteTimeout(),
			IdleTimeout:       engine.opts.effectiveIdleTimeout(),
		}
		go engine.listenLoop(engine.httpsrv, false)
	}

	engine.httpsrvs = &http.Server{
		Addr:              addr,
		TLSConfig:         engine.autocertMgr.TLSConfig(),
		Handler:           engine.Engine,
		ReadHeaderTimeout: engine.opts.effectiveReadHeaderTimeout(),
		ReadTimeout:       engine.opts.effectiveReadTimeout(),
		WriteTimeout:      engine.opts.effectiveWriteTimeout(),
		IdleTimeout:       engine.opts.effectiveIdleTimeout(),
	}
	go engine.listenLoop(engine.httpsrvs, true)
	return nil
}

func (engine *Engine) StartTLSWithConfig(addr string, config *tls.Config) error {
	if addr == "" {
		addr = ":https"
	}

	// 跟 StartTLS 行为对齐: 默认同步起 (DisableHTTPRedirect=true 时跳过) :http 做 301 重定向。
	if !engine.opts.DisableHTTPRedirect {
		engine.httpsrv = &http.Server{
			Addr: ":http",
			Handler: http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				target := "https://" + req.Host
				if addr != ":443" && addr != ":https" {
					_, port, err := net.SplitHostPort(addr)
					if err == nil && port != "" {
						target = "https://" + req.Host + ":" + port
					}
				}
				target += req.RequestURI
				http.Redirect(w, req, target, http.StatusMovedPermanently)
			}),
			ReadHeaderTimeout: engine.opts.effectiveReadHeaderTimeout(),
			ReadTimeout:       engine.opts.effectiveReadTimeout(),
			WriteTimeout:      engine.opts.effectiveWriteTimeout(),
			IdleTimeout:       engine.opts.effectiveIdleTimeout(),
		}
		go engine.listenLoop(engine.httpsrv, false)
	}

	engine.httpsrvs = &http.Server{
		Addr:              addr,
		TLSConfig:         config,
		Handler:           engine.Engine,
		ReadHeaderTimeout: engine.opts.effectiveReadHeaderTimeout(),
		ReadTimeout:       engine.opts.effectiveReadTimeout(),
		WriteTimeout:      engine.opts.effectiveWriteTimeout(),
		IdleTimeout:       engine.opts.effectiveIdleTimeout(),
	}
	go engine.listenLoop(engine.httpsrvs, true)
	return nil
}

// listenLoop 跑 ListenAndServe (TLS), 失败时退避重试。
//
// 之前 listenGo / listenTLSGo 用固定 5s sleep 死循环重试, 端口被占 / 配置错时
// 日志会无限刷屏。这里改用指数退避 + 最大尝试次数, Graceful 关闭时直接退出。
func (engine *Engine) listenLoop(srv *http.Server, tlsMode bool) {
	const maxRetries = 10
	const baseDelay = 1 * time.Second
	const maxDelay = 60 * time.Second

	attempts := 0
	for {
		var err error
		if tlsMode {
			err = srv.ListenAndServeTLS("", "")
		} else {
			err = srv.ListenAndServe()
		}
		if err == nil || errors.Is(err, http.ErrServerClosed) {
			log.Printf("[daemon] %s closed", srv.Addr)
			return
		}
		attempts++
		if attempts >= maxRetries {
			log.Printf("[daemon] %s gave up after %d retries: %v", srv.Addr, attempts, err)
			return
		}
		// 指数退避 + jitter, 避免日志疯狂刷
		delay := baseDelay << (attempts - 1)
		if delay > maxDelay {
			delay = maxDelay
		}
		log.Printf("[daemon] %s error (attempt %d/%d): %v, retry in %v", srv.Addr, attempts, maxRetries, err, delay)
		time.Sleep(delay)
	}
}

func (engine *Engine) Graceful() {
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(interrupt)
	defer close(interrupt)
	<-interrupt
	engine.Shutdown()
}

// Shutdown 主动触发 graceful shutdown (不等信号), 可在外部上下文已经收到关闭意图时调用。
func (engine *Engine) Shutdown() {
	engine.closeOnce.Do(func() {
		timeout := engine.opts.effectiveShutdownTimeout()
		shutdown := func(srv *http.Server, label string) {
			if srv == nil {
				return
			}
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()
			if err := srv.Shutdown(ctx); err != nil {
				log.Printf("[daemon] %s forced to shutdown: %v", label, err)
			}
		}
		shutdown(engine.httpsrv, "http")
		shutdown(engine.httpsrvs, "https")
	})
}

func (engine *Engine) TUSFileComposer(p string) *tusd.StoreComposer {
	composer := tusd.NewStoreComposer()
	locker := filelocker.New(p)
	locker.UseIn(composer)
	engine.TUSFileStore = filestore.New(p)
	engine.TUSFileStore.UseIn(composer)
	return composer
}

func (engine *Engine) TUSHandle(basePath string, composer *tusd.StoreComposer) error {
	if !strings.HasSuffix(basePath, "/") {
		basePath += "/"
	}
	var err error
	if engine.TUSHandler, err = tusd.NewHandler(tusd.Config{
		BasePath:              basePath,
		StoreComposer:         composer,
		NotifyCompleteUploads: true,
	}); err != nil {
		return err
	}
	engine.Engine.Any(basePath, gin.WrapH(http.StripPrefix(basePath, engine.TUSHandler)))
	if basePath != "/" {
		basePathTrimed := strings.TrimSuffix(basePath, "/")
		engine.Engine.Any(basePathTrimed, gin.WrapH(http.StripPrefix(basePathTrimed, engine.TUSHandler)))
	}
	return nil
}

// TUSCompleteUploads 暴露 TUSHandler.CompleteUploads channel, 业务层订阅 "上传完成" 事件。
// TUSHandle 之前调用返回 nil。
func (engine *Engine) TUSCompleteUploads() <-chan tusd.HookEvent {
	if engine.TUSHandler == nil {
		return nil
	}
	return engine.TUSHandler.CompleteUploads
}

// IsolatedStatic 给静态目录加 Cross-Origin-Opener-Policy / Cross-Origin-Embedder-Policy 头,
// 用于需要 cross-origin isolation 的场景 (例如 SharedArrayBuffer / WebAssembly Threads)。
//
// 修复了原 Static 方法的 bug: 之前先 router.Static() 再 router.Use(crossHandle),
// gin 的 IRoutes.Use 只对**之后注册**的 handler 生效, 已经注册的 static handler
// 不会被 wrap, COOP/COEP 头实际没设。
//
// 现在用 Group(prefix, middleware) 先把 middleware 挂在 group 上,
// 再 group.Static — handler chain 正确。
func (engine *Engine) IsolatedStatic(relativePath string, root string) {
	crossHandle := func(ctx *gin.Context) {
		ctx.Header("Cross-Origin-Embedder-Policy", "require-corp")
		ctx.Header("Cross-Origin-Opener-Policy", "same-origin")
		ctx.Next()
	}
	group := engine.Engine.Group(relativePath, crossHandle)
	group.Static("", root)
}

// Static 保留为兼容别名 (老调用方还在用), 内部走 IsolatedStatic 修过 bug 的版本。
//
// Deprecated: 使用 IsolatedStatic 让命名跟 gin 自带的 *gin.Engine.Static 区分开。
func (engine *Engine) Static(relativePath string, root string) {
	engine.IsolatedStatic(relativePath, root)
}
