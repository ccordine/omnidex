package assemblyline

import (
	"strings"
	"testing"
)

func TestCapabilityRelationSeesExactlyTwoLocalNeeds(t *testing.T) {
	t.Parallel()

	input := CapabilityRelationInput{
		LocalContext: "interactive browser inventory",
		LeftNeed:     "filter visible records",
		RightNeed:    "show a selected record summary",
	}
	job, err := NewCapabilityRelationJob(input)
	if err != nil {
		t.Fatal(err)
	}
	prompt, err := RenderPortableJob(job)
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{input.LocalContext, input.LeftNeed, input.RightNeed} {
		if !strings.Contains(prompt, required) {
			t.Fatalf("relation prompt omitted %q:\n%s", required, prompt)
		}
	}
	for _, required := range []string{
		"independent when neither behavior must consume a result uniquely produced by the other",
		"left_reads_right only when LEFT_NEED cannot satisfy",
		"right_reads_left only when RIGHT_NEED cannot satisfy",
		"Shared request or user input", "return independent",
	} {
		if !strings.Contains(prompt, required) {
			t.Fatalf("relation prompt omitted direction authority %q:\n%s", required, prompt)
		}
	}
	for _, forbidden := range []string{"workspace", "filename", "project tree", "document", "agent"} {
		if strings.Contains(strings.ToLower(prompt), forbidden) {
			t.Fatalf("relation prompt leaked %q:\n%s", forbidden, prompt)
		}
	}
	decision, err := DecodeCapabilityRelationDecision(input, string(CapabilityIndependent))
	if err != nil || decision.Relation != CapabilityIndependent || decision.Schema != CapabilityRelationSchemaV1 {
		t.Fatalf("decision=%+v err=%v", decision, err)
	}
}

func TestCapabilityRelationRejectsUnknownDirection(t *testing.T) {
	t.Parallel()

	input := CapabilityRelationInput{
		LocalContext: "interactive browser inventory",
		LeftNeed:     "filter visible records",
		RightNeed:    "show a selected record summary",
	}
	for _, relation := range []CapabilityRelation{"similar", "bidirectional"} {
		err := (CapabilityRelationDecision{
			Schema: CapabilityRelationSchemaV1, Relation: relation,
		}).ValidateFor(input)
		if err == nil {
			t.Fatalf("capability relation accepted unsupported direction %q", relation)
		}
	}
	if _, err := DecodeCapabilityRelationDecision(input, `{"relation":"independent"}`); err == nil {
		t.Fatal("accepted JSON wrapper")
	}
}
