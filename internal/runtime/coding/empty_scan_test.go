package coding

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestEmptyScannerOnlyReportsAndPlannerDecides(t *testing.T) {
	assertEmptyScannerOnlyReportsAndPlannerDecides(t)
}

func TestEmptyScanIsPlannerOwned(t *testing.T) {
	assertEmptyScannerOnlyReportsAndPlannerDecides(t)
}

func assertEmptyScannerOnlyReportsAndPlannerDecides(t *testing.T) {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "empty.go"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "full.go"), []byte("package test\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	report, err := NewEmptyFileScanner(root).ScanEmptyFiles(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(report.Files, []string{"empty.go"}) {
		t.Fatalf("empty scanner report=%+v want only empty.go", report)
	}

	reportType := reflect.TypeOf(report)
	if reportType.NumField() != 1 || reportType.Field(0).Name != "Files" {
		t.Fatalf("empty scanner report must only report files, got fields=%v", reportType.NumField())
	}

	planner := deterministicPlanner{}
	disposition, err := planner.Disposition(context.Background(), CodingPlan{Goal: "decide empties"}, report)
	if err != nil {
		t.Fatal(err)
	}
	if len(disposition.Actions) != 1 {
		t.Fatalf("planner disposition actions=%+v want one action", disposition.Actions)
	}
	if disposition.Actions[0].Path != "empty.go" || disposition.Actions[0].Action == "" {
		t.Fatalf("planner did not decide action for empty file: %+v", disposition.Actions[0])
	}
}
