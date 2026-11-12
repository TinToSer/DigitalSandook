package server

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/TinToSer/DigitalSandook/internal/store"
)

// Cloudflare Tunnel integration.
//
// DigitalSanDook supervises an installed `cloudflared` binary and points it at
// a separate, loopback-only listener that serves ONLY public share links
// (/s/<token>, /api/public/*, static assets). The login page, the API and
// every file operation are unreachable through the tunnel.

const defaultTunnelPort = 8444

var quickURLRe = regexp.MustCompile(`https://[a-z0-9-]+\.trycloudflare\.com`)

type Tunnel struct {
	app      *App
	explicit string // -cloudflared flag

	mu      sync.Mutex
	cancel  context.CancelFunc
	state   string // off | starting | running | error
	msg     string
	url     string
	srv     *http.Server
	srvPort int
	handler http.Handler
}

type TunnelStatus struct {
	Mode      string `json:"mode"`
	State     string `json:"state"`
	Message   string `json:"message"`
	URL       string `json:"url"`
	Binary    string `json:"binary"`
	Port      int    `json:"port"`
	HasToken  bool   `json:"hasToken"`
	PublicURL string `json:"publicUrl"`
}

func (t *Tunnel) findBinary() string {
	name := "cloudflared"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	if t.explicit != "" {
		if _, err := os.Stat(t.explicit); err == nil {
			return t.explicit
		}
		return ""
	}
	var cands []string
	if exe, err := os.Executable(); err == nil {
		cands = append(cands, filepath.Join(filepath.Dir(exe), name))
	}
	cands = append(cands, filepath.Join(t.app.dataDir, name))
	for _, c := range cands {
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			return c
		}
	}
	if p, err := exec.LookPath(name); err == nil {
		return p
	}
	return ""
}

func tunnelPort(s store.Settings) int {
	if s.TunnelPort == 0 {
		return defaultTunnelPort
	}
	return s.TunnelPort
}

func (t *Tunnel) Status() TunnelStatus {
	s := t.app.store.Settings()
	bin := t.findBinary() // file system lookups outside the lock
	t.mu.Lock()
	defer t.mu.Unlock()
	return TunnelStatus{Mode: s.TunnelMode, State: t.state, Message: t.msg, URL: t.url,
		Binary: bin, Port: tunnelPort(s), HasToken: s.TunnelToken != "", PublicURL: s.PublicURL}
}

// PublicBase is the URL prefix used when displaying public share links.
func (t *Tunnel) PublicBase() string {
	if p := t.app.store.Settings().PublicURL; p != "" {
		return p
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.state == "running" {
		return t.url
	}
	return ""
}

func validPublicURL(s string) (string, bool) {
	if s == "" {
		return "", true
	}
	u, err := url.Parse(strings.TrimRight(s, "/"))
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") || u.Path != "" || u.RawQuery != "" || u.User != nil {
		return "", false
	}
	return u.Scheme + "://" + u.Host, true
}

// Apply (re)starts or stops everything according to the saved settings.
func (t *Tunnel) Apply() {
	t.mu.Lock()
	if t.cancel != nil {
		t.cancel()
		t.cancel = nil
	}
	t.url = ""
	s := t.app.store.Settings()
	if s.TunnelMode == "" {
		t.state, t.msg = "off", ""
		t.mu.Unlock()
		t.stopListener()
		return
	}
	t.state, t.msg = "starting", ""
	ctx, cancel := context.WithCancel(context.Background())
	t.cancel = cancel
	t.mu.Unlock()
	go t.run(ctx, s)
}

// set updates the status on behalf of the supervisor started with ctx. A
// supervisor that has been replaced (ctx cancelled) can no longer overwrite
// the status of its successor.
func (t *Tunnel) set(ctx context.Context, state, msg, u string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if ctx.Err() != nil {
		return
	}
	t.state, t.msg = state, msg
	if u != "" || state != "running" {
		t.url = u
	}
}

func (t *Tunnel) startListener(port int) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.srv != nil && t.srvPort == port {
		return nil
	}
	if t.srv != nil {
		t.srv.Close()
		t.srv = nil
	}
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return err
	}
	h := t.handler
	srv := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), tunnelKey, true)))
		}),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       2 * time.Minute,
		MaxHeaderBytes:    32 << 10,
		ErrorLog:          log.New(discardTLSNoise{}, "", 0),
	}
	t.srv, t.srvPort = srv, port
	go srv.Serve(ln)
	return nil
}

func (t *Tunnel) stopListener() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.srv != nil {
		t.srv.Close()
		t.srv = nil
	}
}

func (t *Tunnel) run(ctx context.Context, s store.Settings) {
	port := tunnelPort(s)
	if err := t.startListener(port); err != nil {
		t.set(ctx, "error", fmt.Sprintf("cannot listen on 127.0.0.1:%d: %v", port, err), "")
		return
	}
	bin := t.findBinary()
	if bin == "" {
		t.set(ctx, "error", "cloudflared was not found. Install it, or place the cloudflared executable next to "+AppName+".", "")
		return
	}
	backoff := 3 * time.Second
	for ctx.Err() == nil {
		args := []string{"tunnel", "--no-autoupdate"}
		cmd := exec.CommandContext(ctx, bin)
		if s.TunnelMode == "token" {
			args = append(args, "run")
			cmd.Env = append(os.Environ(), "TUNNEL_TOKEN="+s.TunnelToken) // keep the token out of the process list
		} else {
			args = append(args, "--url", fmt.Sprintf("http://127.0.0.1:%d", port))
		}
		cmd.Args = append([]string{bin}, args...)
		stderr, err := cmd.StderrPipe()
		if err == nil {
			err = cmd.Start()
		}
		if err != nil {
			t.set(ctx, "error", "could not start cloudflared: "+err.Error(), "")
		} else {
			started := time.Now()
			t.set(ctx, "starting", "", "")
			sc := bufio.NewScanner(stderr)
			sc.Buffer(make([]byte, 64<<10), 1<<20)
			for sc.Scan() {
				line := sc.Text()
				if s.TunnelMode == "quick" {
					if u := quickURLRe.FindString(line); u != "" {
						t.set(ctx, "running", "", u)
						log.Printf("Cloudflare quick tunnel: %s", u)
					}
				} else if strings.Contains(line, "Registered tunnel connection") {
					t.set(ctx, "running", "", "")
				}
			}
			werr := cmd.Wait()
			if ctx.Err() != nil {
				return
			}
			t.set(ctx, "error", fmt.Sprintf("cloudflared exited (%v); restarting", werr), "")
			if time.Since(started) > time.Minute {
				backoff = 3 * time.Second
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		backoff = min(backoff*2, time.Minute)
	}
}

func (t *Tunnel) Stop() {
	t.mu.Lock()
	if t.cancel != nil {
		t.cancel()
	}
	t.mu.Unlock()
	t.stopListener()
}

// ---------- admin API ----------

func (a *App) hAdminTunnelGet(w http.ResponseWriter, r *http.Request, c *Ctx) {
	writeJSON(w, 200, a.tunnel.Status())
}

func (a *App) hAdminTunnelSet(w http.ResponseWriter, r *http.Request, c *Ctx) {
	var in struct {
		Mode, Token, PublicURL string
		Port                   int
		KeepToken              bool
	}
	if !readJSON(w, r, &in) {
		return
	}
	if in.Mode != "" && in.Mode != "quick" && in.Mode != "token" {
		jerr(w, 400, "invalid mode")
		return
	}
	pub, valid := validPublicURL(strings.TrimSpace(in.PublicURL))
	if !valid {
		jerr(w, 400, "public URL must look like https://files.example.com")
		return
	}
	if in.Port != 0 && (in.Port < 1024 || in.Port > 65535) {
		jerr(w, 400, "port must be between 1024 and 65535")
		return
	}
	cur := a.store.Settings()
	s := store.Settings{TunnelMode: in.Mode, TunnelToken: cur.TunnelToken, TunnelPort: in.Port, PublicURL: pub}
	if !in.KeepToken {
		s.TunnelToken = strings.TrimSpace(in.Token)
	}
	if s.TunnelMode == "token" && s.TunnelToken == "" {
		jerr(w, 400, "a tunnel token is required for a named tunnel")
		return
	}
	if err := a.store.SetSettings(s); err != nil {
		jerr(w, 500, "could not save")
		return
	}
	a.auditf(c, "tunnel-config", "mode=%q port=%d publicUrl=%q", s.TunnelMode, s.TunnelPort, s.PublicURL)
	a.tunnel.Apply()
	time.Sleep(300 * time.Millisecond) // let a quick start report its first state
	writeJSON(w, 200, a.tunnel.Status())
}
