package main

import (
	"strings"
	"testing"
)

func TestParseStationReplayOptionsRequiresOneFrozenPointAndReport(t *testing.T) {
	options, err := parseStationReplayOptions([]string{
		"--job", "22", "--work-kind", "fragment_correction",
		"--model", "qwen3.5:9b-q4_K_M", "--model", "deepseek-r1:8b",
		"--report", "/tmp/replay.jsonl",
	})
	if err != nil {
		t.Fatal(err)
	}
	if options.JobID != 22 || options.OpeningID != 0 || options.WorkKind != "fragment_correction" ||
		len(options.Models) != 2 || options.Timeout != 0 {
		t.Fatalf("options=%+v", options)
	}

	for name, args := range map[string][]string{
		"no frozen point":    {"--model", "qwen", "--report", "/tmp/replay.jsonl"},
		"both frozen points": {"--opening", "7", "--job", "22", "--model", "qwen", "--report", "/tmp/replay.jsonl"},
		"no model":           {"--opening", "7", "--report", "/tmp/replay.jsonl"},
		"no report":          {"--opening", "7", "--model", "qwen"},
		"duplicate model":    {"--opening", "7", "--model", "qwen", "--model", "qwen", "--report", "/tmp/replay.jsonl"},
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
