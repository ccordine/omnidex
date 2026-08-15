package assemblyline

import (
	"strings"
	"testing"
)

func TestAcceptanceObserverGrammarRequiresAwaitForAsyncProofs(t *testing.T) {
	t.Parallel()

	rejected := map[string]string{
		"standalone find": `async function VerifyStatus(): Promise<void> {
  screen.findByText("Ready");
}`,
		"matcher find": `async function VerifyStatus(): Promise<void> {
  expect(screen.findByText("Ready")).toBeVisible();
}`,
		"wait callback": `async function VerifyStatus(): Promise<void> {
  waitFor(() => expect(screen.getByText("Ready")).toBeVisible());
}`,
		"unsupported promise modifier": `async function VerifyStatus(): Promise<void> {
  expect(screen.getByText("Ready")).resolves.toBeVisible();
}`,
	}
	for name, source := range rejected {
		t.Run("reject "+name, func(t *testing.T) {
			assertAcceptanceObserverTrust(t, source, false)
		})
	}

	accepted := map[string]string{
		"standalone find": `async function VerifyStatus(): Promise<void> {
  await screen.findByText("Ready");
}`,
		"matcher find": `async function VerifyStatus(): Promise<void> {
  expect(await screen.findByText("Ready")).toBeVisible();
}`,
		"wait callback": `async function VerifyStatus(): Promise<void> {
  await waitFor(() => expect(screen.getByText("Ready")).toBeVisible());
}`,
	}
	for name, source := range accepted {
		t.Run("accept "+name, func(t *testing.T) {
			assertAcceptanceObserverTrust(t, source, true)
		})
	}
}

func TestAcceptanceObserverGrammarAcceptsStaticKeyboardAndPointerPayloads(t *testing.T) {
	t.Parallel()

	for name, source := range map[string]string{
		"keyboard": `function VerifyKeyboard(): void {
  fireEvent.keyDown(screen.getByRole("textbox"), { key: "Enter", code: "Enter", ctrlKey: true });
}`,
		"pointer": `function VerifyPointer(): void {
  fireEvent.pointerDown(screen.getByRole("slider"), { pointerId: 7, pointerType: "mouse", clientX: 24, button: 0 });
}`,
	} {
		t.Run(name, func(t *testing.T) {
			assertAcceptanceObserverTrust(t, source, true)
		})
	}
}

func TestAcceptanceGroundingRoutesMatcherButNotEventMechanics(t *testing.T) {
	t.Parallel()

	source := `function VerifySave(): void {
  fireEvent.click(screen.getByRole("button", { name: "Save" }));
  expect(screen.getByRole("status")).toHaveTextContent("Saved");
}`
	context := ApplicationTaskContext{
		WorkloadSHA256: strings.Repeat("8", 64),
		Task: ApplicationTaskContextTask{TaskID: "task_011", AcceptanceCriteria: []string{
			"Saving visibly reports completion.",
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
	lines := strings.Split(source, "\n")
	for name, testCase := range map[string]struct {
		line       int
		needle     string
		authorized bool
	}{
		"event mechanics": {line: 2, needle: "fireEvent", authorized: false},
		"matcher proof":   {line: 3, needle: "expect", authorized: true},
	} {
		t.Run(name, func(t *testing.T) {
			column := strings.Index(lines[testCase.line-1], testCase.needle) + 1
			authorized, err := receipt.AuthorizesFeatureFailureAt(
				input, source, false, testCase.line, column,
			)
			if err != nil {
				t.Fatal(err)
			}
			if authorized != testCase.authorized {
				t.Fatalf("authorized=%v want=%v", authorized, testCase.authorized)
			}
		})
	}
}

func assertAcceptanceObserverTrust(t *testing.T, source string, trusted bool) {
	t.Helper()
	inventory, err := InventoryTypeScriptAcceptanceObservations(source, false)
	if err != nil {
		t.Fatal(err)
	}
	containsUntrusted := strings.Contains(inventory.canonicalModelProjection(), "untrusted_call")
	if containsUntrusted == trusted {
		t.Fatalf("trusted=%v inventory=%s", trusted, inventory.canonicalModelProjection())
	}
}
