package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/modelgauntlet"
)

type completeRequirementGauntletOptions struct {
	CasesPath      string
	LabelsPath     string
	OutputPath     string
	StableModel    string
	ReasoningModel string
	ContextTokens  int
	KeepAlive      string
	Repetitions    int
	HardwareClass  string
	Backend        string
}

func runCompleteRequirementPartitionGauntlet(args []string) {
	defaultContext, err := ollamaPrewarmDefaultContext()
	if err != nil {
		die(err.Error())
	}
	fs := flag.NewFlagSet("model:gauntlet requirement-partition-complete", flag.ExitOnError)
	casesPath := fs.String("cases", "gauntlets/requirement_partition_complete/cases.v1.json", "frozen complete-request cases file")
	labelsPath := fs.String("labels", "gauntlets/requirement_partition_complete/labels.v1.json", "withheld labels loaded only after all inference")
	outputPath := fs.String("output", "", "new evidence file; existing files are never overwritten")
	stableModel := fs.String("stable-model", ollamaPrewarmDefaultModel(), "exact Ollama stable model")
	reasoningModel := fs.String("reasoning-model", getenv("OMNIDEX_GAUNTLET_REASONING_MODEL", "deepseek-r1:8b"), "exact Ollama advisory reasoner")
	contextTokens := fs.Int("num-ctx", defaultContext, "hard inference context tokens")
	keepAlive := fs.String("keep-alive", "5m", "positive Ollama runner retention")
	repetitions := fs.Int("repetitions", 2, "complete repetitions of every case and variant")
	hardwareClass := fs.String("hardware-class", getenv("OMNIDEX_GAUNTLET_HARDWARE_CLASS", ""), "required reproducible hardware class")
	backend := fs.String("backend", getenv("OMNIDEX_GAUNTLET_BACKEND", ""), "required local inference backend")
	baseURL := fs.String("base-url", defaultOllamaBaseURL(), "Ollama base URL")
	timeout := fs.Duration("timeout", 24*time.Hour, "hard end-to-end timeout")
	_ = fs.Parse(args)
	if fs.NArg() != 0 {
		die("model:gauntlet requirement-partition-complete accepts flags only")
	}
	if *timeout <= 0 {
		die("model gauntlet timeout must be positive")
	}
	generator, err := modelgauntlet.NewOllamaGenerator(*baseURL, &http.Client{})
	if err != nil {
		die(err.Error())
	}
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	result, err := executeCompleteRequirementGauntlet(ctx, completeRequirementGauntletOptions{
		CasesPath: *casesPath, LabelsPath: *labelsPath, OutputPath: *outputPath,
		StableModel: *stableModel, ReasoningModel: *reasoningModel,
		ContextTokens: *contextTokens, KeepAlive: *keepAlive, Repetitions: *repetitions,
		HardwareClass: *hardwareClass, Backend: *backend,
	}, &modelGauntletProgressGenerator{generator: generator})
	if err != nil {
		die(err.Error())
	}
	printCompleteRequirementResult(*outputPath, result)
}

func executeCompleteRequirementGauntlet(
	ctx context.Context,
	options completeRequirementGauntletOptions,
	generator modelgauntlet.Generator,
) (modelgauntlet.CompleteRequirementResult, error) {
	if strings.TrimSpace(options.OutputPath) == "" {
		return modelgauntlet.CompleteRequirementResult{}, fmt.Errorf("model gauntlet output path is required")
	}
	if _, err := os.Lstat(options.OutputPath); err == nil {
		return modelgauntlet.CompleteRequirementResult{}, fmt.Errorf("model gauntlet output %q already exists", options.OutputPath)
	} else if !os.IsNotExist(err) {
		return modelgauntlet.CompleteRequirementResult{}, fmt.Errorf("inspect model gauntlet output: %w", err)
	}
	cases, casesHash, err := modelgauntlet.LoadCompleteRequirementCases(options.CasesPath)
	if err != nil {
		return modelgauntlet.CompleteRequirementResult{}, err
	}
	report, err := modelgauntlet.RunCompleteRequirementPartition(ctx, modelgauntlet.CompleteRequirementConfig{
		StableModel: options.StableModel, ReasoningModel: options.ReasoningModel,
		ContextTokens: options.ContextTokens, KeepAlive: options.KeepAlive,
		Repetitions: options.Repetitions, CasesSHA256: casesHash,
		HardwareClass: options.HardwareClass, Backend: options.Backend,
	}, cases, generator)
	if err != nil {
		return modelgauntlet.CompleteRequirementResult{}, err
	}
	// Labels intentionally enter process state only after every model call has stopped.
	labels, labelHash, err := modelgauntlet.LoadCompleteRequirementLabels(options.LabelsPath, cases)
	if err != nil {
		return modelgauntlet.CompleteRequirementResult{}, err
	}
	evaluation, err := modelgauntlet.EvaluateCompleteRequirementPartition(report, labels)
	if err != nil {
		return modelgauntlet.CompleteRequirementResult{}, err
	}
	result := modelgauntlet.CompleteRequirementResult{
		Schema:      modelgauntlet.CompleteRequirementResultSchemaV1,
		LabelSHA256: labelHash, Report: report, Evaluation: evaluation,
	}
	if err := modelgauntlet.WriteCompleteRequirementResult(options.OutputPath, result); err != nil {
		return modelgauntlet.CompleteRequirementResult{}, err
	}
	return result, nil
}

func printCompleteRequirementResult(path string, result modelgauntlet.CompleteRequirementResult) {
	fmt.Printf("evidence: %s\n", path)
	for _, variant := range []modelgauntlet.Variant{
		modelgauntlet.VariantDirect,
		modelgauntlet.VariantPerSplitAdvisory,
		modelgauntlet.VariantFinalPassAdvisory,
	} {
		score := result.Evaluation.Scores[variant]
		metrics := result.Evaluation.Metrics[variant]
		stability := result.Evaluation.Stability[variant]
		fmt.Printf("%s: %d/%d correct (%d valid), stable=%d/%d, calls=%d, model-time=%s, output-tokens=%d\n",
			variant, score.Correct, score.Total, score.Valid, stability.Stable, stability.Cases,
			metrics.Calls, time.Duration(metrics.TotalDuration).Round(time.Millisecond), metrics.EvalTokens,
		)
	}
	transition := result.Evaluation.Transitions[modelgauntlet.VariantFinalPassAdvisory]
	fmt.Printf("final-pass transitions: pass→pass=%d pass→fail=%d fail→pass=%d fail→fail=%d\n",
		transition.DirectPassAssistedPass, transition.DirectPassAssistedFail,
		transition.DirectFailAssistedPass, transition.DirectFailAssistedFail,
	)
	if result.Evaluation.Promotion.Eligible {
		fmt.Println("promotion gate: eligible")
		return
	}
	fmt.Printf("promotion gate: rejected: %s\n", strings.Join(result.Evaluation.Promotion.Reasons, "; "))
}
