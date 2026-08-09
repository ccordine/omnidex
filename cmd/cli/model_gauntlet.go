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

type modelGauntletOptions struct {
	CasesPath      string
	LabelsPath     string
	OutputPath     string
	StableModel    string
	ReasoningModel string
	ContextTokens  int
	KeepAlive      string
}

func runModelGauntlet(args []string) {
	if len(args) == 0 {
		die("model:gauntlet requires capability-relation, requirement-partition, requirement-partition-complete, or repository-retrieval")
	}
	switch args[0] {
	case "capability-relation":
		runCapabilityRelationGauntlet(args[1:])
	case "requirement-partition":
		runRequirementPartitionGauntlet(args[1:])
	case "requirement-partition-complete":
		runCompleteRequirementPartitionGauntlet(args[1:])
	case "repository-retrieval":
		runRepositoryRetrievalGauntlet(args[1:])
	default:
		die(fmt.Sprintf("model:gauntlet subcommand %q is unsupported", args[0]))
	}
}

func runCapabilityRelationGauntlet(args []string) {
	defaultContext, err := ollamaPrewarmDefaultContext()
	if err != nil {
		die(err.Error())
	}
	fs := flag.NewFlagSet("model:gauntlet capability-relation", flag.ExitOnError)
	casesPath := fs.String("cases", "gauntlets/capability_relation/cases.v1.json", "versioned task-neutral cases file")
	labelsPath := fs.String("labels", "gauntlets/capability_relation/labels.v1.json", "independent labels file loaded only after inference")
	outputPath := fs.String("output", "", "new evidence file; existing files are never overwritten")
	stableModel := fs.String("stable-model", ollamaPrewarmDefaultModel(), "exact Ollama stable model")
	reasoningModel := fs.String("reasoning-model", getenv("OMNIDEX_GAUNTLET_REASONING_MODEL", "deepseek-r1:8b"), "exact Ollama advisory reasoner")
	contextTokens := fs.Int("num-ctx", defaultContext, "hard inference context tokens")
	keepAlive := fs.String("keep-alive", "5m", "positive Ollama runner retention")
	baseURL := fs.String("base-url", defaultOllamaBaseURL(), "Ollama base URL")
	timeout := fs.Duration("timeout", 2*time.Hour, "hard end-to-end timeout")
	_ = fs.Parse(args)
	if fs.NArg() != 0 {
		die("model:gauntlet capability-relation accepts flags only")
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
	result, err := executeCapabilityRelationGauntlet(ctx, modelGauntletOptions{
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

func executeCapabilityRelationGauntlet(
	ctx context.Context,
	options modelGauntletOptions,
	generator modelgauntlet.Generator,
) (modelgauntlet.CapabilityRelationResult, error) {
	if strings.TrimSpace(options.OutputPath) == "" {
		return modelgauntlet.CapabilityRelationResult{}, fmt.Errorf("model gauntlet output path is required")
	}
	if _, err := os.Lstat(options.OutputPath); err == nil {
		return modelgauntlet.CapabilityRelationResult{}, fmt.Errorf("model gauntlet output %q already exists", options.OutputPath)
	} else if !os.IsNotExist(err) {
		return modelgauntlet.CapabilityRelationResult{}, fmt.Errorf("inspect model gauntlet output: %w", err)
	}
	cases, err := modelgauntlet.LoadCapabilityRelationCases(options.CasesPath)
	if err != nil {
		return modelgauntlet.CapabilityRelationResult{}, err
	}
	report, err := modelgauntlet.RunCapabilityRelation(ctx, modelgauntlet.CapabilityRelationConfig{
		StableModel: options.StableModel, ReasoningModel: options.ReasoningModel,
		ContextTokens: options.ContextTokens, KeepAlive: options.KeepAlive,
	}, cases, generator)
	if err != nil {
		return modelgauntlet.CapabilityRelationResult{}, err
	}
	labels, labelHash, err := modelgauntlet.LoadCapabilityRelationLabels(options.LabelsPath, cases)
	if err != nil {
		return modelgauntlet.CapabilityRelationResult{}, err
	}
	evaluation, err := modelgauntlet.EvaluateCapabilityRelation(report, labels)
	if err != nil {
		return modelgauntlet.CapabilityRelationResult{}, err
	}
	result := modelgauntlet.CapabilityRelationResult{
		Schema:      modelgauntlet.CapabilityRelationResultSchemaV1,
		LabelSHA256: labelHash, Report: report, Evaluation: evaluation,
	}
	if err := modelgauntlet.WriteCapabilityRelationResult(options.OutputPath, result); err != nil {
		return modelgauntlet.CapabilityRelationResult{}, err
	}
	return result, nil
}

type modelGauntletProgressGenerator struct {
	generator modelgauntlet.Generator
}

func (progress *modelGauntletProgressGenerator) Generate(ctx context.Context, request modelgauntlet.GenerateRequest) (modelgauntlet.GenerateResponse, error) {
	type outcome struct {
		response modelgauntlet.GenerateResponse
		err      error
	}
	started := time.Now()
	fmt.Fprintf(os.Stderr, "gauntlet: case=%s repetition=%d variant=%s operation=%s stage=%s model=%s started\n",
		request.CaseID, request.Repetition, request.Variant, request.Operation, request.Stage, request.Model)
	completed := make(chan outcome, 1)
	go func() {
		response, err := progress.generator.Generate(ctx, request)
		completed <- outcome{response: response, err: err}
	}()
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case result := <-completed:
			if result.err != nil {
				fmt.Fprintf(os.Stderr, "gauntlet: case=%s repetition=%d variant=%s operation=%s stage=%s failed elapsed=%s error=%s\n",
					request.CaseID, request.Repetition, request.Variant, request.Operation, request.Stage,
					time.Since(started).Round(time.Second), result.err,
				)
			} else {
				fmt.Fprintf(os.Stderr, "gauntlet: case=%s repetition=%d variant=%s operation=%s stage=%s finished elapsed=%s thinking=%dB content=%dB\n",
					request.CaseID, request.Repetition, request.Variant, request.Operation, request.Stage,
					time.Since(started).Round(time.Second), len(result.response.Thinking), len(result.response.Content),
				)
			}
			return result.response, result.err
		case <-ticker.C:
			fmt.Fprintf(os.Stderr, "gauntlet: case=%s stage=%s still-working elapsed=%s\n", request.CaseID, request.Stage, time.Since(started).Round(time.Second))
		case <-ctx.Done():
			return modelgauntlet.GenerateResponse{}, ctx.Err()
		}
	}
}
