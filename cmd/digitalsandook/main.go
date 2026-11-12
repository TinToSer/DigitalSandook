// Command digitalsandook is a single-file, zero-dependency, multi-user remote
// file manager. See README.md for usage.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"

	"github.com/TinToSer/DigitalSandook/internal/auth"
	"github.com/TinToSer/DigitalSandook/internal/server"
	"github.com/TinToSer/DigitalSandook/internal/store"
)

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

type listFlag []string

func (l *listFlag) String() string     { return strings.Join(*l, ",") }
func (l *listFlag) Set(v string) error { *l = append(*l, v); return nil }

func defaultDataDir() string {
	if exe, err := os.Executable(); err == nil {
		return filepath.Join(filepath.Dir(exe), "dsd-data")
	}
	return "dsd-data"
}

const usage = `%s %s - single-binary, secure, high-speed file server

Usage:
  DigitalSanDook [flags]                              run the server
  DigitalSanDook resetpw [-data dir] [-force] <user>  set a new random password for a user
  DigitalSanDook version

Flags:
`

func main() {
	if len(os.Args) > 1 && !strings.HasPrefix(os.Args[1], "-") {
		switch os.Args[1] {
		case "version":
			fmt.Printf("%s %s (%s/%s)\n", server.AppName, version, runtime.GOOS, runtime.GOARCH)
		case "resetpw":
			resetPW(os.Args[2:])
		default:
			fmt.Fprintf(os.Stderr, usage, server.AppName, version)
			flag.PrintDefaults()
			os.Exit(2)
		}
		return
	}

	var cfg server.Config
	var shares listFlag
	flag.StringVar(&cfg.Addr, "addr", ":8443", "listen address (host:port)")
	flag.StringVar(&cfg.DataDir, "data", defaultDataDir(), "data folder (users, TLS cert, audit log)")
	flag.BoolVar(&cfg.PlainHTTP, "http", false, "serve plain HTTP (only behind a TLS-terminating reverse proxy or on localhost)")
	flag.StringVar(&cfg.CertFile, "cert", "", "TLS certificate file (default: auto-generated self-signed)")
	flag.StringVar(&cfg.KeyFile, "key", "", "TLS private key file")
	flag.StringVar(&cfg.Cloudflared, "cloudflared", "", "path to the cloudflared executable (default: next to this program, in the data folder, or on PATH)")
	flag.BoolVar(&cfg.TrustProxy, "trust-proxy", false, "trust X-Forwarded-For/Proto from a reverse proxy")
	flag.Var(&shares, "share", "share a local folder: -share name=/path or -share /path (repeatable)")
	flag.Usage = func() { fmt.Fprintf(os.Stderr, usage, server.AppName, version); flag.PrintDefaults() }
	flag.Parse()
	cfg.Shares = shares
	cfg.Version = version

	app, err := server.New(cfg)
	if err != nil {
		log.Fatal(err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := app.Run(ctx); err != nil {
		log.Fatal(err)
	}
}

func resetPW(args []string) {
	fs := flag.NewFlagSet("resetpw", flag.ExitOnError)
	data := fs.String("data", defaultDataDir(), "data folder")
	force := fs.Bool("force", false, "run even if the server seems to be running")
	fs.Parse(args)
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: DigitalSanDook resetpw [-data dir] [-force] <user>")
		os.Exit(2)
	}
	if _, err := os.Stat(filepath.Join(*data, store.FileName)); err != nil {
		log.Fatalf("no %s state found in %s (use -data)", server.AppName, *data)
	}
	// the running server keeps state in memory and would overwrite our change
	if server.DataDirInUse(*data) && !*force {
		log.Fatalf("%s is running with this data folder - stop it first, then run resetpw again", server.AppName)
	}
	st, err := store.Open(*data)
	if err != nil {
		log.Fatal(err)
	}
	name := strings.ToLower(fs.Arg(0))
	pw := auth.RandomPassword()
	hash := auth.HashPassword(pw)
	if _, err := st.UpdateUser(name, func(u *store.User) error {
		u.Hash, u.MustChange, u.Disabled = hash, true, false
		return nil
	}); err != nil {
		log.Fatalf("%s: %v", name, err)
	}
	fmt.Printf("New password for %s: %s\n(it must be changed at next login)\n", name, pw)
}
