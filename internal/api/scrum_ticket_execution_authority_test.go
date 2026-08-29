package api

import (
	"os"
	"strings"
	"testing"
)

func TestDisplayOnlyCardTicketCannotEnterExecutionContext(t *testing.T) {
	t.Parallel()
	for _, path := range []string{
		"scrum_service.go",
		"scrum_manager.go",
		"../scrum/context.go",
		"../worker/v3_coding_runtime.go",
	} {
		source, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{
			"scrum_card_ticket", "CardTicket: card.CardTicket",
			"scrum_card_tags", "Tags: card.Tags",
			"planning_chat", "PlanningChat:",
			"coach_config", "CoachConfig:",
		} {
			if strings.Contains(string(source), forbidden) {
				t.Errorf("%s sends display-only ticket through execution authority %q", path, forbidden)
			}
		}
	}
	for _, retired := range []string{
		"../worker/external_agent.go",
		"scrum_play_agent.go",
		"scrum_agent_stream.go",
	} {
		if _, err := os.Stat(retired); !os.IsNotExist(err) {
			t.Errorf("retired execution source %s must be absent, stat error=%v", retired, err)
		}
	}
}
