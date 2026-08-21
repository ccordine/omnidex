package assemblyline

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestPortableJobIdentityDependsOnlyOnImmutableSmallInput(t *testing.T) {
	t.Parallel()

	input := portableApplicationIntentInput(t, "Build a compact browser catalog with grouped records.")
	first, err := NewApplicationIntentJob(input)
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewApplicationIntentJob(input)
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

func TestRequirementPortableJobIdentityChangesWithIntactRequest(t *testing.T) {
	t.Parallel()
	base := portableApplicationIntentInput(t, "Build a browser catalog with grouped records.")
	first, err := NewApplicationIntentJob(base)
	if err != nil {
		t.Fatal(err)
	}
	base = portableApplicationIntentInput(t, "Build a browser catalog with grouped records and saved filters.")
	second, err := NewApplicationIntentJob(base)
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
		Capabilities:       []string{"const MIN = 0;", "const MAX = 1;"},
		CurrentDeclaration: "function clamp(value: number): number { return value; }",
		RequiredChange:     "Return a value bounded by MIN and MAX.",
		Diagnostic:         "expected 1, received 2",
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewFragmentCorrectionJob(FragmentCorrectionInput{
		Language:           "typescript",
		Signature:          "function clamp(value: number): number",
		Capabilities:       []string{"const MIN = 0;", "const MAX = 1;"},
		CurrentDeclaration: "function clamp(value: number): number { return Math.min(value, MAX); }",
		RequiredChange:     "Return a value bounded by MIN and MAX.",
		Diagnostic:         "expected 0, received -1",
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
		RequiredChange:     "Remove the rejected construct.",
		Diagnostic:         "comment nodes are forbidden",
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

	job, err := NewApplicationIntentJob(
		portableApplicationIntentInput(t, "Build a small tool with status output."),
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

func TestPortableApplicationIntentRejectsRetiredPartitionPayload(t *testing.T) {
	t.Parallel()
	payload := json.RawMessage(`{"source_text":"Enable grouped records.","mode":"extract_features"}`)
	job := PortableJob{Schema: PortableJobSchemaV1, Kind: WorkApplicationIntent, Payload: payload}
	job.ID = portableJobDigest(job.Schema, job.Kind, job.Payload)
	if err := job.Validate(); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("retired requirement payload error=%v", err)
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

func TestPortableJobRejectsRetiredContextStationKinds(t *testing.T) {
	t.Parallel()

	for _, retired := range []WorkKind{
		"conversation_context_selection",
		"memory_context_selection",
		"roleplay_narrative_continuity",
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
		Schema:  PortableJobSchemaV1,
		Kind:    WorkApplicationIntent,
		Payload: payload,
	}
	job.ID = portableJobDigest(job.Schema, job.Kind, job.Payload)
	if err := job.Validate(); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("expected unknown payload field rejection, got %v", err)
	}
}

func portableApplicationIntentInput(t *testing.T, request string) ApplicationIntentInput {
	t.Helper()
	context, err := BootstrapApplicationContext(request, ApplicationWorkspaceEmpty, nil)
	if err != nil {
		t.Fatal(err)
	}
	return ApplicationIntentInput{UserRequest: request, Context: context}
}

func TestPortableResponseCorrectionExposesOnlyOneFieldPatchAndDirectFailure(t *testing.T) {
	t.Parallel()

	original, err := NewApplicationClassificationJob(ApplicationClassificationInput{
		UserRequest: "Build a small browser tool.",
	})
	if err != nil {
		t.Fatal(err)
	}
	retained := `{"schema":"omnidex.application-class.v1","surface":"unsupported"}`
	correction, err := NewRetainedResponseCorrectionJob(
		original, "application surface is unsupported", retained,
	)
	if err != nil {
		t.Fatal(err)
	}
	if correction.ID == original.ID || correction.Kind != WorkResponseCorrection {
		t.Fatalf("correction=%#v original=%#v", correction, original)
	}
	prompt, schema, err := RenderPortableJob(correction)
	if err != nil {
		t.Fatal(err)
	}
	if schema == nil || schema["minProperties"] != 1 || schema["maxProperties"] != 1 {
		t.Fatalf("semantic correction omitted its one-field response schema: %#v", schema)
	}
	for _, required := range []string{
		"JSON merge patch", "application surface is unsupported",
		"Build a small browser tool.", retained,
	} {
		if !strings.Contains(prompt, required) {
			t.Fatalf("correction prompt omitted %q:\n%s", required, prompt)
		}
	}
	for _, forbidden := range []string{
		"workspace", "filename", "dependency graph", "agent",
	} {
		if strings.Contains(strings.ToLower(prompt), strings.ToLower(forbidden)) {
			t.Fatalf("correction prompt leaked %q:\n%s", forbidden, prompt)
		}
	}
	if _, err := NewResponseCorrectionJob(original, "missing retained response"); err == nil {
		t.Fatal("ungrounded correction constructor was accepted")
	}
	if _, err := NewRetainedResponseCorrectionJob(correction, "another failure", retained); err == nil {
		t.Fatal("nested correction chain was accepted")
	}
}

func TestSemanticResponseCorrectionChangesExactlyOneRetainedLeaf(t *testing.T) {
	t.Parallel()

	original, err := NewApplicationClassificationJob(ApplicationClassificationInput{
		UserRequest: "Build a small browser tool.",
	})
	if err != nil {
		t.Fatal(err)
	}
	retained := `{"schema":"omnidex.application-class.v1","surface":"unsupported"}`
	corrected, err := ApplyResponseCorrection(original, retained, `{"surface":"browser_application"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(corrected, `"surface":"browser_application"`) {
		t.Fatalf("corrected=%s", corrected)
	}
	for _, invalid := range []string{
		`{"surface":"unsupported"}`,
		`{"schema":"omnidex.application-class.v1"}`,
		`{"surface":"browser_application","extra":true}`,
	} {
		if _, err := ApplyResponseCorrection(original, retained, invalid); err == nil {
			t.Fatalf("accepted invalid one-field correction %s", invalid)
		}
	}
}

func TestSemanticResponseCorrectionRejectsInexactPatchAuthority(t *testing.T) {
	t.Parallel()
	original, err := NewApplicationClassificationJob(ApplicationClassificationInput{
		UserRequest: "Build a small browser tool.",
	})
	if err != nil {
		t.Fatal(err)
	}
	retained := `{"schema":"omnidex.application-class.v1","surface":"unsupported"}`
	invalid := map[string]string{
		"duplicate":  `{"surface":"unsupported","surface":"browser_application"}`,
		"markdown":   "```json\n{\"surface\":\"browser_application\"}\n```",
		"case_alias": `{"Surface":"browser_application"}`,
		"unknown":    `{"replacement":"browser_application"}`,
		"trailing":   `{"surface":"browser_application"}{"surface":"unsupported"}`,
	}
	for name, patch := range invalid {
		name, patch := name, patch
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if corrected, err := ApplyResponseCorrection(original, retained, patch); err == nil {
				t.Fatalf("accepted inexact correction patch %q as %s", patch, corrected)
			}
		})
	}
}
