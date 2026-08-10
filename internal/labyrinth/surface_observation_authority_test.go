package labyrinth

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
)

func TestEvidenceAcquisitionIsBoundedAndVisibleThroughEveryV1Surface(t *testing.T) {
	for _, config := range frozenCausalConfigs() {
		generated, err := Generate(config)
		if err != nil {
			t.Fatal(err)
		}
		for _, surface := range []string{"symbolic", "filesystem", "records"} {
			t.Run(string(config.Suite)+"/"+surface, func(t *testing.T) {
				start, acquisition, closeSurface := runAcquisitionThroughSurface(t, generated, surface)
				defer closeSurface()
				assertEvidenceAbsentBeforeAcquisition(t, start, generated.oracle.RequiredEvidence, generated.execution.descriptor)
				if len(acquisition.Observations[0].Content) > cognition.MaxObservationBytes {
					t.Fatalf("acquisition observation exceeds %d bytes", cognition.MaxObservationBytes)
				}
				assertExactAcquisitionEvidence(t, acquisition, generated.oracle.RequiredEvidence, surface)
			})
		}
	}
}

func TestSurfaceObservationNeverProjectsPrivateAdapterState(t *testing.T) {
	generated, err := Generate(frozenCausalConfigs()[4])
	if err != nil {
		t.Fatal(err)
	}
	for _, surface := range []string{"filesystem", "records"} {
		start, acquisition, closeSurface := runAcquisitionThroughSurface(t, generated, surface)
		allowedRecordIDs := publicInitialArgumentSet(generated.public.World.InitialFacts)
		for _, transition := range []cognition.Transition{start, acquisition} {
			content := transition.Observations[0].Content
			var envelope map[string]json.RawMessage
			if err := json.Unmarshal([]byte(content), &envelope); err != nil {
				t.Fatal(err)
			}
			if _, leaked := envelope["state"]; leaked {
				t.Fatalf("%s observation exposed adapter persistence state", surface)
			}
			delete(envelope, "result")
			outsideResult, err := json.Marshal(envelope)
			if err != nil {
				t.Fatal(err)
			}
			for _, record := range generated.execution.descriptor.Records {
				if strings.Contains(string(outsideResult), record.Content) ||
					strings.Contains(string(outsideResult), string(record.ID)) && !allowedRecordIDs[string(record.ID)] {
					t.Fatalf("%s leaked record %s outside bounded result", surface, record.ID)
				}
			}
		}
		closeSurface()
	}
}

func publicInitialArgumentSet(predicates []cognition.Predicate) map[string]bool {
	result := make(map[string]bool)
	for _, predicate := range predicates {
		for _, argument := range predicate.Args {
			result[argument] = true
		}
	}
	return result
}

func TestMutationSurfacesCommitTheExactEvidenceBoundValue(t *testing.T) {
	generated, err := Generate(frozenCausalConfigs()[4])
	if err != nil {
		t.Fatal(err)
	}
	written := generated.oracle.Witness[len(generated.oracle.Witness)-1].Request
	target := EntityID(actionArgument(written, mutationTargetArg))
	value := actionArgument(written, mutationValueArg)

	filesystem, transition := startFilesystemCausalTest(t, generated)
	transition = runRemainingWitness(t, filesystem, generated, transition, witnessActor)
	state, err := decodeFilesystemState(filesystem.kernel.surfaceState)
	if err != nil {
		t.Fatal(err)
	}
	document := findDocument(&state, target)
	if document == nil || document.Content != value || document.ContentSHA256 != textSHA256(value) {
		t.Fatalf("filesystem mutation=%#v want exact value %q", document, value)
	}
	_ = filesystem.Close()

	records, recordTransition := startRecordCausalTest(t, generated)
	recordTransition = runRemainingWitness(t, records, generated, recordTransition, witnessActor)
	recordState, err := decodeRecordSurfaceState(generated.execution, records.kernel.surfaceState)
	if err != nil {
		t.Fatal(err)
	}
	record := recordByID(&recordState, target)
	if record == nil || record.Content != value || record.SHA256 != textSHA256(value) {
		t.Fatalf("record mutation=%#v want exact value %q", record, value)
	}
	_ = records.Close()
}

func runAcquisitionThroughSurface(
	t *testing.T,
	generated GeneratedCase,
	surface string,
) (cognition.Transition, cognition.Transition, func()) {
	t.Helper()
	episode := cognition.EpisodeRef{ID: cognition.EpisodeID("acquire-" + string(generated.execution.ref.ID[:16]) + "-" + surface)}
	authorize := func(_ context.Context, candidate cognition.AttemptRef) error {
		if candidate != witnessActor {
			return cognition.ErrAuthorityDenied
		}
		return nil
	}
	var environment cognition.Environment
	closeSurface := func() {}
	switch surface {
	case "symbolic":
		environment, _ = NewEnvironment(generated.execution, episode, authorize)
	case "filesystem":
		value, err := NewFilesystemEnvironment(generated.execution, episode, authorize)
		if err != nil {
			t.Fatal(err)
		}
		environment, closeSurface = value, func() { _ = value.Close() }
	case "records":
		value, err := NewRecordEnvironment(generated.execution, episode, authorize)
		if err != nil {
			t.Fatal(err)
		}
		environment, closeSurface = value, func() { _ = value.Close() }
	default:
		t.Fatalf("surface %q is not registered", surface)
	}
	start, err := environment.Start(context.Background(), generated.public.Scenario)
	if err != nil {
		t.Fatal(err)
	}
	step := generated.oracle.Witness[0]
	action, err := witnessRegisteredAction(generated.execution.definition, step)
	if err != nil {
		t.Fatal(err)
	}
	acquisition, err := environment.Apply(context.Background(), episode, start.Current, action)
	if err != nil {
		t.Fatal(err)
	}
	return start, acquisition, closeSurface
}

func assertEvidenceAbsentBeforeAcquisition(
	t *testing.T,
	transition cognition.Transition,
	evidence []EvidenceIdentity,
	descriptor PublicDescriptor,
) {
	t.Helper()
	content := transition.Observations[0].Content
	records := make(map[string]PublicRecord, len(descriptor.Records))
	for _, record := range descriptor.Records {
		records[string(record.ID)] = record
	}
	for _, identity := range evidence {
		if strings.Contains(content, records[identity.ID].Content) {
			t.Fatalf("start observation exposed evidence content %s before acquisition", identity.ID)
		}
	}
}

func assertExactAcquisitionEvidence(
	t *testing.T,
	transition cognition.Transition,
	evidence []EvidenceIdentity,
	surface string,
) {
	t.Helper()
	content := transition.Observations[0].Content
	if surface != "symbolic" {
		var envelope map[string]json.RawMessage
		if err := json.Unmarshal([]byte(content), &envelope); err != nil {
			t.Fatal(err)
		}
		content = string(envelope["result"])
	}
	for _, identity := range evidence {
		if !strings.Contains(content, `"id":"`+identity.ID+`"`) ||
			!strings.Contains(content, identity.SHA256) {
			t.Fatalf("%s acquisition omitted exact evidence %#v: %s", surface, identity, content)
		}
	}
}

func startFilesystemCausalTest(t *testing.T, generated GeneratedCase) (*FilesystemEnvironment, cognition.Transition) {
	t.Helper()
	episode := cognition.EpisodeRef{ID: "exact-filesystem-mutation"}
	environment, err := NewFilesystemEnvironment(generated.execution, episode, func(_ context.Context, candidate cognition.AttemptRef) error {
		if candidate != witnessActor {
			return cognition.ErrAuthorityDenied
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	transition, err := environment.Start(context.Background(), generated.public.Scenario)
	if err != nil {
		t.Fatal(err)
	}
	return environment, transition
}

func startRecordCausalTest(t *testing.T, generated GeneratedCase) (*RecordEnvironment, cognition.Transition) {
	t.Helper()
	episode := cognition.EpisodeRef{ID: "exact-record-mutation"}
	environment, err := NewRecordEnvironment(generated.execution, episode, func(_ context.Context, candidate cognition.AttemptRef) error {
		if candidate != witnessActor {
			return cognition.ErrAuthorityDenied
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	transition, err := environment.Start(context.Background(), generated.public.Scenario)
	if err != nil {
		t.Fatal(err)
	}
	return environment, transition
}

func runRemainingWitness(
	t *testing.T,
	environment cognition.Environment,
	generated GeneratedCase,
	transition cognition.Transition,
	actor cognition.AttemptRef,
) cognition.Transition {
	t.Helper()
	episode := cognition.EpisodeRef{ID: transition.Current.EpisodeID}
	observations := append([]cognition.Observation(nil), transition.Observations...)
	for _, step := range generated.oracle.Witness {
		schema, _ := generated.execution.Catalog().Schema(step.Request.Kind)
		evidence := observationEvidenceRefs(observations)
		if schema.EvidencePolicy != cognition.EvidenceRequired {
			evidence = nil
		}
		action, err := cognition.NewRegisteredAction(step.ID, actor, schema, step.Request, evidence)
		if err != nil {
			t.Fatal(err)
		}
		transition, err = environment.Apply(context.Background(), episode, transition.Current, action)
		if err != nil {
			t.Fatal(err)
		}
		observations = append(observations, transition.Observations...)
	}
	return transition
}
