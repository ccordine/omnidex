package assemblyline

import (
	"strings"
	"testing"
)

func TestRequirementAdvisoryJobsRenderOneBoundedThreeStationProtocol(t *testing.T) {
	t.Parallel()

	input := RequirementPartitionInput{
		SourceText: "Build a catalog with grouped records and a saved filter.",
		Mode:       RequirementExtractFeatures,
	}
	briefing, err := NewRequirementPartitionBriefingJob(input)
	if err != nil {
		t.Fatal(err)
	}
	briefingPrompt, briefingSchema, err := RenderPortableJob(briefing)
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"code-registered analysis lens", "coverage", "atomicity", input.SourceText} {
		if !strings.Contains(briefingPrompt, required) {
			t.Fatalf("briefing prompt missing %q:\n%s", required, briefingPrompt)
		}
	}
	if briefingSchema["type"] != "object" {
		t.Fatalf("briefing schema=%#v", briefingSchema)
	}

	advisory, err := NewRequirementPartitionAdvisoryJob(RequirementPartitionAdvisoryInput{
		Original: input, Lens: RequirementLensAtomicity,
	})
	if err != nil {
		t.Fatal(err)
	}
	advisoryPrompt, advisorySchema, err := RenderPortableJob(advisory)
	if err != nil {
		t.Fatal(err)
	}
	if advisorySchema != nil {
		t.Fatalf("advisory unexpectedly has a structured response schema: %#v", advisorySchema)
	}
	for _, required := range []string{"plain text", "SELECTED_LENS:\natomicity", "coordinated clauses", input.SourceText} {
		if !strings.Contains(advisoryPrompt, required) {
			t.Fatalf("advisory prompt missing %q:\n%s", required, advisoryPrompt)
		}
	}

	memo := "The phrase joins two independently requested capabilities; preserve each exact span."
	synthesis, err := NewRequirementPartitionSynthesisJob(RequirementPartitionSynthesisInput{
		Original: input, AdvisoryMemo: memo,
	})
	if err != nil {
		t.Fatal(err)
	}
	synthesisPrompt, synthesisSchema, err := RenderPortableJob(synthesis)
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"original prompt is authoritative", "untrusted model output", "UNTRUSTED_ADVISORY_MEMO_JSON", memo, input.SourceText} {
		if !strings.Contains(synthesisPrompt, required) {
			t.Fatalf("synthesis prompt missing %q:\n%s", required, synthesisPrompt)
		}
	}
	if synthesisSchema["type"] != "object" {
		t.Fatalf("synthesis schema=%#v", synthesisSchema)
	}
	if briefing.ID == advisory.ID || advisory.ID == synthesis.ID || briefing.ID == synthesis.ID {
		t.Fatalf("phase jobs do not have distinct immutable identities: %q %q %q", briefing.ID, advisory.ID, synthesis.ID)
	}
}

func TestRequirementAdvisoryBriefingDecodeIsExactAndLensIsRegistered(t *testing.T) {
	t.Parallel()

	decision, err := DecodeRequirementPartitionBriefing(`{"schema":"omnidex.requirement-partition-briefing.v1","lens":"grounding"}`)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Lens != RequirementLensGrounding {
		t.Fatalf("decision=%#v", decision)
	}
	for _, raw := range []string{
		`{"schema":"omnidex.requirement-partition-briefing.v1","lens":"invented"}`,
		`{"schema":"omnidex.requirement-partition-briefing.v1","lens":"coverage","extra":true}`,
		`{"schema":"wrong","lens":"coverage"}`,
	} {
		if _, err := DecodeRequirementPartitionBriefing(raw); err == nil {
			t.Fatalf("accepted invalid briefing %s", raw)
		}
	}
}

func TestRequirementAdvisoryMemoIsBoundedAndCannotReplaceAuthority(t *testing.T) {
	t.Parallel()

	input := RequirementPartitionInput{SourceText: "saved filter and summary", Mode: RequirementSplitFeature}
	if _, err := NewRequirementPartitionAdvisoryJob(RequirementPartitionAdvisoryInput{
		Original: input, Lens: RequirementPartitionLens("invented"),
	}); err == nil {
		t.Fatal("unregistered advisory lens was accepted")
	}
	if _, err := NewRequirementPartitionSynthesisJob(RequirementPartitionSynthesisInput{
		Original: input, AdvisoryMemo: strings.Repeat("x", maxRequirementAdvisoryMemoBytes+1),
	}); err == nil {
		t.Fatal("oversized advisory memo was accepted")
	}
	if _, err := NewRequirementPartitionSynthesisJob(RequirementPartitionSynthesisInput{
		Original: input, AdvisoryMemo: "   ",
	}); err == nil {
		t.Fatal("empty advisory memo was accepted")
	}
}
