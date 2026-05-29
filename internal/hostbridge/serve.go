package hostbridge

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

// ServeOptions configures the host bridge HTTP server.
type ServeOptions struct {
	Listen string
	Token  string
}

// RunServe starts the host bridge until interrupted or the server exits.
func RunServe(opts ServeOptions) error {
	addr := strings.TrimSpace(opts.Listen)
	if addr == "" {
		return fmt.Errorf("listen address is required")
	}

	server := &Server{Token: strings.TrimSpace(opts.Token)}
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
		return err
	}
	return nil
}
