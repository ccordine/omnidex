package assemblyline

import (
	"strings"
	"testing"
)

func TestAcceptanceObserverGrammarRejectsLocalProofSubstitutes(t *testing.T) {
	t.Parallel()

	for name, source := range map[string]string{
		"fake object": `function VerifyFake(): void {
  screen.getByText("Stock items");
  const privateFake = { disabled: false };
  expect(privateFake.disabled).toBe(true);
}`,
		"tautology": `function VerifyTautology(): void {
  expect(true).toBe(true);
}`,
		"query alias": `function VerifyAlias(): void {
  const privateButton = screen.getByRole("button", { name: "Save" });
  expect(privateButton).toBeEnabled();
}`,
		"standalone nonthrowing query": `function VerifyAbsent(): void {
  screen.queryByText("Missing");
}`,
	} {
		t.Run(name, func(t *testing.T) {
			inventory, err := InventoryTypeScriptAcceptanceObservations(source, false)
			if err != nil {
				t.Fatal(err)
			}
			projection := inventory.canonicalModelProjection()
			if !strings.Contains(projection, "untrusted_call") {
				t.Fatalf("local proof substitute was trusted: %s", projection)
			}
			for _, forbidden := range []string{"privateFake", "privateButton"} {
				if strings.Contains(projection, forbidden) {
					t.Fatalf("private proof identity leaked %q: %s", forbidden, projection)
				}
			}
		})
	}
}

func TestAcceptanceObserverGrammarAcceptsOnlyDirectPublicObservationForms(t *testing.T) {
	t.Parallel()

	for name, source := range map[string]string{
		"property and index": `function VerifyRows(): void {
  expect(screen.getAllByRole("row")[4].textContent).toBe("Ready");
}`,
		"event": `function VerifySave(): void {
  fireEvent.click(screen.getByRole("button", { name: "Save" }));
}`,
		"wait callback": `async function VerifyStatus(): Promise<void> {
  await waitFor(() => {
    expect(screen.getByRole("status")).toHaveTextContent("Saved");
    fireEvent.click(screen.getByRole("button", { name: "Dismiss" }));
  });
}`,
		"standalone throwing query": `function VerifyHeading(): void {
  screen.getByRole("heading", { name: "Inventory" });
}`,
	} {
		t.Run(name, func(t *testing.T) {
			inventory, err := InventoryTypeScriptAcceptanceObservations(source, false)
			if err != nil {
				t.Fatal(err)
			}
			if projection := inventory.canonicalModelProjection(); strings.Contains(projection, "untrusted_call") {
				t.Fatalf("direct public observation was rejected: %s", projection)
			}
		})
	}
}

func TestAcceptanceObserverGrammarReceiptRoutesOnlyDirectPublicFailure(t *testing.T) {
	t.Parallel()

	source := `function VerifyStatus(): void {
  expect(screen.getByRole("status")).toHaveTextContent("Ready");
}`
	context := ApplicationTaskContext{
		WorkloadSHA256: strings.Repeat("9", 64),
		Task: ApplicationTaskContextTask{TaskID: "task_010", AcceptanceCriteria: []string{
			"The public status visibly reports readiness.",
		}},
	}
	input, err := NewApplicationAcceptanceGroundingReviewInput(context, source, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	selected := make(map[string]bool)
	for _, site := range input.reviewSites() {
		selected[acceptanceGroundingLeafField(site.ID, "criterion_001")] = true
	}
	review, err := DecodeApplicationAcceptanceGroundingReview(
		input, acceptanceGroundingLeafFixtureRaw(t, input, selected),
	)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := AcceptApplicationAcceptanceGroundingReview(input, review)
	if err != nil {
		t.Fatal(err)
	}
	column := strings.Index(strings.Split(source, "\n")[1], "expect") + 1
	authorized, err := receipt.AuthorizesFeatureFailureAt(input, source, false, 2, column)
	if err != nil {
		t.Fatal(err)
	}
	if !authorized {
		t.Fatal("direct grounded matcher failure was not routed to feature authority")
	}

	untrusted := strings.Replace(source,
		`screen.getByRole("status")`, `({ textContent: "Ready" })`, 1,
	)
	untrustedInput, err := NewApplicationAcceptanceGroundingReviewInput(context, untrusted, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	untrustedReview, err := DecodeApplicationAcceptanceGroundingReview(
		untrustedInput, acceptanceGroundingLeafFixtureRaw(t, untrustedInput, selected),
	)
	if err != nil {
		t.Fatal(err)
	}
	if untrustedReview.Decision != AcceptanceGroundingRepair {
		t.Fatal("local matcher subject obtained routable authority")
	}
}
