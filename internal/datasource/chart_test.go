package datasource

import "testing"

func TestBuildChartSpecBarChart(t *testing.T) {
	spec := BuildChartSpec(QueryResult{
		Answer:  "Top players",
		Columns: []string{"player", "score"},
		Rows: []map[string]any{
			{"player": "Alice", "score": 120},
			{"player": "Bob", "score": 95},
		},
		Count: 2,
	})
	if spec == nil {
		t.Fatal("expected chart spec")
	}
	if spec.Type != "bar" && spec.Type != "pie" {
		t.Fatalf("type = %q", spec.Type)
	}
	if len(spec.Series) != 2 {
		t.Fatalf("series = %#v", spec.Series)
	}
}
