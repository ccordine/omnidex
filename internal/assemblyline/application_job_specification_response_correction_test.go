package assemblyline

import (
	"strings"
	"testing"
)

func TestApplicationJobSpecificationResponseCorrectionOwnsOnlyCurrentInvalidField(t *testing.T) {
	t.Parallel()
	job := applicationJobSpecificationCorrectionTestJob(t)
	retained := `{"objective":"Implement inventory filters.","required_behaviors":["Filter inventory.","Filter inventory."],"acceptance_criteria":["Matching inventory remains visible."]}`
	correction, err := NewResponseCorrectionJobForField(
		job, "required behavior 1 duplicates earlier item 0", "required_behaviors_002",
	)
	if err != nil {
		t.Fatal(err)
	}
	prompt, schema, err := RenderPortableJob(correction)
	if err != nil {
		t.Fatal(err)
	}
	properties, _ := schema["properties"].(map[string]any)
	if len(properties) != 1 || properties["required_behaviors_002"] == nil ||
		strings.Contains(prompt, "Filter inventory") {
		t.Fatalf("correction authority leaked retained state or another field: %s %#v", prompt, schema)
	}
	corrected, err := ApplyResponseCorrectionForField(
		job, retained,
		`{"required_behaviors_002":"Applying it limits visible inventory."}`,
		"required_behaviors_002",
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeApplicationJobSpecificationResult(job, corrected); err != nil {
		t.Fatalf("corrected specification is invalid: %v\n%s", err, corrected)
	}
	if _, err := ApplyResponseCorrectionForField(
		job, retained, `{"objective":"Implement inventory filters now."}`, "objective",
	); err == nil {
		t.Fatal("unrelated specification field correction was accepted")
	}
}

func TestApplicationJobSpecificationResponseCorrectionRejectsValidOrUnscopedState(t *testing.T) {
	t.Parallel()
	job := applicationJobSpecificationCorrectionTestJob(t)
	if _, err := NewResponseCorrectionJob(job, "one field is invalid"); err == nil {
		t.Fatal("unscoped application specification correction was accepted")
	}
	valid := `{"objective":"Implement inventory filters.","required_behaviors":["Users can filter inventory."],"acceptance_criteria":["Applying a filter limits visible inventory."]}`
	if _, err := ApplyResponseCorrectionForField(
		job, valid, `{"objective":"Implement different inventory filters."}`,
		string(ApplicationJobSpecificationObjectiveField),
	); err == nil {
		t.Fatal("valid application specification gained correction authority")
	}
}

func applicationJobSpecificationCorrectionTestJob(t *testing.T) PortableJob {
	t.Helper()
	requirement := Requirement{ID: "requirement_001", SourceQuote: "filter inventory"}
	job, err := NewApplicationJobSpecificationJob(ApplicationJobSpecificationInput{
		Surface: ApplicationSurfaceBrowser, ProductQuote: "inventory console",
		AcceptedRequirements: []Requirement{requirement}, FocusedRequirement: requirement,
	})
	if err != nil {
		t.Fatal(err)
	}
	return job
}
