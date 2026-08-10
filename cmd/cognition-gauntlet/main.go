package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/gryph/omnidex/internal/cognitiongauntlet"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) < 2 {
		return fmt.Errorf("usage: cognition-gauntlet prepare|run|takeover [arguments]")
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	phase := os.Args[1]
	flags := flag.NewFlagSet(phase, flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	configPath := flags.String("config", "", "sealed offline promotion configuration")
	processPath := flags.String("process-config", "", "private child process configuration")
	requestPath := flags.String("request", "", "strict offline experiment request")
	if err := flags.Parse(os.Args[2:]); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("offline cognition command received unexpected arguments")
	}
	switch phase {
	case "prepare":
		if *requestPath == "" || *configPath == "" || *processPath != "" {
			return fmt.Errorf("prepare requires exactly --request and --config")
		}
		return cognitiongauntlet.PrepareOfflineExperimentConfig(ctx, *requestPath, *configPath)
	case "generate":
		if *processPath == "" || *configPath != "" || *requestPath != "" {
			return fmt.Errorf("generate requires exactly --process-config")
		}
		return cognitiongauntlet.RunOfflineGeneratorProcess(ctx, *processPath)
	case "host":
		if *processPath == "" || *configPath != "" || *requestPath != "" {
			return fmt.Errorf("host requires exactly --process-config")
		}
		return cognitiongauntlet.RunOfflineHostProcess(ctx, *processPath)
	case "run":
		if *configPath == "" || *processPath != "" || *requestPath != "" {
			return fmt.Errorf("run requires exactly --config")
		}
		config, err := cognitiongauntlet.LoadOfflinePromotionConfig(*configPath)
		if err != nil {
			return err
		}
		executable, err := os.Executable()
		if err != nil {
			return err
		}
		receipt, err := cognitiongauntlet.RunOfflinePromotion(ctx, config, executable)
		if err != nil {
			return err
		}
		fmt.Printf("sealed episode %s; private evaluation %s\n", receipt.EpisodeSealSHA256, receipt.EvaluationOracleSHA256)
		return nil
	case "takeover":
		if *configPath == "" || *processPath != "" || *requestPath != "" {
			return fmt.Errorf("takeover requires exactly --config")
		}
		config, err := cognitiongauntlet.LoadOfflineTakeoverConfig(*configPath)
		if err != nil {
			return err
		}
		executable, err := os.Executable()
		if err != nil {
			return err
		}
		receipt, err := cognitiongauntlet.RunOfflineTakeover(ctx, config, executable)
		if err != nil {
			return err
		}
		fmt.Printf(
			"sealed takeover episode %s; continuity %s\n",
			receipt.EpisodeSealSHA256, receipt.Continuity.Before.SemanticSHA256,
		)
		return nil
	case "infer":
		if *processPath == "" || *configPath != "" || *requestPath != "" {
			return fmt.Errorf("infer requires exactly --process-config")
		}
		return cognitiongauntlet.RunOfflineInferenceProcess(ctx, *processPath)
	case "evaluate":
		if *processPath == "" || *configPath != "" || *requestPath != "" {
			return fmt.Errorf("evaluate requires exactly --process-config")
		}
		return cognitiongauntlet.RunOfflineEvaluatorProcess(ctx, *processPath)
	default:
		return fmt.Errorf("offline cognition phase %q is not registered", phase)
	}
}
