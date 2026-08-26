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
	prompt, schema, err := RenderPortableJob(job)
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{input.LocalContext, input.LeftNeed, input.RightNeed} {
		if !strings.Contains(prompt, required) {
			t.Fatalf("relation prompt omitted %q:\n%s", required, prompt)
		}
	}
	for _, forbidden := range []string{"workspace", "filename", "project tree", "document", "agent"} {
		if strings.Contains(strings.ToLower(prompt), forbidden) {
			t.Fatalf("relation prompt leaked %q:\n%s", forbidden, prompt)
		}
	}
	if schema == nil {
		t.Fatal("capability relation omitted its response schema")
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
}
