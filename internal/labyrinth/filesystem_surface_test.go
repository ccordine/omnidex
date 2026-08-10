package labyrinth

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
)

func TestFilesystemSurfaceExecutesSevenBoundedMacroActions(t *testing.T) {
	t.Parallel()
	generated := generatedForFilesystemTest(t, 701)
	actor := filesystemTestActor()
	environment, err := NewFilesystemEnvironment(
		generated.ExecutionScenario(), cognition.EpisodeRef{ID: "filesystem-seven-actions"},
		func(_ context.Context, candidate cognition.AttemptRef) error {
			if candidate != actor {
				return cognition.ErrAuthorityDenied
			}
			return nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = environment.Close() })
	transition, err := environment.Start(context.Background(), generated.public.Scenario)
	if err != nil {
		t.Fatal(err)
	}
	start := decodeFilesystemObservation(t, transition)
	if start.Surface != FilesystemSurfaceVersionV1 || start.Operation != "observe" {
		t.Fatalf("start surface = %#v", start)
	}
	seen := make(map[string]bool)
	observations := append([]cognition.Observation(nil), transition.Observations...)
	for _, witness := range generated.oracle.Witness {
		action := registeredFilesystemAction(
			t, generated.execution, actor, witness, observationEvidenceRefs(observations)...,
		)
		transition, err = environment.Apply(
			context.Background(), cognition.EpisodeRef{ID: "filesystem-seven-actions"},
			transition.Current, action,
		)
		if err != nil {
			t.Fatalf("apply %s: %v", witness.Request.Kind, err)
		}
		payload := decodeFilesystemObservation(t, transition)
		if payload.Operation != string(witness.Request.Kind) || len(payload.Result) == 0 {
			t.Fatalf("operation %s payload = %#v", witness.Request.Kind, payload)
		}
		seen[payload.Operation] = true
		observations = append(observations, transition.Observations...)
		if len(payload.Result) > MaxSurfaceResultBytes {
			t.Fatalf("operation %s exceeded result bound", payload.Operation)
		}
		if payload.Operation == "search" {
			var result struct {
				Matches []filesystemSearchMatch `json:"matches"`
			}
			if err := json.Unmarshal(payload.Result, &result); err != nil || len(result.Matches) == 0 {
				t.Fatalf("bounded rg result = %#v error=%v", result, err)
			}
			for _, match := range result.Matches {
				if textSHA256(match.Content) != match.ContentSHA256 {
					t.Fatal("rg result content is not hash-addressed")
				}
			}
		}
		if strings.Contains(string(payload.State), hiddenStateCanary) || strings.Contains(string(payload.Result), environment.rootPath()) {
			t.Fatal("filesystem observation leaked hidden state or host path")
		}
	}
	for _, kind := range v1MacroKinds {
		if !seen[string(kind)] {
			t.Fatalf("macro action %q was not exercised", kind)
		}
	}
	if !transition.Terminal {
		t.Fatal("verified filesystem witness did not reach the symbolic goal")
	}
}

func TestFilesystemWriteReportsExactContentHashes(t *testing.T) {
	t.Parallel()
	generated := generatedForFilesystemTest(t, 702)
	environment, actor, episode, transition := startFilesystemTest(t, generated, "filesystem-write")
	defer environment.Close()
	observations := append([]cognition.Observation(nil), transition.Observations...)
	for _, witness := range generated.oracle.Witness {
		action := registeredFilesystemAction(
			t, generated.execution, actor, witness, observationEvidenceRefs(observations)...,
		)
		var err error
		transition, err = environment.Apply(context.Background(), episode, transition.Current, action)
		if err != nil {
			t.Fatal(err)
		}
		observations = append(observations, transition.Observations...)
		if witness.Request.Kind != "write" {
			continue
		}
		payload := decodeFilesystemObservation(t, transition)
		var result struct {
			PreviousSHA256 string `json:"previous_sha256"`
			CurrentSHA256  string `json:"current_sha256"`
		}
		if err := json.Unmarshal(payload.Result, &result); err != nil {
			t.Fatal(err)
		}
		if !validDigest(result.PreviousSHA256) || !validDigest(result.CurrentSHA256) ||
			result.PreviousSHA256 == result.CurrentSHA256 {
			t.Fatalf("write hashes = %#v", result)
		}
	}
}

func TestFilesystemSurfaceCleanupIsExplicitAndIdempotent(t *testing.T) {
	t.Parallel()
	generated := generatedForFilesystemTest(t, 703)
	environment, err := NewFilesystemEnvironment(
		generated.ExecutionScenario(), cognition.EpisodeRef{ID: "filesystem-cleanup"},
		func(context.Context, cognition.AttemptRef) error { return nil },
	)
	if err != nil {
		t.Fatal(err)
	}
	root := environment.rootPath()
	if _, err := os.Stat(root); err != nil {
		t.Fatalf("surface root: %v", err)
	}
	if err := environment.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(root); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("closed surface root error = %v", err)
	}
	if err := environment.Close(); err != nil {
		t.Fatalf("second close: %v", err)
	}
	if _, err := environment.Start(context.Background(), generated.public.Scenario); !errors.Is(err, ErrSurfaceClosed) {
		t.Fatalf("start after close error = %v", err)
	}
}

type filesystemObservationWire struct {
	Surface   string          `json:"surface"`
	Operation string          `json:"operation"`
	State     json.RawMessage `json:"state"`
	Result    json.RawMessage `json:"result"`
}

func decodeFilesystemObservation(t *testing.T, transition cognition.Transition) filesystemObservationWire {
	t.Helper()
	if len(transition.Observations) != 1 {
		t.Fatalf("observation count = %d", len(transition.Observations))
	}
	var payload filesystemObservationWire
	if err := json.Unmarshal([]byte(transition.Observations[0].Content), &payload); err != nil {
		t.Fatalf("decode filesystem observation: %v", err)
	}
	return payload
}

func generatedForFilesystemTest(t *testing.T, seed uint64) GeneratedCase {
	t.Helper()
	config := testGeneratorConfig(SuiteCombined, seed)
	config.Difficulty.SolutionDepth = 7
	config.Difficulty.DependencyCount = 5
	generated, err := Generate(config)
	if err != nil {
		t.Fatal(err)
	}
	return generated
}

func filesystemTestActor() cognition.AttemptRef {
	return cognition.AttemptRef{JobID: 91, Generation: 2, StepID: 8, Attempt: 1, WorkerID: "filesystem-worker"}
}

func registeredFilesystemAction(
	t *testing.T,
	scenario Scenario,
	actor cognition.AttemptRef,
	witness WitnessAction,
	evidence ...cognition.EvidenceRef,
) cognition.RegisteredAction {
	t.Helper()
	schema, exists := scenario.Catalog().Schema(witness.Request.Kind)
	if !exists {
		t.Fatalf("schema %s absent", witness.Request.Kind)
	}
	if schema.EvidencePolicy != cognition.EvidenceRequired {
		evidence = nil
	}
	action, err := cognition.NewRegisteredAction(witness.ID, actor, schema, witness.Request, evidence)
	if err != nil {
		t.Fatal(err)
	}
	return action
}

func startFilesystemTest(
	t *testing.T,
	generated GeneratedCase,
	episodeID cognition.EpisodeID,
) (*FilesystemEnvironment, cognition.AttemptRef, cognition.EpisodeRef, cognition.Transition) {
	t.Helper()
	actor := filesystemTestActor()
	episode := cognition.EpisodeRef{ID: episodeID}
	environment, err := NewFilesystemEnvironment(
		generated.ExecutionScenario(), episode,
		func(_ context.Context, candidate cognition.AttemptRef) error {
			if candidate != actor {
				return cognition.ErrAuthorityDenied
			}
			return nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	transition, err := environment.Start(context.Background(), generated.public.Scenario)
	if err != nil {
		t.Fatal(err)
	}
	return environment, actor, episode, transition
}
