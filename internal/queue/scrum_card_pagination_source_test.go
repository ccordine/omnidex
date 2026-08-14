package queue

import (
	"os"
	"strings"
	"testing"
)

func TestScrumCardSummaryQueryDoesNotProjectGrowingBodies(t *testing.T) {
	t.Parallel()
	queryRaw, err := os.ReadFile("scrum_card_summary_query.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(queryRaw)
	for _, forbidden := range []string{
		"card.chat,", "card.planning_chat,", "card.console_log,",
		"jsonb_array_elements(card.chat)", "jsonb_array_elements(card.planning_chat)",
		"SELECT card.*", "SELECT *", "jsonb_agg", "array_agg",
	} {
		if strings.Contains(source, forbidden) {
			t.Errorf("Scrum card summary query projects forbidden growing state %q", forbidden)
		}
	}
	for _, required := range []string{"card.channel_message_count"} {
		if !strings.Contains(source, required) {
			t.Errorf("Scrum card summary query lacks database aggregate %q", required)
		}
	}
	paginationRaw, err := os.ReadFile("scrum_card_pagination.go")
	if err != nil {
		t.Fatal(err)
	}
	pagination := string(paginationRaw)
	for _, forbidden := range []string{"Items   []DBScrumCard\n", "scanDBScrumCard(rows)"} {
		if strings.Contains(pagination, forbidden) {
			t.Errorf("Scrum card paginator retains full-card authority %q", forbidden)
		}
	}
	for _, required := range []string{"Items   []DBScrumCardSummary", "scanDBScrumCardSummary(rows)"} {
		if !strings.Contains(pagination, required) {
			t.Errorf("Scrum card paginator lacks summary authority %q", required)
		}
	}
}

func TestScrumChannelTailQueryCannotScanTheGrowingPrefix(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("scrum_card_messages.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	for _, required := range []string{
		"WITH candidates AS MATERIALIZED", "ORDER BY ordinal DESC", "LIMIT $4",
	} {
		if !strings.Contains(source, required) {
			t.Errorf("Scrum message tail lacks bounded indexed authority %q", required)
		}
	}
	for _, forbidden := range []string{"ROW_NUMBER()"} {
		if strings.Contains(source, forbidden) {
			t.Errorf("Scrum message tail retains growing-prefix work %q", forbidden)
		}
	}
}
