package api

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRetiredScrumInferenceHasNoProductionBrowserClient(t *testing.T) {
	t.Parallel()
	var production strings.Builder
	err := filepath.WalkDir("web/src", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || (!strings.HasSuffix(path, ".ts") && !strings.HasSuffix(path, ".tsx")) ||
			strings.Contains(path, ".test.") {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		production.Write(raw)
		production.WriteByte('\n')
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{
		"cardTicketScrumCard", "ScrumCardLlm", "iterate_notes",
		"tags-suggest", "/coach", "/coach-config",
		"planning-chat", "auto_review", "create_ticket",
	} {
		if strings.Contains(production.String(), forbidden) {
			t.Errorf("production browser retains retired Scrum inference capability %q", forbidden)
		}
	}
}

func TestRetiredScrumInferenceRuntimeIsAbsent(t *testing.T) {
	t.Parallel()
	for _, path := range []string{
		"scrum_auto_review.go",
		"scrum_card_llm_enqueue.go",
		"scrum_card_llm_reconcile.go",
		"scrum_card_llm_refresh.go",
		"scrum_coach.go",
		"scrum_coach_config.go",
		"../worker/scrum_card_llm.go",
		"../worker/scrum_card_llm_enqueue.go",
	} {
		if _, err := os.Stat(path); err == nil {
			t.Errorf("retired Scrum inference runtime remains: %s", path)
		} else if !os.IsNotExist(err) {
			t.Fatalf("inspect %s: %v", path, err)
		}
	}
	entries, err := os.ReadDir("../scrumcardllm")
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".go") {
			t.Errorf("retired Scrum inference package file remains: %s", entry.Name())
		}
	}
}
