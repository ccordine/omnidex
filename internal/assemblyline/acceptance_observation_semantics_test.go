package assemblyline

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestAcceptanceObservationInventoryProjectsCompleteMatcherExpression(t *testing.T) {
	t.Parallel()

	source := `function VerifyArchive(): void {
  expect(screen.getAllByRole(` + "`row`" + `)[4].textContent).not.toBe(` + "`Archived`" + `);
}`
	inventory, err := InventoryTypeScriptAcceptanceObservations(source, false)
	if err != nil {
		t.Fatal(err)
	}
	matcher := siteForOperation(t, inventory, "expect_matcher:toBe")
	projection := acceptanceSiteProjection(matcher)
	for _, required := range []string{
		"matcher_modifier:not", "subscript_observation:index",
		"public_observation:textContent", "Archived", "4",
	} {
		if !strings.Contains(projection, required) {
			t.Fatalf("matcher expression omitted %q: %s", required, projection)
		}
	}
	query := siteForOperation(t, inventory, "testing_library_query:getAllByRole")
	if !strings.Contains(acceptanceSiteProjection(query), "row") {
		t.Fatalf("static template query value was omitted: %+v", query)
	}
	for _, forbidden := range []string{"VerifyArchive"} {
		if strings.Contains(inventory.canonicalModelProjection(), forbidden) {
			t.Fatalf("complete expression leaked private source identity %q", forbidden)
		}
	}
}

func TestAcceptanceObservationInventoryRejectsTestingLibraryNamesOnPrivateReceivers(t *testing.T) {
	t.Parallel()

	for name, source := range map[string]string{
		"role impostor": `function VerifyImpostor(): void {
  const privateProbe = { getByRole: () => ({}) };
  expect(privateProbe.getByRole("button")).toBeDefined();
}`,
		"text impostor": `async function VerifyHelper(): Promise<void> {
  const privateHelper = { findByText: async () => ({}) };
  expect(await privateHelper.findByText("Ready")).toBeTruthy();
}`,
		"shadowed globals": `function VerifyShadowed(): void {
  const screen = { getByRole: (_role: string) => ({}) };
  const expect = (_value: unknown) => ({ toBeInTheDocument: () => undefined });
  expect(screen.getByRole("button")).toBeInTheDocument();
}`,
		"destructured and parameter shadows": `function VerifyBindings(): void {
  const { fireEvent } = { fireEvent: { click: (_value: unknown) => undefined } };
  const invoke = (waitFor: (_callback: () => void) => void) => waitFor(() => undefined);
  fireEvent.click("not a public element");
  invoke((_callback) => undefined);
}`,
	} {
		t.Run(name, func(t *testing.T) {
			context := ApplicationTaskContext{
				WorkloadSHA256: strings.Repeat("f", 64),
				Task: ApplicationTaskContextTask{TaskID: "task_009", AcceptanceCriteria: []string{
					"The required public state is observable.",
				}},
			}
			input, err := NewApplicationAcceptanceGroundingReviewInput(context, source, false, nil)
			if err != nil {
				t.Fatal(err)
			}
			projection := input.Inventory.canonicalModelProjection()
			if !strings.Contains(projection, "untrusted_call") {
				t.Fatalf("private query receiver was trusted: %s", projection)
			}
			for _, forbidden := range []string{"privateProbe", "privateHelper"} {
				if strings.Contains(projection, forbidden) {
					t.Fatalf("private query receiver leaked %q: %s", forbidden, projection)
				}
			}
			mappings := make([]AcceptanceGroundingMapping, 0, len(input.reviewSites()))
			for _, site := range input.reviewSites() {
				mappings = append(mappings, AcceptanceGroundingMapping{
					SiteID: site.ID, AuthorityIDs: []string{"criterion_001"},
				})
			}
			raw, err := json.Marshal(ApplicationAcceptanceGroundingReview{
				Decision: AcceptanceGroundingAccept, Mappings: mappings,
			})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := DecodeApplicationAcceptanceGroundingReview(input, string(raw)); err == nil {
				t.Fatal("private query receiver obtained an accepted grounding receipt")
			}
		})
	}
}

func TestAcceptanceObservationInventoryProjectsEventAndPublicValueSemantics(t *testing.T) {
	t.Parallel()

	source := `function VerifyFilter(): void {
  fireEvent.change(screen.getByLabelText(` + "`Filter records`" + `), { target: { value: ` + "`open`" + ` } });
  expect(screen.getByLabelText(` + "`Filter records`" + `).value).toBe(` + "`open`" + `);
}`
	inventory, err := InventoryTypeScriptAcceptanceObservations(source, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		"fire_event:change", "event_payload:target", "event_payload:value",
		"Filter records", "open", "public_observation:value",
	} {
		if !strings.Contains(inventory.canonicalModelProjection(), required) {
			t.Fatalf("event expression omitted %q: %s", required, inventory.canonicalModelProjection())
		}
	}
}

func TestAcceptanceObservationInventoryMarksDynamicOrUnknownSemanticsUntrusted(t *testing.T) {
	t.Parallel()

	for name, source := range map[string]string{
		"interpolated template": `function VerifyDynamic(): void {
  expect(screen.getByText(` + "`Status ${privateState}`" + `)).toBeVisible();
}`,
		"unknown property": `function VerifyPrivate(): void {
  expect(screen.getByRole("status").privateState).toBe("ready");
}`,
		"unknown modifier": `function VerifyEventually(): void {
  expect(screen.getByRole("status")).eventually.toBeVisible();
}`,
	} {
		t.Run(name, func(t *testing.T) {
			inventory, err := InventoryTypeScriptAcceptanceObservations(source, false)
			if err != nil {
				t.Fatal(err)
			}
			projection := inventory.canonicalModelProjection()
			if !strings.Contains(projection, "untrusted_call") {
				t.Fatalf("unclassified semantics were trusted: %s", projection)
			}
			for _, forbidden := range []string{"privateState", "eventually"} {
				if strings.Contains(projection, forbidden) {
					t.Fatalf("untrusted semantics leaked private vocabulary %q: %s", forbidden, projection)
				}
			}
		})
	}
}

func acceptanceSiteProjection(site AcceptanceObservationSite) string {
	parts := append([]string(nil), site.Operations...)
	parts = append(parts, site.Structure...)
	parts = append(parts, site.Operators...)
	for _, literal := range site.Literals {
		parts = append(parts, literal.Kind, literal.Value)
	}
	return strings.Join(parts, " ")
}
