package daemon

import (
	"context"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-contrib/gzip"
	"github.com/gin-gonic/gin"
	"github.com/tus/tusd/v2/pkg/filelocker"
	"github.com/tus/tusd/v2/pkg/filestore"
	tusd "github.com/tus/tusd/v2/pkg/handler"
	"golang.org/x/crypto/acme/autocert"
)

const (
	ReadHeaderTimeout = 3 * time.Second
)

type Engine struct {
	*gin.Engine
	httpsrv  *http.Server
	httpsrvs *http.Server

	TUSFileStore filestore.FileStore
	TUSHandler   *tusd.Handler
}

func NewEngine() *Engine {
	gin.DefaultWriter = os.Stdout      // Gin 的 INFO 级日志输出到 stdout
	gin.DefaultErrorWriter = os.Stderr // Gin 的 ERROR 级日志输出到 stderr
	gin.SetMode(gin.ReleaseMode)

	router := gin.Default()
	router.Use(gin.Logger())
	router.Use(gin.Recovery())
	config := cors.DefaultConfig()
	config.AllowAllOrigins = true
	router.Use(cors.New(config))
	router.Use(gzip.Gzip(gzip.DefaultCompression))
	// router.Use(brotli.Brotli(brotli.DefaultCompression))

	return &Engine{
		Engine:   router,
		httpsrv:  nil,
		httpsrvs: nil,
	}
}

func (engine *Engine) Start(addr string) {
	if addr == "" {
		addr = ":http"
	}
	engine.httpsrv = &http.Server{Addr: addr, Handler: engine.Engine, ReadHeaderTimeout: ReadHeaderTimeout}
	go func() {
		for {
			if err := engine.httpsrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.Printf("Server error: %v, attempting to restart...", err)
				time.Sleep(5 * time.Second) // 等待一段时间再尝试重启
			} else {
				log.Printf("Server closed")
				break
			}
		}
	}()
}

func (engine *Engine) StartTLS(addr string, hosts ...string) error {
	ex, err := os.Executable()
	if err != nil {
		return err
	}
	folder := filepath.Dir(ex)
	certPath := path.Join(folder, "certs")
	if _, err := os.Stat(certPath); os.IsNotExist(err) {
		if err := os.Mkdir(certPath, 0700); err != nil {
			return err
		}
	}
	if addr == "" {
		addr = ":https"
	}

	m := autocert.Manager{
		Prompt:     autocert.AcceptTOS,
		HostPolicy: autocert.HostWhitelist(hosts...),
		Cache:      autocert.DirCache(certPath),
	}
	// http3Server := &http3.Server{Handler: s.Router, Addr: addr, TLSConfig: m.TLSConfig()}
	// go http3Server.ListenAndServe()
	engine.httpsrv = &http.Server{
		Addr: ":http",
		Handler: m.HTTPHandler(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			target := "https://" + req.Host
			if addr != ":443" && addr != ":https" { // 检查是否是非标准 HTTPS 端口
				_, port, err := net.SplitHostPort(addr)
				if err == nil && port != "" {
					target = "https://" + req.Host + ":" + port
				}
			}
			target += req.RequestURI
			http.Redirect(w, req, target, http.StatusMovedPermanently)
		})),
		ReadHeaderTimeout: ReadHeaderTimeout,
	}
	engine.httpsrvs = &http.Server{
		Addr:              addr,
		TLSConfig:         m.TLSConfig(),
		Handler:           engine.Engine,
		ReadHeaderTimeout: ReadHeaderTimeout,
	}
	go engine.listenGo()
	go engine.listenTLSGo()
	return nil
}

func (engine *Engine) listenGo() {
	for {
		if err := engine.httpsrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("Server error: %v, attempting to restart...", err)
			time.Sleep(5 * time.Second) // 等待一段时间再尝试重启
		} else {
			log.Printf("Server closed")
			break
		}
	}
}

func (engine *Engine) listenTLSGo() {
	for {
		if err := engine.httpsrvs.ListenAndServeTLS("", ""); err != nil && err != http.ErrServerClosed {
			log.Printf("Server(s) error: %v, attempting to restart...", err)
			time.Sleep(5 * time.Second) // 等待一段时间再尝试重启
		} else {
			log.Printf("Server(s) closed")
			break
		}
	}
}

func (engine *Engine) Graceful() {
	if engine.httpsrv == nil {
		log.Printf("Server not started")
		return
	}
	interrupt := make(chan os.Signal, 1)
	defer close(interrupt)
	signal.Notify(interrupt, os.Interrupt, syscall.SIGTERM)
	<-interrupt

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := engine.httpsrv.Shutdown(ctx); err != nil {
		log.Printf("Server forced to shutdown: %v", err)
	}
	if engine.httpsrvs != nil {
		if err := engine.httpsrvs.Shutdown(ctx); err != nil {
			log.Printf("Server(s) forced to shutdown: %v", err)
		}
	}
}

func (engine *Engine) TUSFileComposer(path string) *tusd.StoreComposer {
	composer := tusd.NewStoreComposer()
	locker := filelocker.New(path)
	locker.UseIn(composer)
	engine.TUSFileStore = filestore.New(path)
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
	return err
}

// Static add Cross-Origin-Opener-Policy: same-origin and Cross-Origin-Embedder-Policy: require-corp to all routers
func (engine *Engine) Static(relativePath string, root string) {
	crossHandle := func(ctx *gin.Context) {
		ctx.Header("Cross-Origin-Embedder-Policy", "require-corp")
		ctx.Header("Cross-Origin-Opener-Policy", "same-origin")
		ctx.Next()
	}
	router := engine.Engine.Static(relativePath, root)
	router.Use(crossHandle)
}
