package assemblyline

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProductionHasNoWholeGroundedAnswerGenerationOrBindingStation(t *testing.T) {
	t.Parallel()
	forbidden := []string{
		"WorkGroundedAnswerText",
		"WorkGroundedAnswerEvidenceRelation",
		"GroundedAnswerTextInput",
		"GroundedAnswerTextDecision",
		"GroundedAnswerEvidenceRelationInput",
		"GroundedAnswerEvidenceRelationDecision",
		"GroundedEvidenceSupportsAnswer",
		"NewGroundedAnswerTextJob",
		"NewGroundedAnswerEvidenceRelationJob",
		"BuildGroundedAnswerTextPrompt",
		"BuildGroundedAnswerEvidenceRelationPrompt",
		"DecodeGroundedAnswerTextDecision",
		"DecodeGroundedAnswerEvidenceRelationDecision",
		`"grounded_answer_text"`,
		`"grounded_answer_evidence_relation"`,
		`'grounded_answer_text'`,
		`'grounded_answer_evidence_relation'`,
	}
	root := filepath.Clean(filepath.Join("..", ".."))
	files := make([]string, 0)
	err := filepath.WalkDir(filepath.Join(root, "internal"), func(
		path string,
		entry fs.DirEntry,
		err error,
	) error {
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
		filepath.Join(root, "docs", "CHARMELEON-INVARIANTS.md"),
	)
	for _, path := range files {
		source, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		for _, retired := range forbidden {
			if strings.Contains(string(source), retired) {
				t.Fatalf("production source %s retains whole-answer authority %q", path, retired)
			}
		}
	}
}
