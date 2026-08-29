package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/db"
	"github.com/gryph/omnidex/internal/envfile"
	"github.com/gryph/omnidex/internal/ollama"
	"github.com/gryph/omnidex/internal/queue"
	"github.com/gryph/omnidex/internal/worker"
)

type stationReplayOptions struct {
	OpeningID       int64
	JobID           int64
	WorkKind        string
	Models          []string
	Report          string
	Config          string
	OllamaURL       string
	Timeout         time.Duration
	CurrentContract bool
}

type stationReplayModels []string

func (models *stationReplayModels) String() string { return strings.Join(*models, ",") }

func (models *stationReplayModels) Set(raw string) error {
	model := strings.TrimSpace(raw)
	if model == "" || model != raw {
		return errors.New("replay model must be exact non-empty text")
	}
	*models = append(*models, model)
	return nil
}

type stationReplayConfig struct {
	DatabaseURL    string
	DatabaseSchema string
	OllamaBaseURL  string
}

func runStationReplay(args []string) {
	options, err := parseStationReplayOptions(args)
	if err != nil {
		die("bench:replay: " + err.Error())
	}
	executable, err := os.Executable()
	if err != nil {
		die("bench:replay: resolve executable: " + err.Error())
	}
	values, err := stationReplayEnvironment(options.Config, executable)
	if err != nil {
		die("bench:replay: " + err.Error())
	}
	config, err := stationReplayRuntimeConfig(values, options.OllamaURL)
	if err != nil {
		die("bench:replay: " + err.Error())
	}
	ctx := context.Background()
	pool, err := db.ConnectRuntimeReadOnly(ctx, config.DatabaseURL, config.DatabaseSchema)
	if err != nil {
		die("bench:replay: open read-only runtime database: " + err.Error())
	}
	defer pool.Close()
	repository := queue.New(pool)
	point, err := loadStationReplayPoint(ctx, repository, options)
	if err != nil {
		die("bench:replay: " + err.Error())
	}
	report, err := openStationReplayReport(options.Report)
	if err != nil {
		die("bench:replay: " + err.Error())
	}
	encoder := newStationReplayReportEncoder(report)
	encoder.SetEscapeHTML(false)
	reportSchema := stationReplayReportSchema
	if options.CurrentContract {
		reportSchema = stationCurrentContractReplayReportSchema
	}
	header := stationReplayReportHeader{
		Type: "header", Schema: reportSchema, CreatedAt: time.Now().UTC(),
		SourceCallOpening:           point.Call,
		SourceCallWireRequestBase64: stationReplayBase64(point.Call.WireRequest),
		SourceGapOpening:            point.Gap, Models: append([]string(nil), options.Models...),
		Timeout: options.Timeout.String(),
	}
	if err := writeStationReplayReport(encoder, report, header); err != nil {
		_ = report.Close()
		die("bench:replay: write report header: " + err.Error())
	}

	client := stationReplayClient(config.OllamaBaseURL, point.Gap.ContextTokens, options.Timeout)
	failures := 0
	for _, modelName := range options.Models {
		started := time.Now().UTC()
		callCtx, cancel := stationReplayContext(options.Timeout)
		var replay worker.ExactStationReplay
		var replayErr error
		if options.CurrentContract {
			replay, replayErr = worker.ReplayStationWithCurrentContract(callCtx, client, point, modelName)
		} else {
			replay, replayErr = worker.ReplayExactStation(callCtx, client, point, modelName)
		}
		cancel()
		finished := time.Now().UTC()
		run := stationReplayReportRun{
			Type: "run", StartedAt: started, FinishedAt: finished,
			Status: "passed", Replay: replay,
			ProviderResponseCaptureBase64: stationReplayBase64(replay.Generation.ProviderResponseCapture),
			ProviderIdentityEvidence:      replay.Generation.ProviderIdentityEvidence,
		}
		if replayErr != nil {
			run.Status, run.Error = "failed", replayErr.Error()
			failures++
		}
		if err := writeStationReplayReport(encoder, report, run); err != nil {
			_ = report.Close()
			die("bench:replay: append model result: " + err.Error())
		}
		printStationReplayRun(run)
	}
	if err := report.Close(); err != nil {
		die("bench:replay: close report: " + err.Error())
	}
	if failures > 0 {
		die(fmt.Sprintf("bench:replay recorded %d failed model runs; exact evidence is in %s", failures, options.Report))
	}
}

func parseStationReplayOptions(args []string) (stationReplayOptions, error) {
	var options stationReplayOptions
	var models stationReplayModels
	fs := flag.NewFlagSet("bench:replay", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.Int64Var(&options.OpeningID, "opening", 0, "exact historical station-call opening ID")
	fs.Int64Var(&options.JobID, "job", 0, "historical job ID; selects latest matching opening")
	fs.StringVar(&options.WorkKind, "work-kind", "fragment_correction", "portable work kind used with --job")
	fs.Var(&models, "model", "exact installed model to replay (repeatable)")
	fs.StringVar(&options.Report, "report", "", "new JSONL evidence file path")
	fs.StringVar(&options.Config, "config", "", "read-only Omnidex environment file")
	fs.StringVar(&options.OllamaURL, "ollama-url", "", "Ollama URL override")
	fs.DurationVar(&options.Timeout, "timeout", 0, "per-model timeout; 0 waits for provider completion")
	fs.BoolVar(&options.CurrentContract, "current-contract", false, "preserve the frozen portable packet while using the checked-in transport contract")
	if err := fs.Parse(args); err != nil {
		return options, err
	}
	if len(fs.Args()) != 0 || options.Timeout < 0 || (options.OpeningID > 0) == (options.JobID > 0) ||
		len(models) == 0 || strings.TrimSpace(options.Report) == "" {
		return options, errors.New("requires exactly one of --opening or --job, one or more --model values, and --report")
	}
	options.WorkKind = strings.TrimSpace(options.WorkKind)
	if options.OpeningID < 0 || options.JobID < 0 || options.WorkKind == "" || len(options.WorkKind) > 128 {
		return options, errors.New("replay opening, job, or work kind is invalid")
	}
	seen := make(map[string]struct{}, len(models))
	for _, model := range models {
		if _, exists := seen[model]; exists {
			return options, fmt.Errorf("replay model %q is duplicated", model)
		}
		seen[model] = struct{}{}
	}
	options.Models = append([]string(nil), models...)
	return options, nil
}

func stationReplayEnvironment(configPath, executable string) (map[string]string, error) {
	if strings.TrimSpace(configPath) != "" {
		return envfile.Read(filepath.Clean(configPath))
	}
	if _, configured := os.LookupEnv("DATABASE_URL"); configured {
		values := make(map[string]string, 3)
		for _, key := range []string{"DATABASE_URL", "DATABASE_SCHEMA", "OLLAMA_BASE_URL"} {
			if value, exists := os.LookupEnv(key); exists {
				values[key] = value
			}
		}
		return values, nil
	}
	path, err := managedEnvironmentPath(executable)
	if err != nil {
		return nil, fmt.Errorf("resolve managed environment: %w", err)
	}
	return envfile.Read(path)
}

func stationReplayRuntimeConfig(values map[string]string, ollamaURL string) (stationReplayConfig, error) {
	config := stationReplayConfig{DatabaseURL: strings.TrimSpace(values["DATABASE_URL"])}
	if config.DatabaseURL == "" {
		return config, errors.New("DATABASE_URL is required")
	}
	if config.DatabaseURL != values["DATABASE_URL"] {
		return config, errors.New("DATABASE_URL must not contain surrounding whitespace")
	}
	config.DatabaseSchema = db.DefaultRuntimeSchema
	if value, exists := values["DATABASE_SCHEMA"]; exists {
		if value == "" {
			return config, errors.New("DATABASE_SCHEMA is explicitly empty")
		}
		config.DatabaseSchema = value
	}
	if err := db.ValidateRuntimeSchemaName(config.DatabaseSchema); err != nil {
		return config, fmt.Errorf("DATABASE_SCHEMA: %w", err)
	}
	config.OllamaBaseURL = strings.TrimSpace(ollamaURL)
	if config.OllamaBaseURL == "" {
		config.OllamaBaseURL = strings.TrimSpace(values["OLLAMA_BASE_URL"])
	}
	if config.OllamaBaseURL == "" {
		return config, errors.New("OLLAMA_BASE_URL is required")
	}
	return config, nil
}

func loadStationReplayPoint(
	ctx context.Context,
	repository *queue.Repository,
	options stationReplayOptions,
) (queue.StationCallReplayPoint, error) {
	if options.OpeningID > 0 {
		return repository.ReadStationCallReplayPoint(ctx, options.OpeningID)
	}
	return repository.FindLatestStationCallReplayPoint(ctx, options.JobID, options.WorkKind)
}

func stationReplayClient(baseURL string, contextTokens int, timeout time.Duration) *ollama.Client {
	if timeout == 0 {
		return ollama.NewUnbounded(baseURL, "", "", contextTokens)
	}
	return ollama.New(baseURL, "", "", timeout, contextTokens)
}

func stationReplayContext(timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout == 0 {
		return context.Background(), func() {}
	}
	return context.WithTimeout(context.Background(), timeout)
}
