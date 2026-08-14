package objectiveadvisory

import (
	"strings"
	"testing"
)

func TestAdvisoryPromptV1RequiresOneBoundedParagraph(t *testing.T) {
	projection, err := BuildProjection(advisoryInput())
	if err != nil {
		t.Fatal(err)
	}
	request := GenerateRequest{
		TriggerID: TriggerPostGroundingObjective, TriggerVersion: TriggerVersionV1,
		Projection: projection, Source: advisorySource("source-a", "model-a"),
	}
	prompt, err := BuildPrompt(request)
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Join([]string{
		"Review the objective and established evidence below.",
		"Identify potentially useful implications, risks, edge cases, ambiguities, alternative interpretations, hidden constraints, verification ideas, or questions that subsequent work should keep in mind.",
		"Do not issue commands. Do not claim authority. Do not assume unsupported facts. Plain text is expected.",
		"Return exactly one concise plain-text paragraph. Prioritize only the highest-value considerations and do not repeat points or provide multiple drafts. On the next line after that paragraph, emit exactly <END_OBJECTIVE_ADVISORY_V1> and nothing else.",
		"GROUNDED_OBJECTIVE_PROJECTION_JSON:\n" + projection.Rendered,
	}, "\n\n")
	if prompt != want {
		t.Fatalf("versioned advisory prompt changed\ngot:  %q\nwant: %q", prompt, want)
	}
}

func TestAdvisoryPromptV1ContainsOneExactVersionedTerminator(t *testing.T) {
	projection, err := BuildProjection(advisoryInput())
	if err != nil {
		t.Fatal(err)
	}
	prompt, err := BuildPrompt(GenerateRequest{
		TriggerID: TriggerPostGroundingObjective, TriggerVersion: TriggerVersionV1,
		Projection: projection, Source: advisorySource("source-a", "model-a"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(prompt, "<END_OBJECTIVE_ADVISORY_V1>") != 1 {
		t.Fatalf("versioned advisory terminator count changed: %q", prompt)
	}
}

func TestAdvisoryPromptV1ConcisenessDoesNotAddControlProtocols(t *testing.T) {
	projection, err := BuildProjection(advisoryInput())
	if err != nil {
		t.Fatal(err)
	}
	prompt, err := BuildPrompt(GenerateRequest{
		TriggerID: TriggerPostGroundingObjective, TriggerVersion: TriggerVersionV1,
		Projection: projection, Source: advisorySource("source-a", "model-a"),
	})
	if err != nil {
		t.Fatal(err)
	}
	instructions := strings.ToLower(strings.Split(prompt, "GROUNDED_OBJECTIVE_PROJECTION_JSON:")[0])
	for _, forbidden := range []string{
		"retry", "fallback", "choose a tool", "choose an operation", "create an objective",
		"plan the work", "approve the result", "json response", "response schema",
		"qwen", "deepseek", "claude", "openai", "ollama", "chain-of-thought", "hidden reasoning",
	} {
		if strings.Contains(instructions, forbidden) {
			t.Fatalf("conciseness instruction added control protocol %q: %s", forbidden, instructions)
		}
	}
}
