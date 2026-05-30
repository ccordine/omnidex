package datasource

import "testing"

func TestCavemanTextTruncatesLongValues(t *testing.T) {
	got := cavemanText("one two three four five six seven eight nine ten eleven twelve", 30)
	if len(got) > 30 {
		t.Fatalf("cavemanText exceeded max: %q", got)
	}
	if !containsAll(got, "one", "twelve") {
		t.Fatalf("cavemanText lost anchors: %q", got)
	}
}

func TestMinifyQueryStepFactsBudget(t *testing.T) {
	rows := make([]map[string]any, 0, 20)
	for i := 0; i < 20; i++ {
		rows = append(rows, map[string]any{"status": "pending", "count": i})
	}
	facts := minifyQueryStepFacts(1, "count by status", "SELECT status, count(*) FROM t GROUP BY status", []string{"status", "count"}, rows, true)
	joined := stringsJoin(facts, " ")
	if len(joined) > PerStepFactBudget+40 {
		t.Fatalf("facts too large: %d chars", len(joined))
	}
	if !containsAll(joined, "rows=20", "+8 more rows omitted") {
		t.Fatalf("missing row summary: %q", joined)
	}
}

func TestBuildInvestigationCavemanBlockBudget(t *testing.T) {
	steps := []QueryStep{
		{Step: 1, HardFacts: []string{repeat("fact ", 200)}},
		{Step: 2, HardFacts: []string{repeat("fact ", 200)}},
	}
	lines := buildInvestigationCavemanBlock("how many at risk", steps, 900)
	joined := stringsJoin(lines, "\n")
	if len(joined) > 950 {
		t.Fatalf("caveman block too large: %d", len(joined))
	}
	if !containsAll(joined, "CAVE MAN SUMMARY", "question:") {
		t.Fatalf("missing header: %q", joined)
	}
}

func TestFilterDisplayColumnsStrict(t *testing.T) {
	cols := filterDisplayColumns([]string{"patient_name", "status", "count"}, true)
	if len(cols) != 2 || cols[0] != "status" {
		t.Fatalf("unexpected cols: %#v", cols)
	}
}

func containsAll(text string, parts ...string) bool {
	for _, part := range parts {
		if !stringsContains(text, part) {
			return false
		}
	}
	return true
}

func stringsJoin(parts []string, sep string) string {
	out := ""
	for i, part := range parts {
		if i > 0 {
			out += sep
		}
		out += part
	}
	return out
}

func stringsContains(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func repeat(s string, n int) string {
	out := ""
	for i := 0; i < n; i++ {
		out += s
	}
	return out
}
