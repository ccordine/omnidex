package cognition

import (
	"errors"
	"strings"
	"testing"
)

func testActionSchema(t *testing.T, policy EvidencePolicy) ActionSchema {
	t.Helper()
	schema, err := NewActionSchema(
		"catalog.inspect.v1",
		"1.0.0",
		"inspect",
		[]ActionParameterSpec{{Name: "target", Required: true, MaxBytes: 128}},
		policy,
	)
	if err != nil {
		t.Fatalf("new action schema: %v", err)
	}
	return schema
}

func testAttemptRef() AttemptRef {
	return AttemptRef{JobID: 7, Generation: 2, StepID: 11, Attempt: 1, WorkerID: "worker-1"}
}

func testEvidenceRef(t *testing.T) EvidenceRef {
	t.Helper()
	observation, err := NewObservation("observation-1", testRevision(1), "record", "source")
	if err != nil {
		t.Fatalf("new observation: %v", err)
	}
	return observation.EvidenceRef()
}

func TestActionSchemaIsContentAddressedAndValidatesRequest(t *testing.T) {
	t.Parallel()
	schema := testActionSchema(t, EvidenceRequired)
	if err := schema.Validate(); err != nil {
		t.Fatalf("validate schema: %v", err)
	}
	request := ActionRequest{
		Kind:      "inspect",
		Arguments: []ActionArgument{{Name: "target", Value: "entity-1"}},
	}
	if err := schema.ValidateRequest(request, []EvidenceRef{testEvidenceRef(t)}); err != nil {
		t.Fatalf("validate request: %v", err)
	}

	mutated := schema
	mutated.Parameters = append([]ActionParameterSpec(nil), schema.Parameters...)
	mutated.Parameters[0].MaxBytes++
	if err := mutated.Validate(); !errors.Is(err, ErrInvalidActionSchema) {
		t.Fatalf("mutated schema error = %v, want ErrInvalidActionSchema", err)
	}

	for name, request := range map[string]ActionRequest{
		"wrong kind":       {Kind: "write", Arguments: []ActionArgument{{Name: "target", Value: "entity-1"}}},
		"missing required": {Kind: "inspect"},
		"unknown argument": {Kind: "inspect", Arguments: []ActionArgument{{Name: "other", Value: "value"}}},
		"duplicate": {Kind: "inspect", Arguments: []ActionArgument{
			{Name: "target", Value: "one"}, {Name: "target", Value: "two"},
		}},
		"NUL value": {Kind: "inspect", Arguments: []ActionArgument{{Name: "target", Value: "x\x00y"}}},
		"oversized": {Kind: "inspect", Arguments: []ActionArgument{{Name: "target", Value: strings.Repeat("x", 129)}}},
	} {
		request := request
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if err := schema.ValidateRequest(request, []EvidenceRef{testEvidenceRef(t)}); !errors.Is(err, ErrInvalidAction) {
				t.Fatalf("error = %v, want ErrInvalidAction", err)
			}
		})
	}

	if err := schema.ValidateRequest(request, nil); !errors.Is(err, ErrInvalidEvidence) {
		t.Fatalf("missing evidence error = %v, want ErrInvalidEvidence", err)
	}
	forbidden := testActionSchema(t, EvidenceForbidden)
	if err := forbidden.ValidateRequest(request, []EvidenceRef{testEvidenceRef(t)}); !errors.Is(err, ErrInvalidEvidence) {
		t.Fatalf("forbidden evidence error = %v, want ErrInvalidEvidence", err)
	}
}

func TestRegisteredActionHasOneCodeOwnedIdempotencyIdentity(t *testing.T) {
	t.Parallel()
	schema := testActionSchema(t, EvidenceRequired)
	request := ActionRequest{Kind: "inspect", Arguments: []ActionArgument{{Name: "target", Value: "entity-1"}}}
	evidence := []EvidenceRef{testEvidenceRef(t)}
	action, err := NewRegisteredAction("action-1", testAttemptRef(), schema, request, evidence)
	if err != nil {
		t.Fatalf("new registered action: %v", err)
	}
	request.Arguments[0].Value = "mutated"
	evidence[0].SHA256 = strings.Repeat("b", 64)
	if action.Request.Arguments[0].Value != "entity-1" || action.EvidenceRefs[0].SHA256 != testEvidenceRef(t).SHA256 {
		t.Fatal("registered action retained caller-owned slices")
	}
	if err := action.Validate(schema); err != nil {
		t.Fatalf("validate registered action: %v", err)
	}
	if action.Schema != schema.Ref() {
		t.Fatalf("schema reference = %#v, want %#v", action.Schema, schema.Ref())
	}

	missingID := action
	missingID.ID = ""
	if err := missingID.Validate(schema); !errors.Is(err, ErrInvalidAction) {
		t.Fatalf("missing action ID error = %v, want ErrInvalidAction", err)
	}
	wrongSchema := action
	wrongSchema.Schema.SHA256 = strings.Repeat("b", 64)
	if err := wrongSchema.Validate(schema); !errors.Is(err, ErrInvalidAction) {
		t.Fatalf("wrong schema error = %v, want ErrInvalidAction", err)
	}
}

func TestActionEvidenceReferencesAreBoundedUniqueAndEpisodeLocal(t *testing.T) {
	t.Parallel()
	schema := testActionSchema(t, EvidenceRequired)
	request := ActionRequest{Kind: "inspect", Arguments: []ActionArgument{{Name: "target", Value: "entity-1"}}}
	first := testEvidenceRef(t)
	if err := schema.ValidateRequest(request, []EvidenceRef{first, first}); !errors.Is(err, ErrInvalidEvidence) {
		t.Fatalf("duplicate evidence error = %v, want ErrInvalidEvidence", err)
	}
	secondEpisode := first
	secondEpisode.ObservationID = "observation-2"
	secondEpisode.Revision.EpisodeID = "episode-2"
	if err := schema.ValidateRequest(request, []EvidenceRef{first, secondEpisode}); !errors.Is(err, ErrInvalidEvidence) {
		t.Fatalf("cross-episode evidence error = %v, want ErrInvalidEvidence", err)
	}
	overflow := make([]EvidenceRef, MaxEvidenceRefs+1)
	if err := schema.ValidateRequest(request, overflow); !errors.Is(err, ErrInvalidEvidence) {
		t.Fatalf("evidence overflow error = %v, want ErrInvalidEvidence", err)
	}
}

func TestActionCatalogIsVersionedSortedAndContentAddressed(t *testing.T) {
	t.Parallel()
	inspect := testActionSchema(t, EvidenceOptional)
	write, err := NewActionSchema("catalog.write.v1", "1.0.0", "write", nil, EvidenceRequired)
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := NewActionCatalog("catalog.default", "1.0.0", []ActionSchema{write, inspect})
	if err != nil {
		t.Fatalf("new action catalog: %v", err)
	}
	if catalog.Schemas[0].Kind != "inspect" || catalog.Schemas[1].Kind != "write" {
		t.Fatalf("catalog schemas are not canonical: %#v", catalog.Schemas)
	}
	if err := catalog.Validate(); err != nil {
		t.Fatalf("validate catalog: %v", err)
	}
	mutated := catalog.Clone()
	mutated.Version = "1.0.1"
	if err := mutated.Validate(); !errors.Is(err, ErrInvalidActionCatalog) {
		t.Fatalf("mutated catalog error = %v, want ErrInvalidActionCatalog", err)
	}
	duplicateKind, err := NewActionSchema("catalog.inspect.v2", "2.0.0", "inspect", nil, EvidenceOptional)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewActionCatalog("catalog.default", "1.0.0", []ActionSchema{inspect, duplicateKind}); !errors.Is(err, ErrInvalidActionCatalog) {
		t.Fatalf("duplicate kind error = %v, want ErrInvalidActionCatalog", err)
	}
}

func TestAttemptReferenceFailsLoudly(t *testing.T) {
	t.Parallel()
	if err := testAttemptRef().Validate(); err != nil {
		t.Fatalf("validate attempt: %v", err)
	}
	for name, actor := range map[string]AttemptRef{
		"job":        {Generation: 1, StepID: 1, Attempt: 1, WorkerID: "worker-1"},
		"generation": {JobID: 1, StepID: 1, Attempt: 1, WorkerID: "worker-1"},
		"step":       {JobID: 1, Generation: 1, Attempt: 1, WorkerID: "worker-1"},
		"attempt":    {JobID: 1, Generation: 1, StepID: 1, WorkerID: "worker-1"},
		"worker":     {JobID: 1, Generation: 1, StepID: 1, Attempt: 1, WorkerID: "bad worker"},
	} {
		actor := actor
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if err := actor.Validate(); !errors.Is(err, ErrInvalidAttempt) {
				t.Fatalf("error = %v, want ErrInvalidAttempt", err)
			}
		})
	}
}
