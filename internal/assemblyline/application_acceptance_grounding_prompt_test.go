package assemblyline

import (
	"strings"
	"testing"
)

func TestApplicationAcceptanceGroundingPromptOmitsBindingsAndLocators(t *testing.T) {
	t.Parallel()

	source := `async function VerifyBoard(): Promise<void> {
  await waitFor(() => expect(screen.getByRole("status")).toHaveTextContent("Ready"));
}`
	input, err := NewApplicationAcceptanceGroundingReviewInput(ApplicationTaskContext{
		WorkloadSHA256: strings.Repeat("d", 64),
		Task: ApplicationTaskContextTask{TaskID: "task_007", AcceptanceCriteria: []string{
			"The board visibly reports its ready status.",
		}},
	}, source, true, []AcceptanceGroundingAuthority{
		{ID: "platform_wait", Kind: AcceptanceGroundingPlatformInvariant,
			Statement:  "The registered harness may wait for observable behavior.",
			Operations: []string{"harness_call:waitFor"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	prompt, err := BuildApplicationAcceptanceGroundingReviewPrompt(input)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{
		`"locators"`, `"start_byte"`, `"end_byte"`,
		`"start_line"`, `"source_sha256"`, `"inventory_sha256"`, `"tsx"`,
	} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("model projection leaked %q:\n%s", forbidden, prompt)
		}
	}
	for _, required := range []string{"status", "Ready", "expect_matcher:toHaveTextContent"} {
		if !strings.Contains(prompt, required) {
			t.Fatalf("model projection omitted product observation %q:\n%s", required, prompt)
		}
	}
}
