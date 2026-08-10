package labyrinth

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
)

func TestRecordSurfaceExecutesEveryRegisteredMacroThroughTheSymbolicKernel(t *testing.T) {
	t.Parallel()
	generated := generatedForRecordSurfaceTest(t, 801)
	environment, actor, episode, transition := startRecordSurfaceTest(t, generated, "record-seven-actions")
	defer environment.Close()

	started := decodeRecordSurfaceObservation(t, transition)
	if started.Surface != RecordSurfaceVersionV1 || started.Operation != "observe" {
		t.Fatalf("start observation = %#v", started)
	}
	seen := make(map[string]bool, len(v1MacroKinds))
	observations := append([]cognition.Observation(nil), transition.Observations...)
	for _, witness := range generated.oracle.Witness {
		action := registeredRecordAction(
			t, generated.execution, actor, witness, observationEvidenceRefs(observations)...,
		)
		previous := transition.Current
		var err error
		transition, err = environment.Apply(context.Background(), episode, previous, action)
		if err != nil {
			t.Fatalf("apply %s: %v", witness.Request.Kind, err)
		}
		if err := transition.ValidateApply(episode, previous, action); err != nil {
			t.Fatalf("validate %s transition: %v", witness.Request.Kind, err)
		}
		payload := decodeRecordSurfaceObservation(t, transition)
		if payload.Operation != string(witness.Request.Kind) {
			t.Fatalf("operation = %q, want %q", payload.Operation, witness.Request.Kind)
		}
		result := decodeRecordSurfaceResult(t, payload)
		if result.Collection == "" || result.Format != recordSurfaceResultFormatV1 {
			t.Fatalf("operation %s result = %#v", witness.Request.Kind, result)
		}
		switch witness.Request.Kind {
		case "search", "read":
			if len(result.Records) == 0 || result.Records[0].Content == "" ||
				!validDigest(result.Records[0].ContentSHA256) {
				t.Fatalf("%s did not expose a bounded hash-bound record: %#v", witness.Request.Kind, result)
			}
		case "write":
			if !validDigest(result.PreviousSHA256) || !validDigest(result.CurrentSHA256) ||
				result.PreviousSHA256 == result.CurrentSHA256 {
				t.Fatalf("write did not report exact changed hashes: %#v", result)
			}
		}
		seen[payload.Operation] = true
		observations = append(observations, transition.Observations...)
	}
	for _, kind := range v1MacroKinds {
		if !seen[string(kind)] {
			t.Fatalf("macro action %q was not exercised", kind)
		}
	}
	if !transition.Terminal {
		t.Fatal("record-surface witness did not reach the symbolic terminal predicate")
	}
}

func TestRecordSurfaceExactReplayAndStaleRevisionRemainKernelAuthoritative(t *testing.T) {
	t.Parallel()
	generated := generatedForRecordSurfaceTest(t, 802)
	environment, actor, episode, started := startRecordSurfaceTest(t, generated, "record-replay")
	defer environment.Close()

	witness := generated.oracle.Witness[0]
	action := registeredRecordAction(t, generated.execution, actor, witness)
	accepted, err := environment.Apply(context.Background(), episode, started.Current, action)
	if err != nil {
		t.Fatal(err)
	}
	acceptedSurfaceSHA := environment.kernel.surfaceState.StateSHA256
	replayed, err := environment.Apply(context.Background(), episode, started.Current, action)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(accepted, replayed) || environment.kernel.surfaceState.StateSHA256 != acceptedSurfaceSHA {
		t.Fatalf("exact replay changed transition:\naccepted=%#v\nreplayed=%#v", accepted, replayed)
	}

	staleWitness := generated.oracle.Witness[1]
	staleWitness.ID = "record-stale-action"
	stale := registeredRecordAction(t, generated.execution, actor, staleWitness)
	if _, err := environment.Apply(context.Background(), episode, started.Current, stale); !errors.Is(err, cognition.ErrInvalidRevision) {
		t.Fatalf("stale error = %v, want ErrInvalidRevision", err)
	}
	if environment.kernel.current != accepted.Current || environment.kernel.surfaceState.StateSHA256 != acceptedSurfaceSHA {
		t.Fatal("stale action changed symbolic or record surface authority")
	}
}

func TestRecordSurfaceIsDistinctWhileSharingTheExactScenarioAndCatalog(t *testing.T) {
	t.Parallel()
	generated := generatedForRecordSurfaceTest(t, 805)
	scenario := generated.ExecutionScenario()
	episode := cognition.EpisodeRef{ID: "record-transfer"}
	authorize := func(context.Context, cognition.AttemptRef) error { return nil }
	records, err := NewRecordEnvironment(scenario, episode, authorize)
	if err != nil {
		t.Fatal(err)
	}
	defer records.Close()
	symbolic, err := NewEnvironment(scenario, episode, authorize)
	if err != nil {
		t.Fatal(err)
	}
	recordStart, err := records.Start(context.Background(), scenario.Ref())
	if err != nil {
		t.Fatal(err)
	}
	symbolicStart, err := symbolic.Start(context.Background(), scenario.Ref())
	if err != nil {
		t.Fatal(err)
	}
	if records.kernel.scenario.Ref() != symbolic.scenario.Ref() ||
		records.kernel.scenario.Catalog().SHA256 != symbolic.scenario.Catalog().SHA256 {
		t.Fatal("surface selection changed the sealed scenario or action catalog")
	}
	recordContent := recordStart.Observations[0].Content
	symbolicContent := symbolicStart.Observations[0].Content
	if recordContent == symbolicContent || !strings.Contains(recordContent, RecordSurfaceVersionV1) ||
		strings.Contains(symbolicContent, RecordSurfaceVersionV1) {
		t.Fatal("record and symbolic surfaces are not structurally distinct")
	}
}

func TestRecordSurfaceRunsTheVerifiedWitnessForEveryV1Suite(t *testing.T) {
	t.Parallel()
	suites := []Suite{SuiteRetrieve, SuiteRecall, SuiteUnlock, SuiteMutate, SuiteCombined}
	for index, suite := range suites {
		config := testGeneratorConfig(suite, uint64(900+index))
		generated, err := Generate(config)
		if err != nil {
			t.Fatalf("generate %s: %v", suite, err)
		}
		environment, actor, episode, transition := startRecordSurfaceTest(
			t, generated, cognition.EpisodeID("record-suite-"+string(suite)),
		)
		observations := append([]cognition.Observation(nil), transition.Observations...)
		for _, witness := range generated.oracle.Witness {
			action := registeredRecordAction(
				t, generated.execution, actor, witness, observationEvidenceRefs(observations)...,
			)
			transition, err = environment.Apply(context.Background(), episode, transition.Current, action)
			if err != nil {
				t.Fatalf("suite %s action %s: %v", suite, witness.Request.Kind, err)
			}
			observations = append(observations, transition.Observations...)
		}
		if !transition.Terminal {
			t.Fatalf("suite %s witness did not terminate", suite)
		}
		if err := environment.Close(); err != nil {
			t.Fatalf("close %s: %v", suite, err)
		}
	}
}

func TestRecordSurfaceFailedActionHasNoRecordOrSymbolicEffect(t *testing.T) {
	t.Parallel()
	generated := generatedForRecordSurfaceTest(t, 803)
	environment, actor, episode, started := startRecordSurfaceTest(t, generated, "record-failure")
	defer environment.Close()

	invalidWitness := generated.oracle.Witness[1]
	invalidWitness.ID = "record-invalid-precondition"
	invalidWitness.Request = invalidWitness.Request.Clone()
	for index := range invalidWitness.Request.Arguments {
		if invalidWitness.Request.Arguments[index].Name == objectArg {
			invalidWitness.Request.Arguments[index].Value = actionArgument(
				generated.oracle.Witness[len(generated.oracle.Witness)-1].Request, mutationTargetArg,
			)
		}
	}
	invalid := registeredRecordAction(t, generated.execution, actor, invalidWitness)
	beforeSurface := environment.kernel.surfaceState.StateSHA256
	if _, err := environment.Apply(context.Background(), episode, started.Current, invalid); !errors.Is(err, ErrPrecondition) {
		t.Fatalf("invalid action error = %v, want ErrPrecondition", err)
	}
	if environment.kernel.current != started.Current || environment.kernel.surfaceState.StateSHA256 != beforeSurface {
		t.Fatal("failed action changed symbolic or record surface state")
	}

	validWitness := generated.oracle.Witness[0]
	validWitness.ID = "record-valid-after-failure"
	valid := registeredRecordAction(t, generated.execution, actor, validWitness)
	if _, err := environment.Apply(context.Background(), episode, started.Current, valid); err != nil {
		t.Fatalf("valid action after rejected action: %v", err)
	}
}

func TestRecordSurfaceSerializationExcludesPrivateStateAndAdapterInternals(t *testing.T) {
	t.Parallel()
	generated := generatedForRecordSurfaceTest(t, 804)
	environment, _, _, started := startRecordSurfaceTest(t, generated, "record-public")
	defer environment.Close()

	rawEnvironment, err := json.Marshal(environment)
	if err != nil {
		t.Fatal(err)
	}
	rawTransition, err := json.Marshal(started)
	if err != nil {
		t.Fatal(err)
	}
	public := strings.ToLower(string(append(rawEnvironment, rawTransition...)))
	for _, forbidden := range []string{
		"\"seed\"", "oracle", "evidence.required", "evidence.distractor",
		"state.current", "state.completed", "permit.", "state_sha256", "private_state",
	} {
		if strings.Contains(public, forbidden) {
			t.Fatalf("record surface serialization contains private term %q: %s", forbidden, public)
		}
	}
	payload := decodeRecordSurfaceObservation(t, started)
	if strings.Contains(string(payload.Result), "predicates") {
		t.Fatalf("record result reused the symbolic predicate representation: %s", payload.Result)
	}
}

func TestRecordSurfaceRejectsAnyNonV1ActionCatalog(t *testing.T) {
	t.Parallel()
	world := newTestWorld(t)
	_, err := NewRecordEnvironment(
		world.kernel, cognition.EpisodeRef{ID: "record-wrong-catalog"},
		func(context.Context, cognition.AttemptRef) error { return nil },
	)
	if !errors.Is(err, ErrSurfaceOperation) {
		t.Fatalf("error = %v, want exact seven-action catalog rejection", err)
	}
}

type recordSurfaceObservationWire struct {
	Surface   string          `json:"surface"`
	Operation string          `json:"operation"`
	State     json.RawMessage `json:"state"`
	Result    json.RawMessage `json:"result"`
}

func decodeRecordSurfaceObservation(t *testing.T, transition cognition.Transition) recordSurfaceObservationWire {
	t.Helper()
	if len(transition.Observations) != 1 {
		t.Fatalf("observation count = %d", len(transition.Observations))
	}
	var payload recordSurfaceObservationWire
	if err := json.Unmarshal([]byte(transition.Observations[0].Content), &payload); err != nil {
		t.Fatalf("decode record observation: %v", err)
	}
	return payload
}

func decodeRecordSurfaceResult(t *testing.T, observation recordSurfaceObservationWire) recordSurfaceResult {
	t.Helper()
	var result recordSurfaceResult
	if err := json.Unmarshal(observation.Result, &result); err != nil {
		t.Fatalf("decode record result: %v", err)
	}
	return result
}
