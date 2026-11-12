// Package server is the DigitalSanDook HTTP application: routing, security
// middleware, and the handlers for files, uploads, sharing and administration.
package server

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/TinToSer/DigitalSandook/internal/audit"
	"github.com/TinToSer/DigitalSandook/internal/auth"
	"github.com/TinToSer/DigitalSandook/internal/fsjail"
	"github.com/TinToSer/DigitalSandook/internal/store"
	"github.com/TinToSer/DigitalSandook/internal/thumb"
	"github.com/TinToSer/DigitalSandook/internal/tlsutil"
)

const AppName = "DigitalSanDook"

// PIDFile marks a data folder as in use by a running server (see DataDirInUse).
const PIDFile = "server.pid"

// Config is everything the server needs from the command line.
type Config struct {
	Addr        string   // listen address, e.g. ":8443"
	DataDir     string   // users, TLS key, audit log, thumbnail cache
	PlainHTTP   bool     // serve HTTP instead of HTTPS
	CertFile    string   // user certificate (empty = self-signed)
	KeyFile     string   //
	Cloudflared string   // explicit path to the cloudflared executable
	TrustProxy  bool     // trust X-Forwarded-For / X-Forwarded-Proto
	Shares      []string // "-share name=/path" values to register at startup
	Version     string
}

type App struct {
	cfg        Config
	store      *store.Store
	sess       *auth.Sessions
	lim        *auth.Limiter
	ups        *uploads
	aud        *audit.Log
	roots      *fsjail.RootCache
	tunnel     *Tunnel
	site       *staticSite
	dataDir    string
	trustProxy bool
	tlsOn      bool
	hashSem    chan struct{} // bounds concurrent password hashing (CPU)
	zipSem     chan struct{}
	searchSem  chan struct{}
	thumbSem   chan struct{} // bounds concurrent image decoding (CPU/RAM)
	started    time.Time
}

// New opens the data folder and prepares the application without listening.
func New(cfg Config) (*App, error) {
	if err := os.MkdirAll(cfg.DataDir, 0o700); err != nil {
		return nil, fmt.Errorf("cannot create data folder: %w", err)
	}
	dataAbs, err := filepath.Abs(cfg.DataDir)
	if err != nil {
		return nil, err
	}
	st, err := store.Open(dataAbs)
	if err != nil {
		return nil, fmt.Errorf("cannot open state (%v). A backup may exist as %s.bak", err, store.FileName)
	}
	aud, err := audit.Open(dataAbs)
	if err != nil {
		return nil, fmt.Errorf("cannot open audit log: %w", err)
	}
	site, err := newStatic()
	if err != nil {
		return nil, err
	}
	a := &App{
		cfg: cfg, store: st, sess: auth.NewSessions(), lim: auth.NewLimiter(), ups: newUploads(), aud: aud,
		roots: fsjail.NewRootCache(), site: site, dataDir: dataAbs, trustProxy: cfg.TrustProxy, tlsOn: !cfg.PlainHTTP,
		hashSem:   make(chan struct{}, max(2, runtime.NumCPU())),
		thumbSem:  make(chan struct{}, max(2, min(runtime.NumCPU(), 4))),
		zipSem:    make(chan struct{}, 64),
		searchSem: make(chan struct{}, 8),
		started:   time.Now(),
	}
	a.tunnel = &Tunnel{app: a, explicit: cfg.Cloudflared, state: "off"}
	for _, s := range cfg.Shares {
		name, p := parseShare(s)
		if err := a.addMount(name, p, false); err != nil {
			return nil, fmt.Errorf("-share %q: %w", s, err)
		}
		log.Printf("sharing %q as %q", p, name)
	}
	return a, nil
}

// parseShare splits a -share value: "name=/path", or "/path" (name derived).
func parseShare(s string) (name, p string) {
	if i := strings.Index(s, "="); i > 0 && !strings.ContainsAny(s[:i], `/\`) {
		return s[:i], s[i+1:]
	}
	return sanitizeMountName(filepath.Base(s)), s
}

func sanitizeMountName(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteRune('_')
		}
	}
	n := strings.Trim(b.String(), "_")
	if n == "" {
		n = "share"
	}
	if len(n) > 32 {
		n = n[:32]
	}
	return n
}

// EnsureAdmin creates the "admin" account on an empty user database and
// returns its one-time password ("" if users already exist).
func (a *App) EnsureAdmin() (string, error) {
	if len(a.store.Users()) > 0 {
		return "", nil
	}
	pw := auth.RandomPassword()
	u := store.User{Name: "admin", Hash: auth.HashPassword(pw), Admin: true, MustChange: true, Created: time.Now()}
	if err := a.store.CreateUser(u); err != nil {
		return "", err
	}
	return pw, nil
}

// Handler is the complete HTTP handler of the main listener.
func (a *App) Handler() http.Handler { return a.secure(a.routes()) }

// Run serves until ctx is cancelled, then shuts down gracefully.
func (a *App) Run(ctx context.Context) error {
	pw, err := a.EnsureAdmin()
	if err != nil {
		return err
	}
	if pw != "" {
		fmt.Printf("\n  ============================================================\n")
		fmt.Printf("   First run: administrator account created\n")
		fmt.Printf("     username: admin\n     password: %s\n", pw)
		fmt.Printf("   (you will be asked to change it at first login)\n")
		fmt.Printf("  ============================================================\n\n")
	}
	a.cleanTemp()

	srv := &http.Server{
		Addr:              a.cfg.Addr,
		Handler:           a.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       2 * time.Minute,
		MaxHeaderBytes:    64 << 10,
		ErrorLog:          log.New(discardTLSNoise{}, "", 0),
		HTTP2: &http.HTTP2Config{
			// big flow-control windows so uploads are not capped by the 1 MiB
			// default on high-latency links
			MaxConcurrentStreams:          250,
			MaxReadFrameSize:              1 << 20,
			MaxReceiveBufferPerConnection: 64 << 20,
			MaxReceiveBufferPerStream:     16 << 20,
		},
	}
	scheme := "http"
	if !a.cfg.PlainHTTP {
		tc, err := tlsutil.ServerConfig(a.dataDir, a.cfg.CertFile, a.cfg.KeyFile)
		if err != nil {
			return fmt.Errorf("TLS setup failed: %w", err)
		}
		srv.TLSConfig = tc
		scheme = "https"
	}
	ln, err := net.Listen("tcp", a.cfg.Addr)
	if err != nil {
		return fmt.Errorf("cannot listen on %s: %w", a.cfg.Addr, err)
	}

	bgCtx, stopBg := context.WithCancel(ctx)
	defer stopBg()
	go a.background(bgCtx)
	defer os.Remove(filepath.Join(a.dataDir, PIDFile))

	pubMux := http.NewServeMux()
	a.publicRoutes(pubMux)
	a.tunnel.handler = a.secure(pubMux)
	a.tunnel.Apply()
	defer a.tunnel.Stop()

	printURLs(scheme, ln.Addr())
	log.Printf("%s %s ready. data folder: %s", AppName, a.cfg.Version, a.dataDir)

	errc := make(chan error, 1)
	go func() {
		if a.cfg.PlainHTTP {
			errc <- srv.Serve(ln)
		} else {
			errc <- srv.ServeTLS(ln, "", "")
		}
	}()
	select {
	case err := <-errc:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
		log.Println("shutting down...")
		sctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(sctx)
	}
	return a.Close()
}

// Close saves pending state and releases files. The App must not serve
// requests afterwards.
func (a *App) Close() error {
	if err := a.store.Flush(); err != nil {
		log.Printf("saving state: %v", err)
	}
	a.roots.Close()
	return a.aud.Close()
}

// DataDirInUse reports whether a server seems to be running on dir. Offline
// tools (resetpw) must not edit state that the server keeps in memory and
// would overwrite.
func DataDirInUse(dir string) bool {
	st, err := os.Stat(filepath.Join(dir, PIDFile))
	return err == nil && time.Since(st.ModTime()) < 3*time.Minute
}

// discardTLSNoise drops the "TLS handshake error" spam from port scanners and
// untrusted self-signed certificate rejections.
type discardTLSNoise struct{}

func (discardTLSNoise) Write(p []byte) (int, error) {
	if !strings.Contains(string(p), "TLS handshake error") {
		os.Stderr.Write(p)
	}
	return len(p), nil
}

func printURLs(scheme string, addr net.Addr) {
	_, port, _ := net.SplitHostPort(addr.String())
	fmt.Printf("  Open %s at:\n", AppName)
	fmt.Printf("    %s://localhost:%s\n", scheme, port)
	if ifs, err := net.InterfaceAddrs(); err == nil {
		for _, a := range ifs {
			if ipn, ok := a.(*net.IPNet); ok && ipn.IP.To4() != nil && !ipn.IP.IsLoopback() {
				fmt.Printf("    %s://%s:%s\n", scheme, ipn.IP, port)
			}
		}
	}
	if scheme == "https" {
		fmt.Printf("  (self-signed certificate: accept the browser warning once, or pass -cert/-key)\n")
	}
	fmt.Println()
}

// ---------- background upkeep ----------

// heartbeat marks the data folder as in use (see DataDirInUse).
func (a *App) heartbeat() {
	p := filepath.Join(a.dataDir, PIDFile)
	if err := os.WriteFile(p, []byte(fmt.Sprint(os.Getpid())), 0o600); err == nil {
		now := time.Now()
		_ = os.Chtimes(p, now, now)
	}
}

func (a *App) background(ctx context.Context) {
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	a.heartbeat()
	for n := 1; ; n++ {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			a.heartbeat()
			a.sess.GC()
			a.lim.GC()
			a.ups.gc()
			if err := a.store.Flush(); err != nil {
				log.Printf("saving state: %v", err)
			}
			if n%60 == 1 {
				thumb.CleanCache(filepath.Join(a.dataDir, "thumbs"), thumb.CacheMax)
			}
		}
	}
}

// cleanTemp removes leftovers from interrupted uploads on every shared folder.
func (a *App) cleanTemp() {
	for _, m := range a.store.Mounts() {
		if root, err := a.root(m); err == nil {
			_ = root.RemoveAll(fsjail.TmpDir)
		}
	}
}

func hostOS() string { return runtime.GOOS + "/" + runtime.GOARCH }
