package worker

import (
	"strings"
	"testing"
)

func TestBrowserAcceptanceDerivedTextOutcomeRequiresNamedOutput(t *testing.T) {
	surface, err := extractDirectCodingBrowserPublicInteractionSurface(`
function View(): JSX.Element {
  return <main><p>Updated</p></main>;
}`)
	if err != nil {
		t.Fatalf("extract static surface: %v", err)
	}
	validationErr := validateDirectCodingBrowserAcceptanceRoleQueries(
		`function Verify(): void {
  expect(screen.getByText('Updated')).toBeInTheDocument();
}`,
		true,
		surface,
		browserAcceptanceExplicitResult,
	)
	if validationErr == nil || !strings.Contains(
		validationErr.Error(), "code-proven named status output",
	) || !strings.Contains(validationErr.Error(), "receipt has none") {
		t.Fatalf("static text qualified as a derived outcome: %v", validationErr)
	}
}

func TestBrowserAcceptanceUnrelatedNamedOutputCannotAuthorizeStaticText(t *testing.T) {
	surface, err := extractDirectCodingBrowserPublicInteractionSurface(`
		function View({ state }): JSX.Element {
  return <main><output aria-label="Live status">{state.liveStatus}</output><p>Updated</p></main>;
}`)
	if err != nil {
		t.Fatalf("extract unrelated output surface: %v", err)
	}
	validationErr := validateDirectCodingBrowserAcceptanceRoleQueries(
		`function Verify(): void {
  expect(screen.getByText('Updated')).toBeVisible();
}`,
		true,
		surface,
		browserAcceptanceExplicitResult,
	)
	if validationErr == nil || !strings.Contains(
		validationErr.Error(), "select that output by its exact status accessible name",
	) {
		t.Fatalf("unrelated named output authorized static text: %v", validationErr)
	}
}

func TestBrowserAcceptanceFindByTextCannotInventOutputBinding(t *testing.T) {
	surface := browserAcceptanceGrammarSurface()
	surface.Outputs = []directCodingBrowserPublicOutput{
		{AccessibleName: "Saved result"},
	}
	validationErr := validateDirectCodingBrowserAcceptanceRoleQueries(
		`async function Verify(): Promise<void> {
  expect(await screen.findByText('Saved')).toBeInTheDocument();
}`,
		true,
		surface,
		browserAcceptanceExplicitResult,
	)
	if validationErr == nil || !strings.Contains(
		validationErr.Error(), "select that output by its exact status accessible name",
	) {
		t.Fatalf("findByText invented dynamic-owner binding: %v", validationErr)
	}
}

func TestBrowserAcceptanceUnboundTextAfterInteractionFailsWithOwnershipResidual(t *testing.T) {
	surface := browserAcceptanceGrammarSurface()
	surface.Outputs = []directCodingBrowserPublicOutput{
		{AccessibleName: "Saved result"},
	}
	source := `function Verify(): void {
  fireEvent.click(screen.getByRole('button', { name: 'Submit' }));
  expect(screen.getByText('Saved')).toBeInTheDocument();
}`
	err := validateDirectCodingBrowserAcceptanceRoleQueries(
		source, true, surface, browserAcceptanceExplicitResult,
	)
	if err == nil || !strings.Contains(
		err.Error(), "select that output by its exact status accessible name",
	) {
		t.Fatalf("post-interaction text hid the ownership residual: %v", err)
	}
}
