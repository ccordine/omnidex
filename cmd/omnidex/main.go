package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"syscall"

	"github.com/gryph/omnidex/internal/config"
	omnidexruntime "github.com/gryph/omnidex/internal/runtime"
	"github.com/gryph/omnidex/internal/workflow"
)

func main() {
	if err := run(os.Args[1:], os.Stdin, os.Stdout); err != nil {
		var exit *exec.ExitError
		if len(os.Args) > 1 && os.Args[1] == "workflow-exec" && errors.As(err, &exit) && exit.ExitCode() > 0 {
			os.Exit(exit.ExitCode())
		}
		log.Printf("omnidex stopped: %v", err)
		os.Exit(1)
	}
}

func run(args []string, stdin io.Reader, stdout io.Writer) error {
	if len(args) > 0 && args[0] == "workflow-exec" {
		if len(args) < 4 {
			return fmt.Errorf("workflow-exec requires its scope, tool and command")
		}
		scope, err := workflow.Load(args[1])
		if err != nil {
			return err
		}
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		return workflow.Execute(ctx, scope, args[2], args[3:], stdin, stdout, os.Stderr)
	}
	if len(args) > 0 {
		return runCommand(args, stdin, stdout)
	}
	return runServer()
}

func runServer() error {
	cfg, err := config.Load()
	if err != nil {
		return err
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
