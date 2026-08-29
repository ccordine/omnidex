package queue

import (
	"os"
	"strings"
	"testing"
)

func TestMemoryPromotionCallersContainNoSplitPublishThenStatusPath(t *testing.T) {
	api := readFunctionSource(t, "../api/server_jobs.go", "func (s *Server) promoteMemoryCandidate", "func (s *Server) rejectMemoryCandidate")
	for _, forbidden := range []string{"AddMemoryChunk(", "UpdateCurrentMemoryCandidateStatus("} {
		if strings.Contains(api, forbidden) {
			t.Errorf("API promotion still contains split path %q", forbidden)
		}
	}
	if !strings.Contains(api, "PromoteCurrentMemoryCandidate(") {
		t.Error("API promotion omits the atomic current-generation path")
	}
	for _, required := range []string{
		"PromoteHistoricalMemoryCandidate(",
		"PromoteGlobalMemoryCandidate(",
		"MemoryPromotionAuthorityHistorical",
		"MemoryPromotionAuthorityGlobal",
	} {
		if !strings.Contains(api, required) {
			t.Errorf("API promotion omits explicit authority path %q", required)
		}
	}
}

func TestSeparateCandidateStatusPromotionMethodIsAbsent(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		raw, err := os.ReadFile(entry.Name())
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(raw), "UpdateCurrentMemoryCandidateStatus") {
			t.Fatalf("obsolete split promotion status method remains in %s", entry.Name())
		}
	}
}

func readFunctionSource(t *testing.T, path, startToken, endToken string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	start := strings.Index(source, startToken)
	if start < 0 {
		t.Fatalf("%s missing start token %q", path, startToken)
	}
	endRelative := strings.Index(source[start+len(startToken):], endToken)
	if endRelative < 0 {
		t.Fatalf("%s missing end token %q", path, endToken)
	}
	return source[start : start+len(startToken)+endRelative]
}
