package codingagent

import "testing"

func TestNormalizeReviewFindingFiles(t *testing.T) {
	tasks := map[string]Task{
		"cli": {
			ID:          "cli",
			WriteFiles:  []string{"cli.go"},
			ReadFiles:   []string{"internal/model.go"},
			DeleteFiles: []string{"old:name.go"},
		},
	}
	tests := []struct {
		name string
		file string
		want string
	}{
		{name: "exact", file: "cli.go", want: "cli.go"},
		{name: "line", file: "cli.go:172", want: "cli.go"},
		{name: "line range", file: "cli.go:172-175", want: "cli.go"},
		{name: "line and column", file: "internal/model.go:12:4", want: "internal/model.go"},
		{name: "range with columns", file: "internal/model.go:12:4-15:8", want: "internal/model.go"},
		{name: "authorized colon in name", file: "old:name.go:9", want: "old:name.go"},
		{name: "ungranted", file: "other.go:12", want: "other.go:12"},
		{name: "non numeric", file: "cli.go:main", want: "cli.go:main"},
		{name: "zero", file: "cli.go:0", want: "cli.go:0"},
		{name: "extra suffix", file: "cli.go:12:3:2", want: "cli.go:12:3:2"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := ReviewResult{Findings: []Finding{{TaskID: "cli", File: test.file}}}
			normalizeReviewFindingFiles(tasks, &result)
			if got := result.Findings[0].File; got != test.want {
				t.Fatalf("file = %q, want %q", got, test.want)
			}
		})
	}
}

func TestNormalizedReviewFindingRetainsExactScopeValidation(t *testing.T) {
	tasks := map[string]Task{
		"cli": {
			ID:         "cli",
			Acceptance: []string{"output is atomic"},
			WriteFiles: []string{"cli.go"},
		},
	}
	result := ReviewResult{
		Findings:  []Finding{{TaskID: "cli", File: "cli.go:172-175", Requirement: "output is atomic", Observation: "store changed after output failed"}},
		Questions: []string{},
	}
	normalizeReviewFindingFiles(tasks, &result)
	if err := ValidateReview(tasks, result); err != nil {
		t.Fatalf("authorized line reference rejected: %v", err)
	}
	if result.Findings[0].File != "cli.go" {
		t.Fatalf("normalized file = %q", result.Findings[0].File)
	}

	result.Findings[0].File = "other.go:172-175"
	normalizeReviewFindingFiles(tasks, &result)
	if err := ValidateReview(tasks, result); err == nil {
		t.Fatal("ungranted line reference accepted")
	}
}
