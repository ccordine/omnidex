package labyrinth

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
)

type closableCognitionEnvironment interface {
	cognition.Environment
	Close() error
}

type transferSurfaceFactory struct {
	name    string
	version string
	open    func(Scenario, cognition.EpisodeRef, AttemptAuthorizer) (closableCognitionEnvironment, error)
}

type transferSurfaceRun struct {
	name        string
	version     string
	transitions []cognition.Transition
}

func TestSurfaceAdaptersTransferOneLatentCaseThroughTheSameEnvironmentContract(t *testing.T) {
	t.Parallel()
	generated := generatedForFilesystemTest(t, 910)
	scenario := generated.ExecutionScenario()
	episode := cognition.EpisodeRef{ID: "surface-transfer"}
	actor := cognition.AttemptRef{
		JobID: 93, Generation: 3, StepID: 9, Attempt: 1, WorkerID: "surface-transfer-worker",
	}
	authorize := func(_ context.Context, candidate cognition.AttemptRef) error {
		if candidate != actor {
			return cognition.ErrAuthorityDenied
		}
		return nil
	}
	factories := []transferSurfaceFactory{
		{
			name: "filesystem", version: FilesystemSurfaceVersionV1,
			open: func(s Scenario, e cognition.EpisodeRef, a AttemptAuthorizer) (closableCognitionEnvironment, error) {
				return NewFilesystemEnvironment(s, e, a)
			},
		},
		{
			name: "records", version: RecordSurfaceVersionV1,
			open: func(s Scenario, e cognition.EpisodeRef, a AttemptAuthorizer) (closableCognitionEnvironment, error) {
				return NewRecordEnvironment(s, e, a)
			},
		},
	}

	runs := make([]transferSurfaceRun, 0, len(factories))
	for _, factory := range factories {
		runs = append(runs, runTransferSurface(t, factory, scenario, generated.oracle.Witness, episode, actor, authorize))
	}
	if len(runs) != 2 {
		t.Fatalf("surface run count = %d, want 2", len(runs))
	}
	left, right := runs[0], runs[1]
	if len(left.transitions) != len(right.transitions) {
		t.Fatalf("transition counts differ: %d != %d", len(left.transitions), len(right.transitions))
	}
	if left.transitions[0].Current == right.transitions[0].Current {
		t.Fatal("structurally distinct surface state did not produce distinct revision authority")
	}
	for index := 1; index < len(left.transitions); index++ {
		leftTransition, rightTransition := left.transitions[index], right.transitions[index]
		if leftTransition.ActionID != rightTransition.ActionID ||
			leftTransition.Cost != rightTransition.Cost ||
			leftTransition.Terminal != rightTransition.Terminal ||
			leftTransition.PublicOutcome != rightTransition.PublicOutcome ||
			leftTransition.Effects[0].Kind != rightTransition.Effects[0].Kind ||
			leftTransition.Effects[0].Content != rightTransition.Effects[0].Content {
			t.Fatalf("surface-independent semantics differ at transition %d", index)
		}
	}
	if !left.transitions[len(left.transitions)-1].Terminal || !right.transitions[len(right.transitions)-1].Terminal {
		t.Fatal("the shared verified witness did not terminate through both surfaces")
	}
}

func runTransferSurface(
	t *testing.T,
	factory transferSurfaceFactory,
	scenario Scenario,
	witness []WitnessAction,
	episode cognition.EpisodeRef,
	actor cognition.AttemptRef,
	authorize AttemptAuthorizer,
) transferSurfaceRun {
	t.Helper()
	environment, err := factory.open(scenario, episode, authorize)
	if err != nil {
		t.Fatalf("open %s surface: %v", factory.name, err)
	}
	t.Cleanup(func() {
		if err := environment.Close(); err != nil {
			t.Errorf("close %s surface: %v", factory.name, err)
		}
	})
	transition, err := environment.Start(context.Background(), scenario.Ref())
	if err != nil {
		t.Fatalf("start %s surface: %v", factory.name, err)
	}
	if err := transition.ValidateStart(); err != nil {
		t.Fatalf("validate %s start: %v", factory.name, err)
	}
	assertTransferSurfaceObservation(t, transition, factory.version)
	transitions := []cognition.Transition{transition}
	observations := append([]cognition.Observation(nil), transition.Observations...)
	for _, step := range witness {
		schema, exists := scenario.Catalog().Schema(step.Request.Kind)
		if !exists {
			t.Fatalf("schema %s is absent", step.Request.Kind)
		}
		evidence := observationEvidenceRefs(observations)
		if schema.EvidencePolicy != cognition.EvidenceRequired {
			evidence = nil
		}
		action, err := cognition.NewRegisteredAction(step.ID, actor, schema, step.Request, evidence)
		if err != nil {
			t.Fatalf("register shared action %s: %v", step.ID, err)
		}
		previous := transition.Current
		transition, err = environment.Apply(context.Background(), episode, previous, action)
		if err != nil {
			t.Fatalf("apply %s through %s surface: %v", step.ID, factory.name, err)
		}
		if err := transition.ValidateApply(episode, previous, action); err != nil {
			t.Fatalf("validate %s transition: %v", factory.name, err)
		}
		assertTransferSurfaceObservation(t, transition, factory.version)
		transitions = append(transitions, transition)
		observations = append(observations, transition.Observations...)
	}
	return transferSurfaceRun{factory.name, factory.version, transitions}
}

func assertTransferSurfaceObservation(t *testing.T, transition cognition.Transition, version string) {
	t.Helper()
	if len(transition.Observations) != 1 {
		t.Fatalf("observation count = %d, want 1", len(transition.Observations))
	}
	var payload struct {
		Surface          string          `json:"surface"`
		State            json.RawMessage `json:"state"`
		SymbolicState    json.RawMessage `json:"symbolic_state"`
		SurfaceAuthority string          `json:"surface_authority"`
		Result           json.RawMessage `json:"result"`
	}
	content := transition.Observations[0].Content
	if err := json.Unmarshal([]byte(content), &payload); err != nil {
		t.Fatalf("decode %s observation: %v", version, err)
	}
	if payload.Surface != version || len(payload.State) != 0 || len(payload.SymbolicState) == 0 ||
		!validDigest(payload.SurfaceAuthority) || len(payload.Result) == 0 {
		t.Fatalf("surface observation is incomplete: %#v", payload)
	}
	for _, forbidden := range []string{"state.current", "evidence.required", "oracle", "private_state"} {
		if strings.Contains(strings.ToLower(content), forbidden) {
			t.Fatalf("surface observation contains private identity %q", forbidden)
		}
	}
}
