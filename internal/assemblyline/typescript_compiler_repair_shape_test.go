package assemblyline

import (
	"strconv"
	"strings"
	"testing"
)

func TestTypeScriptCompilerRepairPreservesExpressionOwnerShape(t *testing.T) {
	t.Parallel()
	for _, fixture := range []struct {
		name        string
		current     string
		start, end  int
		replacement string
	}{
		{
			name: "event bridge call",
			current: strings.Join([]string{
				"function Connect(value: Value): void {",
				"  subscribe(() => {",
				"    apply(value);",
				"  });",
				"}",
			}, "\n"),
			start: 2, end: 4,
			replacement: strings.Join([]string{
				"  subscribe(() => {",
				"    apply(String(value));",
				"  });",
			}, "\n"),
		},
		{
			name: "persistence call",
			current: strings.Join([]string{
				"function Save(record: RecordValue): void {",
				"  persist(record.value);",
				"}",
			}, "\n"),
			start: 2, end: 2,
			replacement: "  persist(String(record.value));",
		},
		{
			name: "await removed from non async operation",
			current: strings.Join([]string{
				"function Refresh(): void {",
				"  await refresh();",
				"}",
			}, "\n"),
			start: 2, end: 2,
			replacement: "  refresh();",
		},
		{
			name: "typescript angle assertion operation",
			current: strings.Join([]string{
				"function Normalize(value: unknown): void {",
				"  <string>value;",
				"}",
			}, "\n"),
			start: 2, end: 2,
			replacement: "  String(value);",
		},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			lines := strings.Split(fixture.current, "\n")
			region := TypeScriptFragmentRepairRegion{
				Kind:      TypeScriptRepairRegionCompilerOwner,
				StartLine: fixture.start, EndLine: fixture.end,
				Source:   strings.Join(lines[fixture.start-1:fixture.end], "\n"),
				Bindings: typeScriptCompilerRepairTestBindings(),
			}
			projected, err := ProjectTypeScriptFragmentRepairResponse(region, fixture.replacement)
			if err != nil || projected != fixture.replacement {
				t.Fatalf("valid call-shaped replacement=%q err=%v", projected, err)
			}
			if _, err := ApplyTypeScriptFragmentRepairRegion(
				fixture.current, region, fixture.replacement,
			); err != nil {
				t.Fatalf("apply valid call-shaped replacement: %v", err)
			}

			quoted := strconv.Quote(fixture.replacement)
			if _, err := ProjectTypeScriptFragmentRepairResponse(region, quoted); err == nil ||
				!strings.Contains(err.Error(), "string literal statement") {
				t.Fatalf("JSON-quoted replacement projection error=%v", err)
			}
			if _, err := ApplyTypeScriptFragmentRepairRegion(
				fixture.current, region, quoted,
			); err == nil || !strings.Contains(err.Error(), "string literal statement") {
				t.Fatalf("JSON-quoted replacement application error=%v", err)
			}
		})
	}
}
