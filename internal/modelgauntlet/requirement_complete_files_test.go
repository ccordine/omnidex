package modelgauntlet

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadCompleteRequirementFilesHashesCasesAndWithholdsLabels(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	casesPath := filepath.Join(directory, "cases.json")
	labelsPath := filepath.Join(directory, "labels.json")
	writeFixture(t, casesPath, `{
  "schema":"omnidex.model-gauntlet.complete-requirement-cases.v1",
  "cases":[{"id":"timer","source_text":"Build a timer with lap history."}]
}`)
	writeFixture(t, labelsPath, `{
  "schema":"omnidex.model-gauntlet.complete-requirement-labels.v1",
  "labels":[{"case_id":"timer","feature_quotes":["lap history"]}]
}`)

	cases, casesHash, err := LoadCompleteRequirementCases(casesPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(cases) != 1 || len(casesHash) != 64 {
		t.Fatalf("cases=%#v hash=%q", cases, casesHash)
	}
	labels, labelHash, err := LoadCompleteRequirementLabels(labelsPath, cases)
	if err != nil {
		t.Fatal(err)
	}
	if len(labels) != 1 || len(labelHash) != 64 {
		t.Fatalf("labels=%#v hash=%q", labels, labelHash)
	}
}

func TestLoadCompleteRequirementCasesRejectsUnknownFields(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "cases.json")
	if err := os.WriteFile(path, []byte(`{
  "schema":"omnidex.model-gauntlet.complete-requirement-cases.v1",
  "cases":[{"id":"timer","source_text":"Build a timer with lap history.","expected":["lap history"]}]
}`), 0o600); err != nil {
		t.Fatal(err)
	}
	_, _, err := LoadCompleteRequirementCases(path)
	if err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("error=%v", err)
	}
}

func TestCheckedInCompleteRequirementCorpusMeetsPromotionSize(t *testing.T) {
	t.Parallel()

	cases, casesHash, err := LoadCompleteRequirementCases("../../gauntlets/requirement_partition_complete/cases.v1.json")
	if err != nil {
		t.Fatal(err)
	}
	if len(cases) < minimumCompleteRequirementCases || len(cases) > maxCompleteRequirementCases {
		t.Fatalf("checked-in corpus contains %d cases", len(cases))
	}
	if len(casesHash) != 64 {
		t.Fatalf("cases hash=%q", casesHash)
	}
	labels, labelHash, err := LoadCompleteRequirementLabels(
		"../../gauntlets/requirement_partition_complete/labels.v1.json", cases,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(labels) != len(cases) || len(labelHash) != 64 {
		t.Fatalf("labels=%d cases=%d hash=%q", len(labels), len(cases), labelHash)
	}
}
