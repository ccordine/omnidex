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

func runRequirementPartitionGauntlet(args []string) {
	defaultContext, err := ollamaPrewarmDefaultContext()
	if err != nil {
		die(err.Error())
	}
	fs := flag.NewFlagSet("model:gauntlet requirement-partition", flag.ExitOnError)
	casesPath := fs.String("cases", "gauntlets/requirement_partition/cases.v1.json", "versioned task-neutral cases file")
	labelsPath := fs.String("labels", "gauntlets/requirement_partition/labels.v1.json", "independent labels file loaded only after inference")
	outputPath := fs.String("output", "", "new evidence file; existing files are never overwritten")
	stableModel := fs.String("stable-model", ollamaPrewarmDefaultModel(), "exact Ollama stable model")
	reasoningModel := fs.String("reasoning-model", getenv("OMNIDEX_GAUNTLET_REASONING_MODEL", "deepseek-r1:8b"), "exact Ollama advisory reasoner")
	contextTokens := fs.Int("num-ctx", defaultContext, "hard inference context tokens")
	keepAlive := fs.String("keep-alive", "5m", "positive Ollama runner retention")
	baseURL := fs.String("base-url", defaultOllamaBaseURL(), "Ollama base URL")
	timeout := fs.Duration("timeout", 2*time.Hour, "hard end-to-end timeout")
	_ = fs.Parse(args)
	if fs.NArg() != 0 {
		die("model:gauntlet requirement-partition accepts flags only")
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
	progress := &modelGauntletProgressGenerator{generator: generator}
	result, err := executeRequirementPartitionGauntlet(ctx, modelGauntletOptions{
		CasesPath: *casesPath, LabelsPath: *labelsPath, OutputPath: *outputPath,
		StableModel: *stableModel, ReasoningModel: *reasoningModel,
		ContextTokens: *contextTokens, KeepAlive: *keepAlive,
	}, progress)
	if err != nil {
		die(err.Error())
	}
	direct := result.Evaluation.Scores[modelgauntlet.VariantDirect]
	deliberated := result.Evaluation.Scores[modelgauntlet.VariantDeliberated]
	directMetrics := result.Evaluation.Metrics[modelgauntlet.VariantDirect]
	deliberatedMetrics := result.Evaluation.Metrics[modelgauntlet.VariantDeliberated]
	fmt.Printf("evidence: %s\ndirect: %d/%d correct (%d valid), model-time=%s, output-tokens=%d\ndeliberated: %d/%d correct (%d valid), model-time=%s, output-tokens=%d\n",
		*outputPath, direct.Correct, direct.Total, direct.Valid,
		time.Duration(directMetrics.TotalDuration).Round(time.Millisecond), directMetrics.EvalTokens,
		deliberated.Correct, deliberated.Total, deliberated.Valid,
		time.Duration(deliberatedMetrics.TotalDuration).Round(time.Millisecond), deliberatedMetrics.EvalTokens,
	)
}

func executeRequirementPartitionGauntlet(
	ctx context.Context,
	options modelGauntletOptions,
	generator modelgauntlet.Generator,
) (modelgauntlet.RequirementPartitionResult, error) {
	if strings.TrimSpace(options.OutputPath) == "" {
		return modelgauntlet.RequirementPartitionResult{}, fmt.Errorf("model gauntlet output path is required")
	}
	if _, err := os.Lstat(options.OutputPath); err == nil {
		return modelgauntlet.RequirementPartitionResult{}, fmt.Errorf("model gauntlet output %q already exists", options.OutputPath)
	} else if !os.IsNotExist(err) {
		return modelgauntlet.RequirementPartitionResult{}, fmt.Errorf("inspect model gauntlet output: %w", err)
	}
	cases, err := modelgauntlet.LoadRequirementPartitionCases(options.CasesPath)
	if err != nil {
		return modelgauntlet.RequirementPartitionResult{}, err
	}
	report, err := modelgauntlet.RunRequirementPartition(ctx, modelgauntlet.RequirementPartitionConfig{
		StableModel: options.StableModel, ReasoningModel: options.ReasoningModel,
		ContextTokens: options.ContextTokens, KeepAlive: options.KeepAlive,
	}, cases, generator)
	if err != nil {
		return modelgauntlet.RequirementPartitionResult{}, err
	}
	labels, labelHash, err := modelgauntlet.LoadRequirementPartitionLabels(options.LabelsPath, cases)
	if err != nil {
		return modelgauntlet.RequirementPartitionResult{}, err
	}
	evaluation, err := modelgauntlet.EvaluateRequirementPartition(report, labels)
	if err != nil {
		return modelgauntlet.RequirementPartitionResult{}, err
	}
	result := modelgauntlet.RequirementPartitionResult{
		Schema:      modelgauntlet.RequirementPartitionResultSchemaV1,
		LabelSHA256: labelHash, Report: report, Evaluation: evaluation,
	}
	if err := modelgauntlet.WriteRequirementPartitionResult(options.OutputPath, result); err != nil {
		return modelgauntlet.RequirementPartitionResult{}, err
	}
	return result, nil
}
