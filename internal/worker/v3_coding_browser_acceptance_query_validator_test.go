package worker

import (
	"fmt"
	"strings"
	"testing"
)

func TestBrowserAcceptanceRoleQueriesGroundUnrelatedPublicSurfaces(t *testing.T) {
	fixtures := []struct {
		name    string
		surface directCodingBrowserPublicInteractionSurface
		source  string
		tsx     bool
	}{
		{
			name:    "inventory",
			surface: browserAcceptanceDynamicInventorySurface(),
			source: `async function VerifyInventory(): Promise<void> {
  fireEvent.change(screen.getAllByRole('textbox')[0], { target: { value: 'AX-7' } });
  fireEvent.change(screen.getByRole('textbox', { name: 'Lot code' }), { target: { value: 'L2' } });
  fireEvent.click(screen.getByRole('button', { name: 'Apply adjustment' }));
  expect(screen.getByRole('status', { name: 'Inventory result' })).toHaveTextContent(/^Updated$/);
}`,
		},
		{
			name:    "travel",
			surface: browserAcceptanceDynamicTravelSurface(),
			tsx:     true,
			source: `async function VerifyTravel(): Promise<void> {
  fireEvent.change(screen.getAllByRole("textbox")[1], { target: { value: "Lisbon" } });
  fireEvent.click(await screen.findByRole("button", { name: "Find routes" }));
  await waitFor(() => expect(await screen.findByRole("status", { name: "Travel duration" })).toHaveTextContent(/^9 hours$/));
}`,
		},
	}
	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			if err := validateDirectCodingBrowserAcceptanceRoleQueries(
				fixture.source, fixture.tsx, fixture.surface, browserAcceptanceExplicitResult,
			); err != nil {
				t.Fatalf("validate %s role queries: %v", fixture.name, err)
			}
		})
	}
}

func TestBrowserAcceptanceRoleQueriesRejectUnprovenSelectors(t *testing.T) {
	inventory := browserAcceptanceInventorySurface()
	tests := map[string]struct {
		source string
		want   string
	}{
		"singular repeated unnamed role": {
			source: verifyRoleQuery(`screen.getByRole('textbox')`),
			want:   "matches 2 public controls",
		},
		"role absent from receipt": {
			source: verifyRoleQuery(`screen.getByRole('spinbutton')`),
			want:   `no control with role "spinbutton"`,
		},
		"name absent from receipt": {
			source: verifyRoleQuery(`screen.getByRole('button', { name: 'Delete inventory' })`),
			want:   `accessible name "Delete inventory"`,
		},
		"all-query index outside receipt": {
			source: verifyRoleQuery(`screen.getAllByRole('textbox')[2]`),
			want:   "index 2 is outside 2 public matches",
		},
		"dynamic all-query index": {
			source: `function Verify(): void { const index = 0; expect(screen.getAllByRole('textbox')[index]).not.toBeNull(); }`,
			want:   "requires a literal zero-based index",
		},
		"dynamic role": {
			source: `function Verify(): void { const role = 'button'; expect(screen.getByRole(role)).not.toBeNull(); }`,
			want:   "requires an exact role string literal",
		},
		"regular-expression name": {
			source: verifyRoleQuery(`screen.getByRole('button', { name: /apply/i })`),
			want:   "accessible name must be one non-empty exact string literal",
		},
		"unsupported selector option": {
			source: verifyRoleQuery(`screen.getByRole('button', { name: 'Apply adjustment', hidden: true })`),
			want:   "only one exact name literal",
		},
		"nonthrowing query": {
			source: verifyRoleQuery(`screen.queryByRole('button')`),
			want:   "unsupported by the grounded allowlist",
		},
		"non-screen query": {
			source: verifyRoleQuery(`panel.getByRole('button')`),
			want:   "must be a direct screen query",
		},
		"aliased query": {
			source: `function Verify(): void { const select = screen.getByRole; expect(select('button')).not.toBeNull(); }`,
			want:   "must be called directly",
		},
		"computed screen query": {
			source: verifyRoleQuery(`screen['getByRole']('button')`),
			want:   "screen authority requires a direct member query",
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			err := validateDirectCodingBrowserAcceptanceRoleQueries(
				test.source, true, inventory, browserAcceptanceNoDerivedResult,
			)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("want error containing %q, got %v", test.want, err)
			}
		})
	}
}

func TestBrowserAcceptanceTextQueriesDoNotInventDynamicResultSemantics(t *testing.T) {
	for _, expected := range []string{"9 hours", "999 hours"} {
		source := fmt.Sprintf(`async function VerifyTravel(): Promise<void> {
  fireEvent.change(screen.getByRole('textbox', { name: 'Arrival city' }), { target: { value: 'Lisbon' } });
  fireEvent.click(screen.getByRole('button', { name: 'Find routes' }));
  expect(screen.getByText('%s')).toBeInTheDocument();
}`, expected)
		err := validateDirectCodingBrowserAcceptanceRoleQueries(
			source, true, browserAcceptanceTravelSurface(), browserAcceptanceExplicitResult,
		)
		if err == nil || !strings.Contains(err.Error(), "code-proven named status output") {
			t.Fatalf("text query claimed dynamic outcome authority for %q: %v", expected, err)
		}
	}
}

func TestBrowserAcceptanceRoleQueriesRejectNonCanonicalSurface(t *testing.T) {
	surface := browserAcceptanceInventorySurface()
	surface.Controls[0].RoleCount = 1
	err := validateDirectCodingBrowserAcceptanceRoleQueries(
		verifyRoleQuery(`screen.getByRole('button', { name: 'Apply adjustment' })`),
		false,
		surface,
		browserAcceptanceNoDerivedResult,
	)
	if err == nil || !strings.Contains(err.Error(), "non-canonical role ordinals") {
		t.Fatalf("non-canonical surface error=%v", err)
	}
}

func browserAcceptanceInventorySurface() directCodingBrowserPublicInteractionSurface {
	return directCodingBrowserPublicInteractionSurface{Controls: []directCodingBrowserPublicControl{
		{Role: "textbox", RoleOrdinal: 1, RoleCount: 2, AccessibleName: "Stock code", ValueKind: "text"},
		{Role: "textbox", RoleOrdinal: 2, RoleCount: 2, AccessibleName: "Lot code", ValueKind: "text"},
		{Role: "button", RoleOrdinal: 1, RoleCount: 1, AccessibleName: "Apply adjustment", ValueKind: "action"},
	}}
}

func browserAcceptanceTravelSurface() directCodingBrowserPublicInteractionSurface {
	return directCodingBrowserPublicInteractionSurface{Controls: []directCodingBrowserPublicControl{
		{Role: "textbox", RoleOrdinal: 1, RoleCount: 2, AccessibleName: "Departure city", ValueKind: "text"},
		{Role: "textbox", RoleOrdinal: 2, RoleCount: 2, AccessibleName: "Arrival city", ValueKind: "text"},
		{Role: "button", RoleOrdinal: 1, RoleCount: 1, AccessibleName: "Find routes", ValueKind: "action"},
	}}
}

func verifyRoleQuery(query string) string {
	return "function Verify(): void { expect(" + query + ").not.toBeNull(); }"
}
