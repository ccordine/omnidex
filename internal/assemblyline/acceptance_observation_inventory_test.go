package assemblyline

import (
	"reflect"
	"strings"
	"testing"
)

func TestAcceptanceObservationInventoryIsSourceFreeAndDeterministic(t *testing.T) {
	t.Parallel()

	source := `async function VerifyCatalog(): Promise<void> {
  screen.getByRole("textbox", { name: "Filter records" });
  fireEvent.change(screen.getByRole("textbox", { name: "Filter records" }), { target: { value: "open" } });
  await waitFor(() => {
    expect(screen.getAllByRole("row")).toHaveLength(2);
  });
}`
	first, err := InventoryTypeScriptAcceptanceObservations(source, true)
	if err != nil {
		t.Fatal(err)
	}
	second, err := InventoryTypeScriptAcceptanceObservations(source, true)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("inventory is not deterministic:\nfirst=%+v\nsecond=%+v", first, second)
	}
	if len(first.Sites) != 6 {
		t.Fatalf("sites=%+v", first.Sites)
	}
	for index, site := range first.Sites {
		wantSite := "site_00" + string(rune('1'+index))
		wantAssertion := "assertion_00" + string(rune('1'+index))
		if site.ID != wantSite || site.AssertionID != wantAssertion {
			t.Fatalf("site %d identity=%s/%s", index, site.ID, site.AssertionID)
		}
	}
	encoded := first.canonicalModelProjection()
	for _, forbidden := range []string{"function VerifyCatalog", "Promise<void>"} {
		if strings.Contains(encoded, forbidden) {
			t.Fatalf("source-free inventory leaked %q: %s", forbidden, encoded)
		}
	}
	for _, required := range []string{
		"harness_call:waitFor",
		"testing_library_query:getByRole", "fire_event:change",
		"testing_library_query:getAllByRole", "expect_matcher:toHaveLength",
		"Filter records", "open", "row", `"2"`,
	} {
		if !strings.Contains(encoded, required) {
			t.Fatalf("inventory omitted %q: %s", required, encoded)
		}
	}
}

func TestAcceptanceObservationInventoryIgnoresCommentsButKeepsWaitHarness(t *testing.T) {
	t.Parallel()

	source := `async function VerifyBoard(): Promise<void> {
  // This preference is not an acceptance obligation.
  await waitFor(() => expect(screen.getByRole("status")).toHaveTextContent("Ready"));
}`
	inventory, err := InventoryTypeScriptAcceptanceObservations(source, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(inventory.Sites) != 3 {
		t.Fatalf("comment became an authority site: %+v", inventory.Sites)
	}
	projection := inventory.canonicalModelProjection()
	for _, required := range []string{
		"harness_call:waitFor", "expect_matcher:toHaveTextContent",
	} {
		if !strings.Contains(projection, required) {
			t.Fatalf("inventory omitted %q: %s", required, projection)
		}
	}
	if strings.Contains(projection, "preference") || strings.Contains(projection, "comment") {
		t.Fatalf("comment entered inventory: %s", projection)
	}
}

func TestAcceptanceObservationInventoryRejectsRemovedHarnessOperations(t *testing.T) {
	t.Parallel()

	source := `function VerifyRemovedHarness(): void {
  render(<section />);
  createFeatureRuntime(createApplicationRuntime(), "opaque");
}`
	inventory, err := InventoryTypeScriptAcceptanceObservations(source, true)
	if err != nil {
		t.Fatal(err)
	}
	projection := inventory.canonicalModelProjection()
	for _, forbidden := range []string{
		"harness_call:render", "harness_call:createFeatureRuntime", "harness_call:createApplicationRuntime",
	} {
		if strings.Contains(projection, forbidden) {
			t.Fatalf("removed harness operation survived: %s", projection)
		}
	}
	if !strings.Contains(projection, "untrusted_call") {
		t.Fatalf("removed harness call was not marked untrusted: %s", projection)
	}
}

func TestAcceptanceObservationInventoryKeepsEveryNestedMatcherInOneSite(t *testing.T) {
	t.Parallel()

	source := `async function VerifySummary(): Promise<void> {
  await waitFor(() => {
    expect(screen.getByRole("status")).toHaveTextContent("Saved");
    expect(screen.getAllByRole("row")).toHaveLength(4);
  });
}`
	inventory, err := InventoryTypeScriptAcceptanceObservations(source, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(inventory.Sites) != 5 {
		t.Fatalf("nested observations were not independently inventoried: %+v", inventory.Sites)
	}
	projection := inventory.canonicalModelProjection()
	for _, required := range []string{
		"expect_matcher:toHaveTextContent", "expect_matcher:toHaveLength",
		"testing_library_query:getByRole", "testing_library_query:getAllByRole",
		"Saved", "row", `"4"`,
	} {
		if !strings.Contains(projection, required) {
			t.Fatalf("nested site omitted %q: %s", required, projection)
		}
	}
	matcherSites := 0
	querySites := 0
	for _, site := range inventory.Sites {
		if len(site.Operations) == 0 {
			t.Fatalf("atomic site has merged operations: %+v", site)
		}
		if strings.HasPrefix(site.Operations[0], "expect_matcher:") {
			matcherSites++
		}
		if strings.HasPrefix(site.Operations[0], "testing_library_query:") {
			querySites++
		}
	}
	if matcherSites != 2 || querySites != 2 {
		t.Fatalf("matcher/query sites=%d/%d inventory=%+v", matcherSites, querySites, inventory.Sites)
	}
	for _, site := range inventory.Sites {
		if len(site.Operations) == 1 && site.Operations[0] == "harness_call:waitFor" && len(site.Literals) != 0 {
			t.Fatalf("wait harness inherited nested product literals: %+v", site)
		}
	}
}

func TestResolveTypeScriptAcceptanceObservationSiteUsesSmallestNestedFailureSite(t *testing.T) {
	t.Parallel()

	source := `async function VerifySummary(): Promise<void> {
  await waitFor(() => {
    expect(screen.getByRole("status")).toHaveTextContent("Saved");
  });
}`
	inventory, err := InventoryTypeScriptAcceptanceObservations(source, true)
	if err != nil {
		t.Fatal(err)
	}
	matcherID := siteIDForOperation(t, inventory, "expect_matcher:toHaveTextContent")
	queryID := siteIDForOperation(t, inventory, "testing_library_query:getByRole")
	matcherColumn := strings.Index(strings.Split(source, "\n")[2], "expect") + 1
	queryColumn := strings.Index(strings.Split(source, "\n")[2], "screen") + 1
	for name, testCase := range map[string]struct {
		column int
		want   string
	}{
		"matcher": {column: matcherColumn, want: matcherID},
		"query":   {column: queryColumn, want: queryID},
	} {
		t.Run(name, func(t *testing.T) {
			got, mapped, err := ResolveTypeScriptAcceptanceObservationSite(source, true, 3, testCase.column)
			if err != nil {
				t.Fatal(err)
			}
			if !mapped || got != testCase.want {
				t.Fatalf("resolved=%q mapped=%v want=%q", got, mapped, testCase.want)
			}
		})
	}
	if got, mapped, err := ResolveTypeScriptAcceptanceObservationSite(source, true, 2, 3); err != nil || mapped || got != "" {
		t.Fatalf("pure wait harness resolved as product failure: id=%q mapped=%v error=%v", got, mapped, err)
	}
}

func siteIDForOperation(t *testing.T, inventory AcceptanceObservationInventory, operation string) string {
	t.Helper()
	for _, site := range inventory.Sites {
		if len(site.Operations) == 1 && site.Operations[0] == operation {
			return site.ID
		}
	}
	t.Fatalf("operation %s absent from %+v", operation, inventory.Sites)
	return ""
}
