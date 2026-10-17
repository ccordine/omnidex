package queue

import (
	"testing"

	"github.com/gryph/omnidex/internal/codingagent"
)

func TestLedgerCompletionBindsLocalParserWithoutInventingNativeSessions(t *testing.T) {
	entries, tasks := codingTaskLedgerCompletionJournalFixture()
	configuration := codingAgentTestConfiguration()
	configuration.LedgerParser = codingagent.Route{Provider: "ollama", Model: "local-parser:7b"}
	for index := range entries {
		entry := &entries[index]
		if entry.Role != codingagent.ReviewResponse {
			continue
		}
		if entry.Assignment != nil {
			entry.Assignment.Route = configuration.LedgerParser
		}
		if entry.Result != nil {
			entry.Result.Provider, entry.Result.Model = "ollama", "local-parser:7b"
			entry.Result.ThreadID, entry.Result.TurnID = "", ""
		}
	}
	if err := validateCodingTaskLedgerCompletionJournal(entries, 1, configuration, tasks, map[string]bool{"task-1": true}); err != nil {
		t.Fatalf("local parser was required to invent native provenance: %v", err)
	}
	for index := range entries {
		entry := &entries[index]
		if entry.Role == codingagent.Review && entry.Result != nil {
			entry.Result.ThreadID, entry.Result.TurnID = "", ""
		}
	}
	if err := validateCodingTaskLedgerCompletionJournal(entries, 1, configuration, tasks, map[string]bool{"task-1": true}); err == nil {
		t.Fatal("native review acceptance lost its isolated-session requirement")
	}
}
