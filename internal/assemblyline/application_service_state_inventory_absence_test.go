package assemblyline

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestApplicationServiceStateSourceHasNoRetiredCoverageOrSinglePurposeStations(t *testing.T) {
	t.Parallel()
	forbidden := []string{
		"WorkApplicationStateFieldCoverage",
		"WorkApplicationRecordFieldCoverage",
		"NewApplicationStateFieldCoverageJob",
		"NewApplicationRecordFieldCoverageJob",
		"DecodeApplicationStateFieldCoverageLeaf",
		"DecodeApplicationRecordFieldCoverageLeaf",
		"NewApplicationStateFieldPurposeJob(",
		"NewApplicationRecordFieldPurposeJob(",
		"DecodeApplicationStateFieldPurposeLeaf",
		"DecodeApplicationRecordFieldPurposeLeaf",
		"ApplicationStateFieldLeafInput",
		"ApplicationRecordFieldLeafInput",
		"CodingApplicationStateFieldCoverage",
		"CodingApplicationRecordFieldCoverage",
		"CodingApplicationStateFieldPurpose,",
		"CodingApplicationRecordFieldPurpose,",
		`"application_state_field_coverage"`,
		`"application_state_field_purpose"`,
		`"application_record_field_coverage"`,
		`"application_record_field_purpose"`,
		`"coding_application_state_field_coverage"`,
		`"coding_application_state_field_purpose"`,
		`"coding_application_record_field_coverage"`,
		`"coding_application_record_field_purpose"`,
		"'application_state_field_coverage'",
		"'application_state_field_purpose'",
		"'application_record_field_coverage'",
		"'application_record_field_purpose'",
		"'coding_application_state_field_coverage'",
		"'coding_application_state_field_purpose'",
		"'coding_application_record_field_coverage'",
		"'coding_application_record_field_purpose'",
		"STATE_FIELD_REMAINS",
		"NO_UNCOVERED_STATE_FIELD",
		"RECORD_FIELD_REMAINS",
		"NO_UNCOVERED_RECORD_FIELD",
		"state-field coverage",
		"record-field coverage",
	}
	root := filepath.Clean(filepath.Join("..", ".."))
	files := make([]string, 0)
	err := filepath.WalkDir(filepath.Join(root, "internal"), func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		files = append(files, path)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	files = append(files,
		filepath.Join(root, "database", "setup.sql"),
		filepath.Join(root, "docs", "LOCAL_MODEL_PROFILE.md"),
	)
	for _, file := range files {
		source, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		for _, retired := range forbidden {
			if strings.Contains(string(source), retired) {
				t.Fatalf("%s retains retired application state station source %q", file, retired)
			}
		}
	}
}
