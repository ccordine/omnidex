package queue

import (
	"os"
	"strings"
	"testing"
)

func TestMemoryPromotionSchemaBindsCandidateAndMemoryAtomically(t *testing.T) {
	raw, err := os.ReadFile("../../migrations/029_atomic_memory_candidate_promotion.sql")
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	for _, required := range []string{
		"promoted_memory_id BIGINT",
		"memory_candidates_promoted_memory_id_fkey",
		"ON DELETE RESTRICT",
		"UNIQUE (promoted_memory_id)",
		"memory_candidates_promotion_shape",
		"status IN ('approved', 'durable') AND promoted_memory_id IS NOT NULL",
		"cannot bind previously split promotions",
	} {
		if !strings.Contains(source, required) {
			t.Errorf("atomic promotion migration missing %q", required)
		}
	}
}

func TestMemoryPromotionCallersContainNoSplitPublishThenStatusPath(t *testing.T) {
	worker := readFunctionSource(t, "../worker/runtime_v3_completion.go", "func (r *nativeRuntimeV3) runMemoryReview", "func (r *nativeRuntimeV3) runFinalize")
	api := readFunctionSource(t, "../api/server_jobs.go", "func (s *Server) promoteMemoryCandidate", "func (s *Server) rejectMemoryCandidate")
	for name, source := range map[string]string{"worker": worker, "api": api} {
		for _, forbidden := range []string{"AddMemoryChunk(", "UpdateCurrentMemoryCandidateStatus("} {
			if strings.Contains(source, forbidden) {
				t.Errorf("%s promotion still contains split path %q", name, forbidden)
			}
		}
		promotion := "PromoteCurrentMemoryCandidate("
		if name == "worker" {
			promotion = "PromoteCurrentMemoryCandidateByStepAttempt("
		}
		if !strings.Contains(source, promotion) {
			t.Errorf("%s promotion omits the atomic current-generation path", name)
		}
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
