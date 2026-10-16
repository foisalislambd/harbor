package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"harbor/internal/config"
	"harbor/internal/httpapi"
	"harbor/internal/store"
)

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "serve":
			if err := serve(os.Args[2:]); err != nil {
				log.Fatal(err)
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
	if err := serve(nil); err != nil {
		log.Fatal(err)
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, `Harbor %s — self-hosted backup

Usage:
  harbor serve [-addr :8787] [-data data] [-panel dist]
  harbor push -file archive.tar.gz -token hb_... [-endpoint http://127.0.0.1:8787]

Environment:
  HARBOR_ADDR, HARBOR_DATA_DIR, HARBOR_PANEL_DIR, HARBOR_MAX_UPLOAD_BYTES
  HARBOR_TOKEN (for push), HARBOR_COOKIE_SECURE=1, HARBOR_CORS_ORIGIN
`, config.Version)
}

func serve(args []string) error {
	cfg := config.FromEnv()
	if err := cfg.ParseFlags(args); err != nil {
		return err
	}
	st, err := store.Open(cfg.DataDir)
	if err != nil {
		return err
	}
	defer st.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := st.GCOrphanBlobs(context.Background()); err != nil {
		log.Printf("blob gc: %v", err)
	}
	if err := st.EnforceAllRetention(context.Background()); err != nil {
		log.Printf("retention: %v", err)
	}
	go func() {
		ticker := time.NewTicker(15 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := st.EnforceAllRetention(context.Background()); err != nil {
					log.Printf("retention: %v", err)
				}
			}
		}
	}()

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           httpapi.New(cfg, st).Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	errCh := make(chan error, 1)
	go func() {
		log.Printf("harbor listening on %s (data %s)", cfg.Addr, cfg.DataDir)
		errCh <- srv.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}
