package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"harbor/internal/config"
	"harbor/internal/desktop"
	"harbor/internal/httpapi"
	"harbor/internal/panel"
	"harbor/internal/replicate"
	"harbor/internal/store"

	"golang.org/x/crypto/bcrypt"
)

func main() {
	log.SetOutput(os.Stderr)
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "serve":
			if err := serve(os.Args[2:]); err != nil {
				fail(err)
			}
			return
		case "desktop":
			if err := desktopApp(os.Args[2:]); err != nil {
				fail(err)
			}
			return
		case "push":
			if err := push(os.Args[2:]); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
			return
		case "help", "-h", "--help":
			usage()
			return
		default:
			usage()
			os.Exit(2)
		}
	}
	if err := desktopApp(nil); err != nil {
		fail(err)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	if !interactiveConsole() {
		desktop.Alert("Harbor", err.Error())
	}
	os.Exit(1)
}

func usage() {
	fmt.Fprintf(os.Stderr, `Harbor %s — self-hosted backup

Usage:
  harbor                         open the desktop app
  harbor desktop [-addr 127.0.0.1:8787] [-data <dir>] [-panel dist]
  harbor serve [-addr :8787] [-data data] [-panel dist]
  harbor push -file archive.tar.gz -token hb_... [-endpoint http://127.0.0.1:8787]

The desktop app stores data in the user data directory and listens on 127.0.0.1.
harbor serve is the headless server used by Docker and a VPS.

Environment:
  HARBOR_ADDR, HARBOR_DATA_DIR, HARBOR_PANEL_DIR, HARBOR_MAX_UPLOAD_BYTES
  HARBOR_TOKEN (for push), HARBOR_COOKIE_SECURE=1, HARBOR_CORS_ORIGIN
`, config.Version)
}

type running struct {
	srv    *http.Server
	store  *store.Store
	cancel context.CancelFunc
}

func (r *running) shutdown() {
	r.cancel()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = r.srv.Shutdown(ctx)
	r.store.Close()
}

func boot(cfg config.Config) (*running, error) {
	st, err := store.Open(cfg.DataDir)
	if err != nil {
		return nil, err
	}
	if err := seedOwner(st); err != nil {
		st.Close()
		return nil, err
	}
	ln, err := net.Listen("tcp", cfg.Addr)
	if err != nil {
		st.Close()
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	if err := st.GCOrphanBlobs(context.Background()); err != nil {
		log.Printf("blob gc: %v", err)
	}
	runRetention(st)
	go func() {
		ticker := time.NewTicker(15 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				runRetention(st)
			}
		}
	}()
	srv := &http.Server{
		Handler:           httpapi.New(cfg, st).Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	go func() {
		log.Printf("harbor listening on %s (data %s)", cfg.Addr, cfg.DataDir)
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("server: %v", err)
		}
	}()
	return &running{srv: srv, store: st, cancel: cancel}, nil
}

func runRetention(st *store.Store) {
	if err := st.EnforceAllRetention(context.Background()); err != nil {
		log.Printf("retention: %v", err)
	}
	replicate.FlushDeletes(context.Background(), st)
}

const (
	seedEmail    = "admin@harbor.local"
	seedPassword = "admin@123456"
)

func seedOwner(st *store.Store) error {
	n, err := st.UserCount(context.Background())
	if err != nil || n > 0 {
		return err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(seedPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	_, err = st.CreateOwner(context.Background(), seedEmail, "Admin", string(hash))
	if err != nil {
		return err
	}
	log.Printf("seeded owner %s", seedEmail)
	return nil
}

func serve(args []string) error {
	cfg := config.FromEnv()
	if err := cfg.ParseFlags(args); err != nil {
		return err
	}
	app, err := boot(cfg)
	if err != nil {
		return err
	}
	defer app.shutdown()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()
	return nil
}

func desktopApp(args []string) error {
	cfg := config.FromEnv()
	if os.Getenv("HARBOR_ADDR") == "" {
		cfg.Addr = "127.0.0.1:8787"
	}
	if os.Getenv("HARBOR_DATA_DIR") == "" {
		dir, err := desktop.DataDir()
		if err != nil {
			return err
		}
		cfg.DataDir = dir
	}
	if err := cfg.ParseFlags(args); err != nil {
		return err
	}
	if !panel.Usable(cfg.PanelDir) {
		return errors.New("this build has no panel; build apps/panel and copy dist to apps/server/internal/panel/dist, or pass -panel")
	}
	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		return err
	}
	if logFile, err := os.OpenFile(filepath.Join(cfg.DataDir, "harbor.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644); err == nil {
		defer logFile.Close()
		log.SetOutput(io.MultiWriter(os.Stderr, logFile))
	}

	page := desktop.BrowserURL(cfg.Addr)
	profile := filepath.Join(cfg.DataDir, "browser-profile")
	if desktop.Healthy(page) {
		log.Printf("harbor already running at %s", page)
		return desktop.Launch(page, profile, false)
	}
	app, err := boot(cfg)
	if err != nil {
		if desktop.Healthy(page) {
			return desktop.Launch(page, profile, false)
		}
		return err
	}
	defer app.shutdown()
	if err := desktop.WaitReady(page); err != nil {
		return err
	}
	log.Printf("desktop window at %s", page)
	if err := desktop.Launch(page, profile, true); err != nil {
		fmt.Fprintf(os.Stderr, "Could not open a window: %v\n", err)
		return desktop.StayOpen(page)
	}
	return nil
}
