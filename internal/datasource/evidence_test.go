package datasource

import "testing"

func TestPickChartResultPrefersChartableStep(t *testing.T) {
	result := QueryResult{
		Question: "counts by status",
		Columns:  []string{"id"},
		Rows:     []map[string]any{{"id": "x"}},
		QuerySteps: []QueryStep{
			{
				Step:    1,
				Columns: []string{"status", "count"},
				Rows: []map[string]any{
					{"status": "active", "count": 10},
					{"status": "pending", "count": 4},
				},
			},
		},
	}
	got := pickChartResult(result)
	if len(got.Columns) != 2 || got.Columns[0] != "status" {
		t.Fatalf("expected chartable step columns, got %#v", got.Columns)
	}
}

func TestFinalizeQueryResultBuildsEvidence(t *testing.T) {
	got := FinalizeQueryResult(QueryResult{
		Question: "How many active?",
		Answer:   "12 active records.",
		QuerySteps: []QueryStep{
			{Step: 1, SQL: "SELECT COUNT(*) FROM t", RowCount: 1, HardFacts: []string{"rows=1", "r1=count=12"}},
		},
		HardFacts: []string{"q1 rows=1", "q1 r1=count=12"},
		Columns:   []string{"count"},
		Rows:      []map[string]any{{"count": 12}},
		Count:     1,
	})
	if got.Evidence.StepCount != 1 {
		t.Fatalf("evidence steps=%d", got.Evidence.StepCount)
	}
	if got.Evidence.Confidence == "" {
		t.Fatal("expected confidence")
	}
	if !containsAll(got.Answer, "Evidence:") {
		t.Fatalf("expected evidence prefix in answer: %q", got.Answer)
	}
}

func TestFinalizeQueryResultIdempotent(t *testing.T) {
	once := FinalizeQueryResult(QueryResult{
		Question: "q",
		Answer:   "a",
		Columns:  []string{"n"},
		Rows:     []map[string]any{{"n": 1}},
		Count:    1,
	})
	twice := FinalizeQueryResult(once)
	if twice.Answer != once.Answer {
		t.Fatalf("double finalize changed answer: %q vs %q", once.Answer, twice.Answer)
	}
}
