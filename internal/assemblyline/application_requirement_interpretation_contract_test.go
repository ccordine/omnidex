package assemblyline

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestResolveApplicationRequirementsGroundsSortsAndAssignsCodeOwnedIDs(t *testing.T) {
	t.Parallel()
	input := ApplicationRequirementInterpretationInput{
		UserRequest: "Build a small browser music studio with channels, drum pads, and a keyboard.",
	}
	candidate := ApplicationRequirementInterpretation{
		Schema: ApplicationRequirementInterpretationSchemaV1,
		Items: []ApplicationRequirementItem{
			{Kind: ApplicationRequirementFeature, SourceQuote: "a keyboard"},
			{Kind: ApplicationRequirementProduct, SourceQuote: "browser music studio"},
			{Kind: ApplicationRequirementFeature, SourceQuote: "drum pads"},
			{Kind: ApplicationRequirementFeature, SourceQuote: "channels"},
		},
	}
	resolved, err := ResolveApplicationRequirements(input, candidate)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.ProductQuote != "browser music studio" {
		t.Fatalf("product=%q", resolved.ProductQuote)
	}
	want := []Requirement{
		{ID: "requirement_001", SourceQuote: "channels"},
		{ID: "requirement_002", SourceQuote: "drum pads"},
		{ID: "requirement_003", SourceQuote: "a keyboard"},
	}
	if !reflect.DeepEqual(resolved.Requirements, want) {
		t.Fatalf("requirements=%+v want %+v", resolved.Requirements, want)
	}
}

func TestResolveApplicationRequirementsRejectsInvalidTypedLists(t *testing.T) {
	t.Parallel()
	baseInput := ApplicationRequirementInterpretationInput{
		UserRequest: "Build a browser catalog with grouped records, saved filters, and printable summaries.",
	}
	valid := ApplicationRequirementInterpretation{
		Schema: ApplicationRequirementInterpretationSchemaV1,
		Items: []ApplicationRequirementItem{
			{Kind: ApplicationRequirementProduct, SourceQuote: "browser catalog"},
			{Kind: ApplicationRequirementFeature, SourceQuote: "grouped records"},
		},
	}
	tests := map[string]struct {
		input     ApplicationRequirementInterpretationInput
		candidate ApplicationRequirementInterpretation
	}{
		"schema": {input: baseInput, candidate: ApplicationRequirementInterpretation{
			Schema: "omnidex.invalid", Items: valid.Items,
		}},
		"kind": {input: baseInput, candidate: ApplicationRequirementInterpretation{
			Schema: valid.Schema, Items: []ApplicationRequirementItem{
				{Kind: ApplicationRequirementProduct, SourceQuote: "browser catalog"},
				{Kind: ApplicationRequirementKind("constraint"), SourceQuote: "grouped records"},
			},
		}},
		"missing product": {input: baseInput, candidate: ApplicationRequirementInterpretation{
			Schema: valid.Schema, Items: []ApplicationRequirementItem{
				{Kind: ApplicationRequirementFeature, SourceQuote: "grouped records"},
			},
		}},
		"multiple products": {input: baseInput, candidate: ApplicationRequirementInterpretation{
			Schema: valid.Schema, Items: append(append([]ApplicationRequirementItem{}, valid.Items...),
				ApplicationRequirementItem{Kind: ApplicationRequirementProduct, SourceQuote: "catalog"}),
		}},
		"missing feature": {input: baseInput, candidate: ApplicationRequirementInterpretation{
			Schema: valid.Schema, Items: []ApplicationRequirementItem{
				{Kind: ApplicationRequirementProduct, SourceQuote: "browser catalog"},
			},
		}},
		"duplicate": {input: baseInput, candidate: ApplicationRequirementInterpretation{
			Schema: valid.Schema, Items: append(append([]ApplicationRequirementItem{}, valid.Items...),
				ApplicationRequirementItem{Kind: ApplicationRequirementFeature, SourceQuote: "grouped records"}),
		}},
		"ungrounded": {input: baseInput, candidate: ApplicationRequirementInterpretation{
			Schema: valid.Schema, Items: []ApplicationRequirementItem{
				{Kind: ApplicationRequirementProduct, SourceQuote: "browser catalog"},
				{Kind: ApplicationRequirementFeature, SourceQuote: "invented grouping"},
			},
		}},
		"overlap": {
			input: ApplicationRequirementInterpretationInput{UserRequest: "Build a browser catalog with grouped records."},
			candidate: ApplicationRequirementInterpretation{Schema: valid.Schema, Items: []ApplicationRequirementItem{
				{Kind: ApplicationRequirementProduct, SourceQuote: "browser catalog"},
				{Kind: ApplicationRequirementFeature, SourceQuote: "browser"},
			}},
		},
		"ambiguous": {
			input: ApplicationRequirementInterpretationInput{UserRequest: "Build a catalog with search and search history."},
			candidate: ApplicationRequirementInterpretation{Schema: valid.Schema, Items: []ApplicationRequirementItem{
				{Kind: ApplicationRequirementProduct, SourceQuote: "catalog"},
				{Kind: ApplicationRequirementFeature, SourceQuote: "search"},
			}},
		},
	}
	for name, test := range tests {
		name, test := name, test
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := ResolveApplicationRequirements(test.input, test.candidate); err == nil {
				t.Fatalf("accepted invalid interpretation %+v", test.candidate)
			}
		})
	}
}

func TestResolveApplicationRequirementsEnforcesOneToTenFeatures(t *testing.T) {
	t.Parallel()
	parts := []string{"Build a catalog with"}
	items := []ApplicationRequirementItem{
		{Kind: ApplicationRequirementProduct, SourceQuote: "catalog"},
	}
	for index := 1; index <= 11; index++ {
		quote := fmt.Sprintf("feature %02d", index)
		parts = append(parts, quote)
		items = append(items, ApplicationRequirementItem{
			Kind: ApplicationRequirementFeature, SourceQuote: quote,
		})
	}
	input := ApplicationRequirementInterpretationInput{UserRequest: strings.Join(parts, ", ") + "."}
	tooMany := ApplicationRequirementInterpretation{
		Schema: ApplicationRequirementInterpretationSchemaV1, Items: items,
	}
	if _, err := ResolveApplicationRequirements(input, tooMany); err == nil {
		t.Fatal("accepted eleven features")
	}
	maximum := tooMany
	maximum.Items = append([]ApplicationRequirementItem{}, items[:11]...)
	resolved, err := ResolveApplicationRequirements(input, maximum)
	if err != nil {
		t.Fatal(err)
	}
	if len(resolved.Requirements) != 10 || resolved.Requirements[9].ID != "requirement_010" {
		t.Fatalf("maximum resolution=%+v", resolved)
	}
}

func TestApplicationRequirementJobCarriesOneIntactRequestAndClosedSchema(t *testing.T) {
	t.Parallel()
	const request = "Create an appointment scheduler with recurring reminders and cancellation notices."
	job, err := NewApplicationRequirementInterpretationJob(
		ApplicationRequirementInterpretationInput{UserRequest: request},
	)
	if err != nil {
		t.Fatal(err)
	}
	if job.Kind != WorkApplicationRequirements {
		t.Fatalf("work kind=%q", job.Kind)
	}
	prompt, schema, err := RenderPortableJob(job)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(prompt, request) != 1 {
		t.Fatalf("intact request count=%d prompt=%q", strings.Count(prompt, request), prompt)
	}
	properties := schema["properties"].(map[string]any)
	items := properties["items"].(map[string]any)
	if len(properties) != 2 || items["minItems"] != 2 || items["maxItems"] != 11 {
		t.Fatalf("interpretation schema=%#v", schema)
	}
	item := items["items"].(map[string]any)
	itemProperties := item["properties"].(map[string]any)
	kinds := itemProperties["kind"].(map[string]any)["enum"]
	quote := itemProperties["source_quote"].(map[string]any)
	if !reflect.DeepEqual(kinds, []string{"product", "feature"}) ||
		quote["minLength"] != 1 || quote["maxLength"] != maxRequirementQuoteBytes {
		t.Fatalf("interpretation item schema=%#v", item)
	}
}

func TestApplicationRequirementInterpretationRejectsRetiredPartitionWires(t *testing.T) {
	t.Parallel()
	for name, raw := range map[string]string{
		"partition payload": `{"source_text":"Build a catalog.","mode":"extract_features","excluded_quotes":[]}`,
		"candidate choice":  `{"schema":"omnidex.requirement-partition-extraction.v1","candidate_id":"none"}`,
		"step none":         `{"schema":"omnidex.requirement-partition-step.v1","outcome":"none","feature_quotes":[]}`,
		"aggregate quotes":  `{"schema":"omnidex.requirement-partition.v1","feature_quotes":["grouped records"]}`,
	} {
		name, raw := name, raw
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if name == "partition payload" {
				var input ApplicationRequirementInterpretationInput
				if err := decodePortablePayload([]byte(raw), &input); err == nil {
					t.Fatalf("accepted retired input %s", raw)
				}
				return
			}
			var candidate ApplicationRequirementInterpretation
			if err := decodePortablePayload([]byte(raw), &candidate); err == nil {
				t.Fatalf("accepted retired response %s", raw)
			}
		})
	}
}
