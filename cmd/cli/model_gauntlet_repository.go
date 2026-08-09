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

func runRepositoryRetrievalGauntlet(args []string) {
	defaultContext, err := ollamaPrewarmDefaultContext()
	if err != nil {
		die(err.Error())
	}
	fs := flag.NewFlagSet("model:gauntlet repository-retrieval", flag.ExitOnError)
	casesPath := fs.String("cases", "gauntlets/repository_retrieval/cases.v2.json", "versioned path-blind cases file")
	labelsPath := fs.String("labels", "gauntlets/repository_retrieval/labels.v2.json", "independent labels file loaded only after inference")
	outputPath := fs.String("output", "", "new evidence file; existing files are never overwritten")
	stableModel := fs.String("stable-model", ollamaPrewarmDefaultModel(), "exact Ollama stable model")
	reasoningModel := fs.String("reasoning-model", getenv("OMNIDEX_GAUNTLET_REASONING_MODEL", "deepseek-r1:8b"), "exact Ollama advisory reasoner")
	contextTokens := fs.Int("num-ctx", defaultContext, "hard inference context tokens")
	keepAlive := fs.String("keep-alive", "5m", "positive Ollama runner retention")
	baseURL := fs.String("base-url", defaultOllamaBaseURL(), "Ollama base URL")
	timeout := fs.Duration("timeout", 2*time.Hour, "hard end-to-end timeout")
	_ = fs.Parse(args)
	if fs.NArg() != 0 {
		die("model:gauntlet repository-retrieval accepts flags only")
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
	result, err := executeRepositoryRetrievalGauntlet(ctx, modelGauntletOptions{
		CasesPath: *casesPath, LabelsPath: *labelsPath, OutputPath: *outputPath,
		StableModel: *stableModel, ReasoningModel: *reasoningModel,
		ContextTokens: *contextTokens, KeepAlive: *keepAlive,
	}, &modelGauntletProgressGenerator{generator: generator})
	if err != nil {
		die(err.Error())
	}
	direct := result.Evaluation.Scores[modelgauntlet.VariantDirect]
	assisted := result.Evaluation.Scores[modelgauntlet.VariantDeliberated]
	directMetrics := result.Evaluation.Metrics[modelgauntlet.VariantDirect]
	assistedMetrics := result.Evaluation.Metrics[modelgauntlet.VariantDeliberated]
	fmt.Printf("evidence: %s\ndirect: %d/%d correct (%d valid), model-time=%s, output-tokens=%d\ndeliberated: %d/%d correct (%d valid), model-time=%s, output-tokens=%d\n",
		*outputPath, direct.Correct, direct.Total, direct.Valid,
		time.Duration(directMetrics.TotalDuration).Round(time.Millisecond), directMetrics.EvalTokens,
		assisted.Correct, assisted.Total, assisted.Valid,
		time.Duration(assistedMetrics.TotalDuration).Round(time.Millisecond), assistedMetrics.EvalTokens,
	)
}

func executeRepositoryRetrievalGauntlet(
	ctx context.Context,
	options modelGauntletOptions,
	generator modelgauntlet.Generator,
) (modelgauntlet.RepositoryRetrievalResult, error) {
	if strings.TrimSpace(options.OutputPath) == "" {
		return modelgauntlet.RepositoryRetrievalResult{}, fmt.Errorf("model gauntlet output path is required")
	}
	if _, err := os.Lstat(options.OutputPath); err == nil {
		return modelgauntlet.RepositoryRetrievalResult{}, fmt.Errorf("model gauntlet output %q already exists", options.OutputPath)
	} else if !os.IsNotExist(err) {
		return modelgauntlet.RepositoryRetrievalResult{}, fmt.Errorf("inspect model gauntlet output: %w", err)
	}
	cases, err := modelgauntlet.LoadRepositoryRetrievalCases(options.CasesPath)
	if err != nil {
		return modelgauntlet.RepositoryRetrievalResult{}, err
	}
	report, err := modelgauntlet.RunRepositoryRetrieval(ctx, modelgauntlet.RepositoryRetrievalConfig{
		StableModel: options.StableModel, ReasoningModel: options.ReasoningModel,
		ContextTokens: options.ContextTokens, KeepAlive: options.KeepAlive,
	}, cases, generator)
	if err != nil {
		return modelgauntlet.RepositoryRetrievalResult{}, err
	}
	labels, labelHash, err := modelgauntlet.LoadRepositoryRetrievalLabels(options.LabelsPath, cases)
	if err != nil {
		return modelgauntlet.RepositoryRetrievalResult{}, err
	}
	evaluation, err := modelgauntlet.EvaluateRepositoryRetrieval(report, labels)
	if err != nil {
		return modelgauntlet.RepositoryRetrievalResult{}, err
	}
	result := modelgauntlet.RepositoryRetrievalResult{
		Schema:      modelgauntlet.RepositoryRetrievalResultSchemaV2,
		LabelSHA256: labelHash, Report: report, Evaluation: evaluation,
	}
	if err := modelgauntlet.WriteRepositoryRetrievalResult(options.OutputPath, result); err != nil {
		return modelgauntlet.RepositoryRetrievalResult{}, err
	}
	return result, nil
}
