// omni-inference exposes Omnidex's bounded inference transport without its
// persistent workspace/chat runtime. It has no tools and stores no requests.
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gryph/omnidex/internal/inferenceapi"
	"github.com/gryph/omnidex/internal/ollama"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	listen := os.Getenv("OMNIDEX_INFERENCE_LISTEN")
	if err := validateListener(listen); err != nil {
		return err
	}
	model := os.Getenv("OMNIDEX_INFERENCE_MODEL")
	var client inferenceapi.Provider
	switch os.Getenv("OMNIDEX_INFERENCE_PROVIDER") {
	case "ollama":
		baseURL := os.Getenv("OLLAMA_BASE_URL")
		if baseURL == "" || os.Getenv("OMNIDEX_INFERENCE_BACKEND_URL") != "" || os.Getenv("OMNIDEX_INFERENCE_BACKEND_TOKEN") != "" {
			return fmt.Errorf("Ollama inference requires only OLLAMA_BASE_URL as its provider configuration")
		}
		client = inferenceapi.NewOllamaProvider(ollama.New(baseURL, model, "", 45*time.Second), model)
	case "http":
		if os.Getenv("OLLAMA_BASE_URL") != "" {
			return fmt.Errorf("native HTTP inference does not accept OLLAMA_BASE_URL")
		}
		provider, err := inferenceapi.NewHTTPProvider(os.Getenv("OMNIDEX_INFERENCE_BACKEND_URL"), os.Getenv("OMNIDEX_INFERENCE_BACKEND_TOKEN"), model, 45*time.Second)
		if err != nil {
			return err
		}
		client = provider
	default:
		return fmt.Errorf("OMNIDEX_INFERENCE_PROVIDER must explicitly select ollama or http")
	}
	app, err := inferenceapi.New(client, model, os.Getenv("OMNIDEX_INTEGRATION_API_TOKEN"), 45*time.Second)
	if err != nil {
		return err
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	checkCtx, checkCancel := context.WithTimeout(ctx, 5*time.Second)
	err = client.Check(checkCtx)
	checkCancel()
	if err != nil {
		return fmt.Errorf("configured local inference model is unavailable")
	}
	server := &http.Server{Addr: listen, Handler: app.Handler(), ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 30 * time.Second,
		BaseContext: func(net.Listener) context.Context { return ctx }}
	done := make(chan error, 1)
	go func() { done <- server.ListenAndServe() }()
	log.Printf("stateless inference listening on %s; model=%s", listen, model)
	select {
	case <-ctx.Done():
	case err = <-done:
	}
	shutdown, stop := context.WithTimeout(context.Background(), 5*time.Second)
	defer stop()
	_ = server.Shutdown(shutdown)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}
