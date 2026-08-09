package assemblyline

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRequirementFinalAdvisoryBindsCompletedCandidateAndMemo(t *testing.T) {
	t.Parallel()

	source := "Build a catalog with grouped records and a saved filter."
	candidate := RequirementPartitionDecision{
		Schema: RequirementPartitionSchemaV1, FeatureQuotes: []string{"grouped records", "a saved filter"},
	}
	subject, err := NewRequirementFinalSubject(source, candidate)
	if err != nil {
		t.Fatal(err)
	}
	if len(subject.DirectCandidateSHA256) != 64 || len(subject.SubjectSHA256) != 64 {
		t.Fatalf("subject hashes=%#v", subject)
	}

	advisoryJob, err := NewRequirementFinalAdvisoryJob(subject)
	if err != nil {
		t.Fatal(err)
	}
	advisoryPrompt, advisorySchema, err := RenderPortableJob(advisoryJob)
	if err != nil {
		t.Fatal(err)
	}
	if advisorySchema != nil {
		t.Fatalf("advisory schema=%#v", advisorySchema)
	}
	for _, required := range []string{
		"plain text", subject.SubjectSHA256, subject.DirectCandidateSHA256,
		"thinking-only response is invalid", "FEATURE_001", "grouped records", source,
	} {
		if !strings.Contains(advisoryPrompt, required) {
			t.Fatalf("advisory prompt omitted %q:\n%s", required, advisoryPrompt)
		}
	}
	if strings.Contains(advisoryPrompt, "RESPONSE_SCHEMA_JSON") {
		t.Fatalf("advisory prompt leaked a target response schema:\n%s", advisoryPrompt)
	}

	memo := "FEATURE_001 and FEATURE_002 are distinct and cover both explicit requested behaviors."
	synthesisJob, err := NewRequirementFinalSynthesisJob(subject, advisoryJob, memo)
	if err != nil {
		t.Fatal(err)
	}
	synthesisPrompt, synthesisSchema, err := RenderPortableJob(synthesisJob)
	if err != nil {
		t.Fatal(err)
	}
	if len(synthesisSchema) == 0 {
		t.Fatal("synthesis response schema is missing")
	}
	for _, required := range []string{
		"original source is authoritative", "untrusted", advisoryJob.ID,
		subject.SubjectSHA256, memo,
	} {
		if !strings.Contains(synthesisPrompt, required) {
			t.Fatalf("synthesis prompt omitted %q:\n%s", required, synthesisPrompt)
		}
	}
}

func TestRequirementFinalAdvisoryRejectsBrokenBindings(t *testing.T) {
	t.Parallel()

	source := "Build a timer with lap history and keyboard controls."
	candidate := RequirementPartitionDecision{
		Schema: RequirementPartitionSchemaV1, FeatureQuotes: []string{"lap history", "keyboard controls"},
	}
	subject, err := NewRequirementFinalSubject(source, candidate)
	if err != nil {
		t.Fatal(err)
	}
	advisoryJob, err := NewRequirementFinalAdvisoryJob(subject)
	if err != nil {
		t.Fatal(err)
	}

	tamperedSubject := subject
	tamperedSubject.DirectCandidate.FeatureQuotes = append([]string(nil), subject.DirectCandidate.FeatureQuotes...)
	tamperedSubject.DirectCandidate.FeatureQuotes[0] = "timer"
	if _, err := NewRequirementFinalAdvisoryJob(tamperedSubject); err == nil || !strings.Contains(err.Error(), "candidate hash") {
		t.Fatalf("tampered candidate error=%v", err)
	}

	input := RequirementFinalSynthesisInput{
		Subject: subject, AdvisoryJobID: advisoryJob.ID,
		AdvisoryMemo: "Check both feature spans.", AdvisoryMemoSHA256: strings.Repeat("0", 64),
	}
	if _, err := newValidatedPortableJob(WorkRequirementFinalSynthesis, input, input.validate); err == nil || !strings.Contains(err.Error(), "memo hash") {
		t.Fatalf("tampered memo hash error=%v", err)
	}

	input.AdvisoryMemoSHA256 = requirementFinalTextDigest(input.AdvisoryMemo)
	input.AdvisoryJobID = strings.Repeat("f", 64)
	if _, err := newValidatedPortableJob(WorkRequirementFinalSynthesis, input, input.validate); err == nil || !strings.Contains(err.Error(), "advisory job") {
		t.Fatalf("tampered advisory job error=%v", err)
	}
}

func TestCompleteRequirementPartitionUsesResidualAndGraphValidation(t *testing.T) {
	t.Parallel()

	source := "Build a board with drag controls and due dates."
	valid := RequirementPartitionDecision{
		Schema: RequirementPartitionSchemaV1, FeatureQuotes: []string{"drag controls", "due dates"},
	}
	if err := ValidateCompleteRequirementPartition(source, valid); err != nil {
		t.Fatal(err)
	}
	empty := RequirementPartitionDecision{Schema: RequirementPartitionSchemaV1, FeatureQuotes: []string{}}
	if err := ValidateCompleteRequirementPartition(source, empty); err == nil || !strings.Contains(err.Error(), "at least one") {
		t.Fatalf("empty partition error=%v", err)
	}
}

func TestRequirementFinalPortablePayloadRejectsUnknownFields(t *testing.T) {
	t.Parallel()

	payload := json.RawMessage(`{"subject":{"protocol":"x"},"invented":true}`)
	job := PortableJob{Schema: PortableJobSchemaV1, Kind: WorkRequirementFinalAdvisory, Payload: payload}
	job.ID = portableJobDigest(job.Schema, job.Kind, job.Payload)
	if err := job.Validate(); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("unknown field error=%v", err)
	}
}
