package assemblyline

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestPortableJobIdentityDependsOnlyOnImmutableSmallInput(t *testing.T) {
	t.Parallel()

	input := portableApplicationProductContextInput(t, "Build a compact browser catalog with grouped records.")
	first, err := NewApplicationProductContextJob(input)
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewApplicationProductContextJob(input)
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != second.ID || !reflect.DeepEqual(first, second) {
		t.Fatalf("same immutable input produced different jobs:\n%#v\n%#v", first, second)
	}

	encoded, err := json.Marshal(first)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{
		"path", "filename", "document", "queue", "lease", "attempt", "worker", "model",
	} {
		if strings.Contains(strings.ToLower(string(encoded)), forbidden) {
			t.Fatalf("portable model job leaked %q: %s", forbidden, encoded)
		}
	}
}

func TestProductContextPortableJobIdentityChangesWithIntactRequest(t *testing.T) {
	t.Parallel()
	base := portableApplicationProductContextInput(t, "Build a browser catalog with grouped records.")
	first, err := NewApplicationProductContextJob(base)
	if err != nil {
		t.Fatal(err)
	}
	base = portableApplicationProductContextInput(t, "Build a browser catalog with grouped records and saved filters.")
	second, err := NewApplicationProductContextJob(base)
	if err != nil {
		t.Fatal(err)
	}
	if first.ID == second.ID || reflect.DeepEqual(first.Payload, second.Payload) {
		t.Fatalf("request did not change immutable job identity: %s", second.ID)
	}
}

func TestPortableJobIdentityChangesWhenLocalWorkChanges(t *testing.T) {
	t.Parallel()

	first, err := NewFragmentCorrectionJob(FragmentCorrectionInput{
		Language:           "typescript",
		Signature:          "function clamp(value: number): number",
		CurrentDeclaration: "function clamp(value: number): number { return value; }",
		RepairGuidance:     "Clamp the returned value to the accepted range.",
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewFragmentCorrectionJob(FragmentCorrectionInput{
		Language:           "typescript",
		Signature:          "function clamp(value: number): number",
		CurrentDeclaration: "function clamp(value: number): number { return Math.min(value, MAX); }",
		RepairGuidance:     "Clamp the returned value to the accepted range.",
	})
	if err != nil {
		t.Fatal(err)
	}
	if first.ID == second.ID {
		t.Fatal("different local corrections produced the same content identity")
	}
}

func TestFragmentCorrectionWirePayloadCannotCarryInitialBehavior(t *testing.T) {
	t.Parallel()

	job, err := NewFragmentCorrectionJob(FragmentCorrectionInput{
		Language:           "typescript",
		Signature:          "function render(): null",
		CurrentDeclaration: "function render(): null { return null; }",
		RepairGuidance:     "Remove the rejected construct.",
	})
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(job)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"behavior", "contract", "product", "workspace", "filename"} {
		if strings.Contains(strings.ToLower(string(encoded)), forbidden) {
			t.Fatalf("correction payload carried forbidden %q field: %s", forbidden, encoded)
		}
	}
}

func TestPortableResultMustAnswerTheExactClaimedJob(t *testing.T) {
	t.Parallel()

	job, err := NewApplicationProductContextJob(
		portableApplicationProductContextInput(t, "Build a small tool with status output."),
	)
	if err != nil {
		t.Fatal(err)
	}
	result := PortableResult{JobID: strings.Repeat("0", 64), Candidate: `{"done":true}`}
	if err := result.ValidateFor(job); err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("expected mismatched result rejection, got %v", err)
	}

	result.JobID = job.ID
	if err := result.ValidateFor(job); err != nil {
		t.Fatal(err)
	}
}

func TestPortableApplicationProductContextRejectsRetiredPartitionPayload(t *testing.T) {
	t.Parallel()
	payload := json.RawMessage(`{"source_text":"Enable grouped records.","mode":"extract_features"}`)
	job := PortableJob{Schema: PortableJobSchemaV2, Kind: WorkApplicationProductContext, Payload: payload}
	job.ID = portableJobDigest(job.Schema, job.Kind, job.Payload)
	if err := job.Validate(); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("retired requirement payload error=%v", err)
	}
	if validWorkKind(WorkKind("application_intent")) {
		t.Fatal("retired aggregate application-intent work kind remains registered")
	}
}

func TestPortableJobRejectsRemovedFeaturePresencePath(t *testing.T) {
	t.Parallel()

	if validWorkKind(WorkKind("requirement_feature_presence")) {
		t.Fatal("removed feature-presence path remains registered")
	}
	if validWorkKind(WorkKind("requirement_coverage")) {
		t.Fatal("removed requirement-coverage jury remains registered")
	}
	if validWorkKind(WorkKind("requirement_quote")) {
		t.Fatal("removed iterative requirement quote selector remains registered")
	}
	if validWorkKind(WorkKind("requirement_kind")) || validWorkKind(WorkKind("requirement_outcome")) {
		t.Fatal("removed model-invented requirement mapping remains registered")
	}
}

func TestPortableJobRejectsRetiredStationKinds(t *testing.T) {
	t.Parallel()

	for _, retired := range []WorkKind{
		"conversation_context_selection",
		"memory_context_selection",
		"roleplay_narrative_continuity",
		"application_acceptance_grounding_review",
		"known_artifact_truth",
	} {
		if validWorkKind(retired) {
			t.Fatalf("retired context work kind %q remains registered", retired)
		}
		if _, err := newPortableJob(retired, map[string]string{"value": "x"}); err == nil ||
			!strings.Contains(err.Error(), "unsupported") {
			t.Fatalf("retired context work kind %q error=%v", retired, err)
		}
	}
}

func TestPortableJobRejectsUnknownWirePayloadFields(t *testing.T) {
	t.Parallel()

	payload := json.RawMessage(`{"user_request":"Build a tool.","accepted":[],"workspace":"/secret"}`)
	job := PortableJob{
		Schema:  PortableJobSchemaV2,
		Kind:    WorkApplicationProductContext,
		Payload: payload,
	}
	job.ID = portableJobDigest(job.Schema, job.Kind, job.Payload)
	if err := job.Validate(); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("expected unknown payload field rejection, got %v", err)
	}
}

func portableApplicationProductContextInput(
	t *testing.T,
	request string,
) ApplicationProductContextInput {
	t.Helper()
	context, err := BootstrapApplicationContext(request, ApplicationWorkspaceEmpty)
	if err != nil {
		t.Fatal(err)
	}
	return ApplicationProductContextInput{UserRequest: request, Context: context}
}
