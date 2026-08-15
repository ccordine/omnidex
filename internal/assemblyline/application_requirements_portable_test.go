package assemblyline

import "testing"

func TestApplicationRequirementInterpretationResultUsesPortableAuthority(t *testing.T) {
	t.Parallel()

	job, err := NewApplicationRequirementInterpretationJob(
		ApplicationRequirementInterpretationInput{
			UserRequest: "Build an inventory console with filters and exports.",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	resolution, err := DecodeApplicationRequirementInterpretationResult(job, `{
		"schema":"omnidex.application-requirements.v1",
		"items":[
			{"kind":"product","source_quote":"inventory console"},
			{"kind":"feature","source_quote":"filters"},
			{"kind":"feature","source_quote":"exports"}
		]
	}`)
	if err != nil {
		t.Fatal(err)
	}
	if resolution.ProductQuote != "inventory console" || len(resolution.Requirements) != 2 {
		t.Fatalf("resolution=%+v", resolution)
	}
	if _, err := DecodeApplicationRequirementInterpretationResult(job, `{
		"schema":"omnidex.application-requirements.v1",
		"items":[
			{"kind":"product","source_quote":"inventory console"},
			{"kind":"feature","source_quote":"invented feature"}
		]
	}`); err == nil {
		t.Fatal("portable requirement result bypassed exact source grounding")
	}
}
