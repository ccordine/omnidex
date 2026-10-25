package codex

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/codingagent"
)

func TestInstalledParserSieveCurrentCorpus(t *testing.T) {
	if os.Getenv("OMNI_TEST_PARSER_SIEVE") != "1" {
		t.Skip("OMNI_TEST_PARSER_SIEVE=1 is required")
	}
	provider, route := installedSieveProvider(t)
	var evidence *os.File
	if path := os.Getenv("OMNI_PARSER_SIEVE_EVIDENCE"); path != "" {
		var err error
		evidence, err = os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			t.Fatal(err)
		}
		defer evidence.Close()
	}
	t.Logf("live parser provider=%s model=%s effort=%s", route.Provider, route.Model, route.Effort)
	raw, err := os.ReadFile("../../testdata/parser-sieve/cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var corpus struct {
		Cases []struct {
			Name, Role, Source string
			Task               *codingagent.Task               `json:"task"`
			Clarifications     []codingagent.TaskClarification `json:"clarifications"`
			Expected           struct {
				Tasks    *int     `json:"tasks"`
				Kind     string   `json:"kind"`
				Findings *int     `json:"findings"`
				Required []string `json:"required"`
			} `json:"expected"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(raw, &corpus); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Minute)
	defer cancel()
	for _, fixture := range corpus.Cases {
		if fixture.Role == "planning_mutation" {
			continue
		}
		t.Run(fixture.Name, func(t *testing.T) {
			call := codingagent.Assignment{JobID: 17, Generation: 1, Role: codingagent.Role(fixture.Role), Root: t.TempDir(), Route: route}
			switch call.Role {
			case codingagent.PlanningInitial:
				call.PlanningResponse = fixture.Source
			case codingagent.ReviewResponse:
				call.Response = fixture.Source
				call.Task = fixture.Task
				if call.Task == nil {
					call.Task = &codingagent.Task{ID: "fixture-task"}
				}
			default:
				call.Task = fixture.Task
				call.Response = fixture.Source
				call.Clarifications = fixture.Clarifications
			}
			result, err := provider.Run(ctx, call, func(event codingagent.NativeEvent) error {
				if evidence == nil || len(event.Frame) == 0 {
					return nil
				}
				record := map[string]any{"case": fixture.Name, "route": route, "frame": event.Frame}
				envelope, err := sieveBenchmarkEnvelopeFor(call, event.Frame)
				if err != nil {
					return err
				}
				if envelope != nil {
					record["code_rendered_envelope"] = envelope
				}
				return json.NewEncoder(evidence).Encode(record)
			})
			if err != nil {
				t.Fatal(err)
			}
			switch call.Role {
			case codingagent.PlanningInitial:
				var parsed struct {
					Ledger codingagent.EngineeringLedger `json:"ledger"`
				}
				if err := json.Unmarshal(result.Content, &parsed); err != nil {
					t.Fatal(err)
				}
				if err := parsed.Ledger.Validate(); err != nil {
					t.Fatal(err)
				}
				if fixture.Expected.Tasks != nil && len(parsed.Ledger.Tasks) != *fixture.Expected.Tasks {
					t.Fatalf("tasks=%d, want %d", len(parsed.Ledger.Tasks), *fixture.Expected.Tasks)
				}
				encoded, _ := json.Marshal(parsed.Ledger)
				for _, required := range fixture.Expected.Required {
					if !strings.Contains(string(encoded), required) {
						t.Fatalf("ledger lost required text %q", required)
					}
				}
			case codingagent.ExecutionResponse:
				var parsed struct {
					Kind string `json:"kind"`
				}
				if err := json.Unmarshal(result.Content, &parsed); err != nil || parsed.Kind != fixture.Expected.Kind {
					t.Fatalf("kind=%s, want %s: %s", parsed.Kind, fixture.Expected.Kind, result.Content)
				}
			case codingagent.ReviewResponse:
				var parsed codingagent.ReviewResponseResult
				if err := json.Unmarshal(result.Content, &parsed); err != nil {
					t.Fatal(err)
				}
				if parsed.Verification != fixture.Source {
					t.Fatal("review source evidence changed")
				}
				if string(parsed.Kind) != fixture.Expected.Kind {
					t.Fatalf("kind=%s, want %s", parsed.Kind, fixture.Expected.Kind)
				}
				if fixture.Expected.Findings != nil && len(parsed.Findings) != *fixture.Expected.Findings {
					t.Fatalf("findings=%d, want %d", len(parsed.Findings), *fixture.Expected.Findings)
				}
			}
		})
	}
}
