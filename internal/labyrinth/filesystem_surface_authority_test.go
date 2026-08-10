package labyrinth

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
)

func TestFilesystemSurfaceReplayAndStaleRevisionDoNotExecuteTwice(t *testing.T) {
	t.Parallel()
	generated := generatedForFilesystemTest(t, 704)
	environment, actor, episode, started := startFilesystemTest(t, generated, "filesystem-replay")
	defer environment.Close()
	witness := generated.oracle.Witness[0]
	action := registeredFilesystemAction(t, generated.execution, actor, witness)
	accepted, err := environment.Apply(context.Background(), episode, started.Current, action)
	if err != nil {
		t.Fatal(err)
	}
	stateSHA := environment.surfaceStateSHA256()
	executions := environment.surfaceExecutionCount()
	replayed, err := environment.Apply(context.Background(), episode, started.Current, action)
	if err != nil {
		t.Fatal(err)
	}
	if canonicalJSON(accepted) != canonicalJSON(replayed) || environment.surfaceStateSHA256() != stateSHA ||
		environment.surfaceExecutionCount() != executions {
		t.Fatal("exact replay changed the transition or filesystem surface state")
	}
	staleWitness := generated.oracle.Witness[1]
	staleWitness.ID = "filesystem-stale-action"
	stale := registeredFilesystemAction(t, generated.execution, actor, staleWitness)
	if _, err := environment.Apply(context.Background(), episode, started.Current, stale); !errors.Is(err, cognition.ErrInvalidRevision) {
		t.Fatalf("stale error = %v", err)
	}
	if environment.surfaceStateSHA256() != stateSHA {
		t.Fatal("stale action changed filesystem surface state")
	}
	if environment.surfaceExecutionCount() != executions {
		t.Fatal("stale action reached filesystem execution")
	}
}

func TestFilesystemSurfaceRejectsPathEscapeBeforeAnyIO(t *testing.T) {
	t.Parallel()
	generated := generatedForFilesystemTest(t, 705)
	environment, actor, episode, started := startFilesystemTest(t, generated, "filesystem-path")
	defer environment.Close()
	canaryPath := filepath.Join(environment.rootPath(), "escape-canary")
	if err := os.WriteFile(canaryPath, []byte("unchanged"), 0o600); err != nil {
		t.Fatal(err)
	}
	witness := witnessOfKind(t, generated.oracle.Witness, "navigate")
	witness.ID = "filesystem-path-escape"
	for index := range witness.Request.Arguments {
		if witness.Request.Arguments[index].Name == fromArg {
			witness.Request.Arguments[index].Value = "../escape-canary"
		}
	}
	action := registeredFilesystemAction(t, generated.execution, actor, witness)
	before := environment.surfaceStateSHA256()
	executions := environment.surfaceExecutionCount()
	if _, err := environment.Apply(context.Background(), episode, started.Current, action); !errors.Is(err, cognition.ErrInvalidAction) {
		t.Fatalf("path escape error = %v", err)
	}
	raw, err := os.ReadFile(canaryPath)
	if err != nil || string(raw) != "unchanged" {
		t.Fatalf("path canary changed: content=%q error=%v", raw, err)
	}
	if environment.surfaceStateSHA256() != before || environment.kernel.current != started.Current {
		t.Fatal("path escape changed symbolic or filesystem state")
	}
	if environment.surfaceExecutionCount() != executions {
		t.Fatal("path escape reached filesystem execution")
	}
}

func TestFilesystemSurfacePublicSerializationOmitsRootAndHiddenState(t *testing.T) {
	t.Parallel()
	generated := generatedForFilesystemTest(t, 706)
	environment, _, _, _ := startFilesystemTest(t, generated, "filesystem-public")
	defer environment.Close()
	raw, err := json.Marshal(environment)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), environment.rootPath()) || strings.Contains(string(raw), hiddenStateCanary) ||
		strings.Contains(string(raw), "evidence.required") {
		t.Fatalf("filesystem environment leaked private state: %s", raw)
	}
}

func TestFilesystemSurfaceRejectsAnyNonV1ActionCatalog(t *testing.T) {
	t.Parallel()
	world := newTestWorld(t)
	_, err := NewFilesystemEnvironment(
		world.kernel, cognition.EpisodeRef{ID: "filesystem-wrong-catalog"},
		func(context.Context, cognition.AttemptRef) error { return nil },
	)
	if !errors.Is(err, ErrSurfaceOperation) {
		t.Fatalf("error = %v, want exact seven-action catalog rejection", err)
	}
}

func TestFilesystemSurfaceFailureIsAtomicAndReplayable(t *testing.T) {
	t.Parallel()
	generated := generatedForFilesystemTest(t, 707)
	descriptor := generated.execution.descriptor.clone()
	takeWitness := generated.oracle.Witness[1]
	object := EntityID(actionArgument(takeWitness.Request, objectArg))
	for index := range descriptor.Records {
		if descriptor.Records[index].ID == object {
			descriptor.Records[index].Location = "stage-001"
		}
	}
	scenario, err := NewScenario(
		generated.execution.ref.ID, generated.execution.definition, descriptor,
	)
	if err != nil {
		t.Fatal(err)
	}
	actor := filesystemTestActor()
	episode := cognition.EpisodeRef{ID: "filesystem-failure"}
	environment, err := NewFilesystemEnvironment(
		scenario, episode, func(_ context.Context, candidate cognition.AttemptRef) error {
			if candidate != actor {
				return cognition.ErrAuthorityDenied
			}
			return nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	defer environment.Close()
	started, err := environment.Start(context.Background(), scenario.Ref())
	if err != nil {
		t.Fatal(err)
	}
	search := registeredFilesystemAction(t, scenario, actor, generated.oracle.Witness[0])
	searched, err := environment.Apply(context.Background(), episode, started.Current, search)
	if err != nil {
		t.Fatal(err)
	}
	take := registeredFilesystemAction(t, scenario, actor, generated.oracle.Witness[1])
	beforeSHA := environment.surfaceStateSHA256()
	_, firstErr := environment.Apply(context.Background(), episode, searched.Current, take)
	if !errors.Is(firstErr, ErrSurfacePrecondition) || !errors.Is(firstErr, ErrSurfaceOperation) {
		t.Fatalf("surface failure = %v", firstErr)
	}
	var failure cognition.ActionFailure
	if !errors.As(firstErr, &failure) || failure.Code != cognition.ActionFailurePreconditionFailed {
		t.Fatalf("typed failure = %#v", failure)
	}
	if environment.kernel.current != searched.Current || environment.surfaceStateSHA256() != beforeSHA {
		t.Fatal("failed surface action changed authoritative state")
	}
	executions := environment.surfaceExecutionCount()
	_, replayErr := environment.Apply(context.Background(), episode, searched.Current, take)
	if canonicalJSON(firstErr.Error()) != canonicalJSON(replayErr.Error()) ||
		environment.surfaceExecutionCount() != executions {
		t.Fatal("failed action replay executed again or changed its exact failure")
	}
}

func witnessOfKind(t *testing.T, witness []WitnessAction, kind cognition.ActionKind) WitnessAction {
	t.Helper()
	for _, action := range witness {
		if action.Request.Kind == kind {
			return action
		}
	}
	t.Fatalf("witness has no %s action", kind)
	return WitnessAction{}
}
