package worker

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/queue"
)

func TestScrumCardLLMContextRejectsMalformedDurableState(t *testing.T) {
	card := queue.DBScrumCard{
		ID:           "card-1",
		Title:        "Card",
		RefFiles:     json.RawMessage(`{"not":"an array"}`),
		Tags:         json.RawMessage(`[]`),
		Checklist:    json.RawMessage(`[]`),
		TestCriteria: json.RawMessage(`[]`),
	}
	if _, err := dbScrumCardLLMContext(card); err == nil {
		t.Fatal("expected malformed ref_files to fail")
	}
}

func TestScrumCardLLMWorkerHasNoModelOrPersistenceFallbacks(t *testing.T) {
	raw, err := os.ReadFile("scrum_card_llm.go")
	if err != nil {
		t.Fatalf("read worker source: %v", err)
	}
	source := string(raw)
	for _, forbidden := range []string{
		"CoachModelName",
		"TicketModelName",
		"ParseCoachModel",
		"scrumCardTicketLLM",
		"mergeScrumCardProjectTags",
		"ListScrumCards(ctx, projectID)",
		"_ = json.",
		"_ = s.",
		"qwen3:4b-thinking",
		"llama3.2",
	} {
		if strings.Contains(source, forbidden) {
			t.Errorf("Scrum card LLM worker contains forbidden fallback path %q", forbidden)
		}
	}
}
