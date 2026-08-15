package assemblyline

import (
	"strings"
	"testing"
)

func TestAcceptanceGroundingDoesNotPrebindUntrustedWaitSite(t *testing.T) {
	t.Parallel()

	source := `async function VerifyStatus(): Promise<void> {
  await waitFor(() => expect(true).toBe(true));
}`
	input, err := NewApplicationAcceptanceGroundingReviewInput(ApplicationTaskContext{
		WorkloadSHA256: strings.Repeat("7", 64),
		Task: ApplicationTaskContextTask{TaskID: "task_012", AcceptanceCriteria: []string{
			"The current status is visible.",
		}},
	}, source, false, []AcceptanceGroundingAuthority{{
		ID: "platform_wait_harness", Kind: AcceptanceGroundingPlatformInvariant,
		Statement:  "The registered harness may await direct observable behavior.",
		Operations: []string{"harness_call:waitFor"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	for _, site := range input.Inventory.Sites {
		if len(site.Operations) > 0 && site.Operations[0] == "harness_call:waitFor" &&
			!stringInSet("untrusted_call", site.Operations) {
			t.Fatalf("invalid callback did not taint wait site: %+v", site)
		}
	}
	prompt, err := BuildApplicationAcceptanceGroundingReviewPrompt(input)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prompt, "harness_call:waitFor") || !strings.Contains(prompt, "untrusted_call") {
		t.Fatalf("untrusted wait site was hidden from review: %s", prompt)
	}
	review, err := DecodeApplicationAcceptanceGroundingReview(
		input, acceptanceGroundingLeafFixtureRaw(t, input, nil),
	)
	if err != nil {
		t.Fatal(err)
	}
	if review.Decision != AcceptanceGroundingRepair || review.UnsupportedSiteID == "" {
		t.Fatalf("untrusted wait site obtained deterministic platform authority: %+v", review)
	}
}

func TestAcceptanceGroundingRejectsShadowedHarnessGlobalsThroughTypedRepair(t *testing.T) {
	t.Parallel()

	source := `async function VerifyStatus(): Promise<void> {
  const waitFor = async (callback: () => void): Promise<void> => callback();
  await waitFor(() => expect(screen.getByText("Ready")).toBeVisible());
}`
	input, err := NewApplicationAcceptanceGroundingReviewInput(ApplicationTaskContext{
		WorkloadSHA256: strings.Repeat("6", 64),
		Task: ApplicationTaskContextTask{TaskID: "task_013", AcceptanceCriteria: []string{
			"The ready state is visible.",
		}},
	}, source, false, []AcceptanceGroundingAuthority{{
		ID: "platform_wait_harness", Kind: AcceptanceGroundingPlatformInvariant,
		Statement:  "The registered harness may await direct observable behavior.",
		Operations: []string{"harness_call:waitFor"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	var unsupported string
	for _, site := range input.reviewSites() {
		if stringInSet("untrusted_call", site.Operations) {
			unsupported = site.ID
			break
		}
	}
	if unsupported == "" {
		t.Fatal("shadow binding did not remain a typed unsupported review site")
	}
	review, err := DecodeApplicationAcceptanceGroundingReview(
		input, acceptanceGroundingLeafFixtureRaw(t, input, nil),
	)
	if err != nil {
		t.Fatalf("shadow binding could not enter typed correction: %v", err)
	}
	if review.Decision != AcceptanceGroundingRepair || review.UnsupportedSiteID != unsupported {
		t.Fatalf("shadow binding repair was not code-derived: %+v", review)
	}
}
