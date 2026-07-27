package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScrumRequiresPostgresRepository(t *testing.T) {
	server := NewServer(nil, &fakeLLMClient{})
	req := httptest.NewRequest(http.MethodGet, "/v1/scrum?project_id=1", nil)
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d want=%d body=%s", rec.Code, http.StatusServiceUnavailable, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "postgres repository is required") {
		t.Fatalf("missing explicit PostgreSQL requirement: %s", rec.Body.String())
	}
}

func TestScrumProductionSourceHasNoLocalPersistenceFallback(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read API source directory: %v", err)
	}
	var source strings.Builder
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		content, readErr := os.ReadFile(filepath.Join(".", entry.Name()))
		if readErr != nil {
			t.Fatalf("read %s: %v", entry.Name(), readErr)
		}
		source.Write(content)
	}
	for _, forbidden := range []string{
		"type ScrumStore struct",
		"NewScrumStore(",
		"scrumStore *ScrumStore",
		"OMNI_SCRUM_ROOT",
		`json:"jira_ticket`,
		`json:"jira_prompt`,
		"Agent run completed (direct mode)",
		"Running job could not accept steer",
		"_, _ = s.repo.CancelJob",
		"scrumAutoPlayThroughKey",
		`json:"auto_play_through"`,
		"project, err = s.repo.GetProjectByLocation",
	} {
		if strings.Contains(source.String(), forbidden) {
			t.Errorf("Scrum production source contains forbidden local/legacy persistence path %q", forbidden)
		}
	}
}

func TestScrumQueueSourceDoesNotSwallowRepositoryFailures(t *testing.T) {
	source := readAPISource(t, "scrum_play_queue.go")
	for _, forbidden := range []string{
		"if saved, err := s.persistScrumCard",
		"if reviewed, err := s.maybeStartScrumAutoReview",
		"if jobID, err := parseJobID",
		"return board, nil\n\t}\n\tfor i, card := range board.Cards",
	} {
		if strings.Contains(source, forbidden) {
			t.Errorf("Scrum queue source contains a swallowed failure path %q", forbidden)
		}
	}
}
