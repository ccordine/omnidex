package main

import (
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/worker"
)

func TestParseStationReplayOptionsRequiresOneFrozenPointAndReport(t *testing.T) {
	options, err := parseStationReplayOptions([]string{
		"--job", "22", "--work-kind", "fragment_correction",
		"--model", "qwen3.5:9b-q4_K_M", "--model", "deepseek-r1:8b",
		"--report", "/tmp/replay.jsonl", "--current-contract",
	})
	if err != nil {
		t.Fatal(err)
	}
	if options.JobID != 22 || options.OpeningID != 0 || options.WorkKind != "fragment_correction" ||
		len(options.Models) != 2 || options.Timeout != 0 || !options.CurrentContract {
		t.Fatalf("options=%+v", options)
	}

	for name, args := range map[string][]string{
		"no frozen point":    {"--model", "qwen", "--report", "/tmp/replay.jsonl"},
		"both frozen points": {"--opening", "7", "--job", "22", "--model", "qwen", "--report", "/tmp/replay.jsonl"},
		"no model":           {"--opening", "7", "--report", "/tmp/replay.jsonl"},
		"no report":          {"--opening", "7", "--model", "qwen"},
		"duplicate model":    {"--opening", "7", "--model", "qwen", "--model", "qwen", "--report", "/tmp/replay.jsonl"},
		"conflicting modes":  {"--opening", "7", "--model", "qwen", "--report", "/tmp/replay.jsonl", "--current-contract", "--compiler-converge"},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := parseStationReplayOptions(args); err == nil {
				t.Fatal("expected option rejection")
			}
		})
	}
}

func TestParseStationReplayOptionsSupportsSpecificationConvergenceReviewer(t *testing.T) {
	options, err := parseStationReplayOptions([]string{
		"--opening", "266", "--model", "llama3.2:3b",
		"--review-model", "gemma3:4b", "--specification-converge",
		"--report", "/tmp/specification-convergence.jsonl",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !options.SpecificationConverge || options.ReviewModel != "gemma3:4b" {
		t.Fatalf("options=%+v", options)
	}
	for name, args := range map[string][]string{
		"missing reviewer": {
			"--opening", "266", "--model", "llama3.2:3b", "--specification-converge",
			"--report", "/tmp/specification-convergence.jsonl",
		},
		"reviewer outside mode": {
			"--opening", "266", "--model", "llama3.2:3b", "--review-model", "gemma3:4b",
			"--report", "/tmp/specification-convergence.jsonl",
		},
		"conflicting replay mode": {
			"--opening", "266", "--model", "llama3.2:3b", "--review-model", "gemma3:4b",
			"--specification-converge", "--current-contract", "--report", "/tmp/specification-convergence.jsonl",
		},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := parseStationReplayOptions(args); err == nil {
				t.Fatal("expected option rejection")
			}
		})
	}
}

func TestStationReplayRuntimeConfigRequiresDirectReadOnlyInputs(t *testing.T) {
	config, err := stationReplayRuntimeConfig(map[string]string{
		"DATABASE_URL":    "postgres://agent:secret@127.0.0.1/omnidex",
		"DATABASE_SCHEMA": "replay_runtime",
		"OLLAMA_BASE_URL": "http://127.0.0.1:11434",
	}, "")
	if err != nil {
		t.Fatal(err)
	}
	if config.DatabaseSchema != "replay_runtime" || config.OllamaBaseURL != "http://127.0.0.1:11434" {
		t.Fatalf("config=%+v", config)
	}

	_, err = stationReplayRuntimeConfig(map[string]string{
		"DATABASE_URL": "postgres://agent:secret@127.0.0.1/omnidex",
	}, "")
	if err == nil || !strings.Contains(err.Error(), "OLLAMA_BASE_URL") {
		t.Fatalf("error=%v", err)
	}
}

func TestStationConvergenceDiagnosticSummaryDoesNotReportUncompiledOutputAsZeroErrors(t *testing.T) {
	if got := stationConvergenceDiagnosticSummary(worker.ExactTypeScriptConvergenceIteration{
		ArtifactError: "no required declaration",
	}); got != "not_compiled" {
		t.Fatalf("malformed artifact summary=%q", got)
	}
	if got := stationConvergenceDiagnosticSummary(worker.ExactTypeScriptConvergenceIteration{
		AfterDiagnostic: &worker.ExactTypeScriptReplayDiagnostic{Count: 4},
	}); got != "4" {
		t.Fatalf("compiled artifact summary=%q", got)
	}
}

func TestStationConvergenceReportMeasuresCompilerSetupAndModelLoop(t *testing.T) {
	started := time.Date(2026, 8, 15, 1, 0, 0, 0, time.UTC)
	finished := started.Add(7 * time.Second)
	run := newStationConvergenceReportRun(
		started, finished,
		worker.ExactTypeScriptConvergence{WallDuration: 2 * time.Second}, nil,
	)
	if run.Convergence.WallDuration != 7*time.Second {
		t.Fatalf("reported convergence duration=%s", run.Convergence.WallDuration)
	}
}
