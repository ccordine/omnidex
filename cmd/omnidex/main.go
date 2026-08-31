package main

import (
	"context"
	"io"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/gryph/omnidex/internal/config"
	omnidexruntime "github.com/gryph/omnidex/internal/runtime"
)

func main() {
	if err := run(os.Args[1:], os.Stdin, os.Stdout); err != nil {
		log.Printf("omnidex stopped: %v", err)
		os.Exit(1)
	}
}

func run(args []string, stdin io.Reader, stdout io.Writer) error {
	if len(args) > 0 {
		return runCommand(args, stdin, stdout)
	}
	return runServer()
}

func runServer() error {
	cfg := config.Load()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	runtime, err := omnidexruntime.New(ctx, cfg, log.Default())
	if err != nil {
		return err
	}
	defer runtime.Close()
	return runtime.Run()
}
