package worker

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

const (
	browserAcceptanceNoDerivedResult = assemblyline.ApplicationRequirementNoDerivedResult
	browserAcceptanceExplicitResult  = assemblyline.ApplicationRequirementExplicitResultRelation
)

func TestBrowserAcceptanceRequiresOutcomeAfterFinalInteraction(t *testing.T) {
	fixtures := []struct {
		name    string
		surface directCodingBrowserPublicInteractionSurface
		source  string
	}{
		{
			name:    "inventory control existence",
			surface: browserAcceptanceDynamicInventorySurface(),
			source: `function VerifyInventory(): void {
  fireEvent.change(screen.getByRole('textbox', { name: 'Stock code' }), { target: { value: 'AX-7' } });
  fireEvent.click(screen.getByRole('button', { name: 'Apply adjustment' }));
  expect(screen.getByRole('button', { name: 'Apply adjustment' })).toBeInTheDocument();
}`,
		},
		{
			name:    "travel outcome before final interaction",
			surface: browserAcceptanceDynamicTravelSurface(),
			source: `function VerifyTravel(): void {
  fireEvent.change(screen.getByRole('textbox', { name: 'Arrival city' }), { target: { value: 'Lisbon' } });
  expect(screen.getByRole('status', { name: 'Travel duration' })).toHaveTextContent(/^9 hours$/);
  fireEvent.click(screen.getByRole('button', { name: 'Find routes' }));
  expect(screen.getByRole('button', { name: 'Find routes' })).toBeInTheDocument();
}`,
		},
	}
	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			err := validateDirectCodingBrowserAcceptanceRoleQueries(
				fixture.source, true, fixture.surface, browserAcceptanceExplicitResult,
			)
			if err == nil || !strings.Contains(
				err.Error(), "exact named status-output assertion after the final fireEvent",
			) {
				t.Fatalf("non-outcome verification error=%v", err)
			}
		})
	}
}

func TestBrowserAcceptanceAcceptsQualifyingPostInteractionOutcomes(t *testing.T) {
	fixtures := []struct {
		name     string
		surface  directCodingBrowserPublicInteractionSurface
		source   string
		relation string
	}{
		{
			name:     "inventory exact output",
			surface:  browserAcceptanceDynamicInventorySurface(),
			relation: browserAcceptanceExplicitResult,
			source: `function VerifyInventory(): void {
  fireEvent.change(screen.getByRole('textbox', { name: 'Stock code' }), { target: { value: 'AX-7' } });
  fireEvent.click(screen.getByRole('button', { name: 'Apply adjustment' }));
  expect(screen.getByRole('status', { name: 'Inventory result' })).toHaveTextContent(/^Updated$/);
}`,
		},
		{
			name:     "notification boolean state",
			relation: browserAcceptanceNoDerivedResult,
			surface: directCodingBrowserPublicInteractionSurface{Controls: []directCodingBrowserPublicControl{
				{
					Role: "checkbox", RoleOrdinal: 1, RoleCount: 1,
					AccessibleName: "Email alerts", ValueKind: "boolean",
				},
			}},
			source: `function VerifyNotifications(): void {
  fireEvent.click(screen.getByRole('checkbox', { name: 'Email alerts' }));
  expect(screen.getByRole('checkbox', { name: 'Email alerts' })).toBeChecked();
}`,
		},
		{
			name:     "profile field value",
			relation: browserAcceptanceNoDerivedResult,
			surface: directCodingBrowserPublicInteractionSurface{Controls: []directCodingBrowserPublicControl{
				{
					Role: "textbox", RoleOrdinal: 1, RoleCount: 1,
					AccessibleName: "Display name", ValueKind: "text",
				},
			}},
			source: `function VerifyProfile(): void {
  fireEvent.change(screen.getByRole('textbox', { name: 'Display name' }), { target: { value: 'Ada' } });
  expect(screen.getByRole('textbox', { name: 'Display name' })).toHaveValue('Ada');
}`,
		},
	}
	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			if err := validateDirectCodingBrowserAcceptanceRoleQueries(
				fixture.source, true, fixture.surface, fixture.relation,
			); err != nil {
				t.Fatalf("qualifying outcome was rejected: %v", err)
			}
		})
	}
}

func TestBrowserAcceptanceNamedOutputRequiresExactOutputOutcome(t *testing.T) {
	source := `function VerifyInventory(): void {
  fireEvent.change(screen.getByRole('textbox', { name: 'Stock code' }), { target: { value: 'AX-7' } });
  fireEvent.click(screen.getByRole('button', { name: 'Apply adjustment' }));
  expect(screen.getByRole('textbox', { name: 'Stock code' })).toHaveValue('AX-7');
}`
	err := validateDirectCodingBrowserAcceptanceRoleQueries(
		source, true, browserAcceptanceDynamicInventorySurface(), browserAcceptanceExplicitResult,
	)
	if err == nil || !strings.Contains(
		err.Error(), "exact named status-output assertion after the final fireEvent",
	) {
		t.Fatalf("role value replaced required dynamic text outcome: %v", err)
	}
}

func TestBrowserAcceptanceRejectsRoleIncompatibleOutcomeMatcher(t *testing.T) {
	surface := directCodingBrowserPublicInteractionSurface{Controls: []directCodingBrowserPublicControl{
		{
			Role: "button", RoleOrdinal: 1, RoleCount: 1,
			AccessibleName: "Publish report", ValueKind: "action",
		},
	}}
	for name, matcher := range map[string]string{
		"value":    "toHaveValue('published')",
		"disabled": "toBeDisabled()",
		"enabled":  "toBeEnabled()",
		"required": "toBeRequired()",
	} {
		t.Run(name, func(t *testing.T) {
			source := `function VerifyReport(): void {
  fireEvent.click(screen.getByRole('button', { name: 'Publish report' }));
  expect(screen.getByRole('button', { name: 'Publish report' })).` + matcher + `;
}`
			err := validateDirectCodingBrowserAcceptanceRoleQueries(
				source, true, surface, browserAcceptanceExplicitResult,
			)
			if err == nil || !strings.Contains(err.Error(), "unsupported or has non-static") {
				t.Fatalf("action control qualified through %s: %v", matcher, err)
			}
		})
	}
}

func TestBrowserAcceptanceControlPresenceWithoutInteractionRemainsValid(t *testing.T) {
	source := `function VerifyControl(): void {
  expect(screen.getByRole('button', { name: 'Apply adjustment' })).toBeInTheDocument();
}`
	if err := validateDirectCodingBrowserAcceptanceRoleQueries(
		source, true, browserAcceptanceInventorySurface(), browserAcceptanceNoDerivedResult,
	); err != nil {
		t.Fatalf("control-presence verification was rejected: %v", err)
	}
}

func TestBrowserAcceptanceExplicitResultRejectsPureControlPresence(t *testing.T) {
	source := `function VerifyControl(): void {
  expect(screen.getByRole('button', { name: 'Apply adjustment' })).toBeInTheDocument();
}`
	err := validateDirectCodingBrowserAcceptanceRoleQueries(
		source, true, browserAcceptanceInventorySurface(), browserAcceptanceExplicitResult,
	)
	if err == nil || !strings.Contains(
		err.Error(), "explicit derived-result relation requires one qualifying exact named status-output assertion",
	) {
		t.Fatalf("explicit derived result accepted pure control presence: %v", err)
	}
}

func browserAcceptanceDynamicInventorySurface() directCodingBrowserPublicInteractionSurface {
	surface := browserAcceptanceInventorySurface()
	surface.Outputs = []directCodingBrowserPublicOutput{
		{AccessibleName: "Inventory result"},
	}
	return surface
}

func browserAcceptanceDynamicTravelSurface() directCodingBrowserPublicInteractionSurface {
	surface := browserAcceptanceTravelSurface()
	surface.Outputs = []directCodingBrowserPublicOutput{
		{AccessibleName: "Travel duration"},
	}
	return surface
}
