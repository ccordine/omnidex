package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/gryph/omnidex/internal/config"
	omnidexruntime "github.com/gryph/omnidex/internal/runtime"
)

func main() {
	if err := run(); err != nil {
		log.Printf("omnidex stopped: %v", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load runtime configuration: %w", err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	runtime, err := omnidexruntime.New(ctx, cfg, log.Default())
	if err != nil {
		return err
	}
	defer runtime.Close()
	return runtime.Run()
}
