package assemblyline

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestLanguageBlindCorrectionBindsProjectionOutsideModelPayload(t *testing.T) {
	t.Parallel()
	input := FragmentCorrectionInput{
		CurrentDeclaration: "func Value() int { return missing() }",
		RepairGuidance:     "Replace the missing call with a local expression.",
	}
	projected, err := NewSourceProjectedFragmentCorrectionJob(input, "go")
	if err != nil {
		t.Fatal(err)
	}
	base := projected
	base.SourceProjection = ""
	base.ID = portableJobDigest(base.Schema, base.Kind, base.Payload)
	if err := base.Validate(); err != nil {
		t.Fatal(err)
	}
	basePrompt, err := RenderPortableJob(base)
	if err != nil {
		t.Fatal(err)
	}
	projectedPrompt, err := RenderPortableJob(projected)
	if err != nil {
		t.Fatal(err)
	}
	if projected.SourceProjection != "go" || projected.ID == base.ID ||
		!bytes.Equal(projected.Payload, base.Payload) || projectedPrompt != basePrompt {
		t.Fatalf("base=%+v projected=%+v prompts_equal=%t", base, projected, projectedPrompt == basePrompt)
	}
	var payload map[string]any
	if err := json.Unmarshal(projected.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload) != 2 || payload["current_declaration"] != input.CurrentDeclaration ||
		payload["repair_guidance"] != input.RepairGuidance {
		t.Fatalf("model payload gained projection authority: %s", projected.Payload)
	}
	encoded, err := json.Marshal(projected)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), `"source_projection":"go"`) {
		t.Fatalf("portable evidence omitted projection identity: %s", encoded)
	}
}

func TestLanguageBlindCorrectionRejectsInvalidProjectionIdentity(t *testing.T) {
	t.Parallel()
	base := FragmentCorrectionInput{
		CurrentDeclaration: "func Value() int { return missing() }",
		RepairGuidance:     "Replace the missing call.",
	}
	for _, projection := range []string{"", " go ", "unknown", "typescript"} {
		if _, err := NewSourceProjectedFragmentCorrectionJob(base, projection); err == nil {
			t.Fatalf("accepted projection identity %q", projection)
		}
	}
	base.Language, base.Signature = "go", "func Value() int"
	if _, err := NewSourceProjectedFragmentCorrectionJob(base, "go"); err == nil {
		t.Fatal("accepted redundant model-payload and portable projection identities")
	}
}

func TestGeneralCorrectionConstructorRejectsLanguageBlindDeclaration(t *testing.T) {
	t.Parallel()
	_, err := NewFragmentCorrectionJob(FragmentCorrectionInput{
		CurrentDeclaration: "func Value() int { return missing() }",
		RepairGuidance:     "Replace the missing call with a local expression.",
	})
	if err == nil || !strings.Contains(err.Error(), "source-projected constructor") {
		t.Fatalf("language-blind correction error=%v", err)
	}
}
