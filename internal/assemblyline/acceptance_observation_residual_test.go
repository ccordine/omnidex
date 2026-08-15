package assemblyline

import (
	"strings"
	"testing"
)

func TestAcceptanceObservationInventoryCoversControlFlowWithoutRecognizedQuery(t *testing.T) {
	t.Parallel()

	source := `function VerifySchedule(): void {
  for (let index = 0; index < 3; index += 1) {
    if (index === 2) throw new Error("missing slot");
  }
  expect(true).toBe(true);
}`
	inventory, err := InventoryTypeScriptAcceptanceObservations(source, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(inventory.Sites) != 3 {
		t.Fatalf("sites=%+v", inventory.Sites)
	}
	projection := inventory.canonicalModelProjection()
	for _, required := range []string{
		"for_statement", "if_statement", "throw_statement", "binary_expression",
		`"3"`, `"2"`, "untrusted_call",
	} {
		if !strings.Contains(projection, required) {
			t.Fatalf("control-flow inventory omitted %q: %s", required, projection)
		}
	}
	operators := strings.Join(inventory.Sites[0].Operators, ",")
	for _, required := range []string{"<", "===", "+="} {
		if !strings.Contains(operators, required) {
			t.Fatalf("control-flow inventory omitted operator %q: %+v", required, inventory.Sites[0])
		}
	}
}

func TestAcceptanceObservationInventoryExposesControlInsideWaitCallbackSeparately(t *testing.T) {
	t.Parallel()

	source := `async function VerifyRows(): Promise<void> {
  await waitFor(() => {
    for (let index = 0; index < 4; index += 1) {
      if (index === 3) expect(screen.getByRole("row")).toBeVisible();
    }
  });
}`
	inventory, err := InventoryTypeScriptAcceptanceObservations(source, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(inventory.Sites) != 4 {
		t.Fatalf("callback control was not independently inventoried: %+v", inventory.Sites)
	}
	residual := inventory.Sites[0]
	projection := residualProjection(residual)
	for _, required := range []string{"for_statement", "if_statement", "4", "3"} {
		if !strings.Contains(projection, required) {
			t.Fatalf("callback control site omitted %q: %s", required, projection)
		}
	}
	if !stringInSet("untrusted_call", residual.Operations) {
		t.Fatalf("callback control was reviewable as trusted: %+v", residual)
	}
	wait := siteForOperation(t, inventory, "harness_call:waitFor")
	if len(wait.Literals) != 0 || !stringInSet("untrusted_call", wait.Operations) {
		t.Fatalf("invalid wait harness was prebindable: %+v", wait)
	}
	for _, operation := range []string{
		"testing_library_query:getByRole", "expect_matcher:toBeVisible",
	} {
		site := siteForOperation(t, inventory, operation)
		if site.ID == residual.ID || !stringInSet("untrusted_call", site.Operations) {
			t.Fatalf("nested invalid observation %s was merged or trusted: %+v", operation, site)
		}
	}
}

func siteForOperation(
	t *testing.T,
	inventory AcceptanceObservationInventory,
	operation string,
) AcceptanceObservationSite {
	t.Helper()
	for _, site := range inventory.Sites {
		if stringInSet(operation, site.Operations) {
			return site
		}
	}
	t.Fatalf("operation %s absent from %+v", operation, inventory.Sites)
	return AcceptanceObservationSite{}
}

func residualProjection(site AcceptanceObservationSite) string {
	parts := append([]string(nil), site.Structure...)
	parts = append(parts, site.Operators...)
	for _, literal := range site.Literals {
		parts = append(parts, literal.Value)
	}
	return strings.Join(parts, " ")
}
