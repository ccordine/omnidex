package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gryph/omnidex/internal/hostbridge"
)

func runHostServe(args []string) {
	fs := flag.NewFlagSet("host serve", flag.ExitOnError)
	listen := fs.String("listen", getenv("HOST_AGENT_LISTEN", "127.0.0.1:8091"), "listen address")
	token := fs.String("token", getenv("HOST_AGENT_TOKEN", ""), "optional bearer token")
	_ = fs.Parse(args)

	addr := strings.TrimSpace(*listen)
	if addr == "" {
		die("listen address is required")
	}

	server := &hostbridge.Server{Token: strings.TrimSpace(*token)}
	httpServer := &http.Server{
		Addr:              addr,
		Handler:           server.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(shutdownCtx)
	}()

	log.Printf("omni host bridge listening on http://%s (native directory picker + browse)", addr)
	if strings.HasPrefix(addr, "127.0.0.1") || strings.HasPrefix(addr, "localhost") {
		log.Printf("docker tip: run with --listen 0.0.0.0:8091 and set HOST_AGENT_URL=http://host.docker.internal:8091 in core")
	}
	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		die(err.Error())
	}
}
