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
		return fmt.Errorf("usage: cognition-gauntlet prepare|prepare-matrix|prepare-resume|prepare-transfer|prepare-scale|run|matrix|verify-matrix|resume|verify-resume|transfer|verify-transfer|scale|verify-scale|takeover [arguments]")
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
	case "prepare-matrix":
		if *requestPath == "" || *configPath == "" || *processPath != "" {
			return fmt.Errorf("prepare-matrix requires exactly --request and --config")
		}
		return cognitiongauntlet.PrepareOfflineMatrixConfig(ctx, *requestPath, *configPath)
	case "prepare-resume":
		if *requestPath == "" || *configPath == "" || *processPath != "" {
			return fmt.Errorf("prepare-resume requires exactly --request and --config")
		}
		return cognitiongauntlet.PrepareOfflineResumeConfig(ctx, *requestPath, *configPath)
	case "prepare-transfer":
		if *requestPath == "" || *configPath == "" || *processPath != "" {
			return fmt.Errorf("prepare-transfer requires exactly --request and --config")
		}
		return cognitiongauntlet.PrepareOfflineTransferConfig(ctx, *requestPath, *configPath)
	case "prepare-scale":
		if *requestPath == "" || *configPath == "" || *processPath != "" {
			return fmt.Errorf("prepare-scale requires exactly --request and --config")
		}
		return cognitiongauntlet.PrepareOfflineScaleConfig(ctx, *requestPath, *configPath)
	case "generate":
		if *processPath == "" || *configPath != "" || *requestPath != "" {
			return fmt.Errorf("generate requires exactly --process-config")
		}
		return cognitiongauntlet.RunOfflineGeneratorProcess(ctx, *processPath)
	case "generate-scale":
		if *processPath == "" || *configPath != "" || *requestPath != "" {
			return fmt.Errorf("generate-scale requires exactly --process-config")
		}
		return cognitiongauntlet.RunOfflineScaleGeneratorProcess(ctx, *processPath)
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
	case "matrix":
		if *configPath == "" || *processPath != "" || *requestPath != "" {
			return fmt.Errorf("matrix requires exactly --config")
		}
		config, err := cognitiongauntlet.LoadOfflineMatrixConfig(*configPath)
		if err != nil {
			return err
		}
		executable, err := os.Executable()
		if err != nil {
			return err
		}
		receipt, err := cognitiongauntlet.RunOfflineMatrix(ctx, config, executable)
		if err != nil {
			return err
		}
		fmt.Print(gateEvidenceSummary(
			"sealed matrix runs", receipt.RunCount(),
			receipt.GateEvidenceQualified(), receipt.PromotionEligible(),
		))
		return nil
	case "verify-matrix":
		if *configPath == "" || *processPath != "" || *requestPath != "" {
			return fmt.Errorf("verify-matrix requires exactly --config")
		}
		config, err := cognitiongauntlet.LoadOfflineMatrixConfig(*configPath)
		if err != nil {
			return err
		}
		receipt, err := cognitiongauntlet.LoadVerifiedOfflineMatrixReceipt(config)
		if err != nil {
			return err
		}
		fmt.Print(gateEvidenceSummary(
			"verified sealed matrix runs", receipt.RunCount(),
			receipt.GateEvidenceQualified(), receipt.PromotionEligible(),
		))
		return nil
	case "resume":
		if *configPath == "" || *processPath != "" || *requestPath != "" {
			return fmt.Errorf("resume requires exactly --config")
		}
		config, err := cognitiongauntlet.LoadOfflineResumeConfig(*configPath)
		if err != nil {
			return err
		}
		executable, err := os.Executable()
		if err != nil {
			return err
		}
		receipt, err := cognitiongauntlet.RunOfflineResume(ctx, config, executable)
		if err != nil {
			return err
		}
		fmt.Print(gateEvidenceSummary(
			"sealed Resume schedules", receipt.RunCount(),
			receipt.GateEvidenceQualified(), receipt.PromotionEligible(),
		))
		return nil
	case "verify-resume":
		if *configPath == "" || *processPath != "" || *requestPath != "" {
			return fmt.Errorf("verify-resume requires exactly --config")
		}
		config, err := cognitiongauntlet.LoadOfflineResumeConfig(*configPath)
		if err != nil {
			return err
		}
		receipt, err := cognitiongauntlet.LoadVerifiedOfflineResumeReceipt(config)
		if err != nil {
			return err
		}
		fmt.Print(gateEvidenceSummary(
			"verified sealed Resume schedules", receipt.RunCount(),
			receipt.GateEvidenceQualified(), receipt.PromotionEligible(),
		))
		return nil
	case "transfer":
		if *configPath == "" || *processPath != "" || *requestPath != "" {
			return fmt.Errorf("transfer requires exactly --config")
		}
		config, err := cognitiongauntlet.LoadOfflineTransferConfig(*configPath)
		if err != nil {
			return err
		}
		executable, err := os.Executable()
		if err != nil {
			return err
		}
		receipt, err := cognitiongauntlet.RunOfflineTransfer(ctx, config, executable)
		if err != nil {
			return err
		}
		fmt.Print(gateEvidenceSummary(
			"sealed Transfer surfaces", len(receipt.Receipt().Runs),
			receipt.GateEvidenceQualified(), receipt.PromotionEligible(),
		))
		return nil
	case "verify-transfer":
		if *configPath == "" || *processPath != "" || *requestPath != "" {
			return fmt.Errorf("verify-transfer requires exactly --config")
		}
		config, err := cognitiongauntlet.LoadOfflineTransferConfig(*configPath)
		if err != nil {
			return err
		}
		receipt, err := cognitiongauntlet.LoadVerifiedOfflineTransferReceipt(config)
		if err != nil {
			return err
		}
		fmt.Print(gateEvidenceSummary(
			"verified sealed Transfer surfaces", len(receipt.Receipt().Runs),
			receipt.GateEvidenceQualified(), receipt.PromotionEligible(),
		))
		return nil
	case "scale":
		if *configPath == "" || *processPath != "" || *requestPath != "" {
			return fmt.Errorf("scale requires exactly --config")
		}
		config, err := cognitiongauntlet.LoadOfflineScaleConfig(*configPath)
		if err != nil {
			return err
		}
		executable, err := os.Executable()
		if err != nil {
			return err
		}
		receipt, err := cognitiongauntlet.RunOfflineScale(ctx, config, executable)
		if err != nil {
			return err
		}
		fmt.Print(gateEvidenceSummary(
			"sealed Scale runs", len(receipt.Receipt().Runs),
			receipt.GateEvidenceQualified(), receipt.PromotionEligible(),
		))
		return nil
	case "verify-scale":
		if *configPath == "" || *processPath != "" || *requestPath != "" {
			return fmt.Errorf("verify-scale requires exactly --config")
		}
		config, err := cognitiongauntlet.LoadOfflineScaleConfig(*configPath)
		if err != nil {
			return err
		}
		receipt, err := cognitiongauntlet.LoadVerifiedOfflineScaleReceipt(config)
		if err != nil {
			return err
		}
		fmt.Print(gateEvidenceSummary(
			"verified sealed Scale runs", len(receipt.Receipt().Runs),
			receipt.GateEvidenceQualified(), receipt.PromotionEligible(),
		))
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
	case "evaluate-scale":
		if *processPath == "" || *configPath != "" || *requestPath != "" {
			return fmt.Errorf("evaluate-scale requires exactly --process-config")
		}
		return cognitiongauntlet.RunOfflineScaleEvaluatorProcess(ctx, *processPath)
	default:
		return fmt.Errorf("offline cognition phase %q is not registered", phase)
	}
}
