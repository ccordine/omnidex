package queue

import (
	"os"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/scrumcardllm"
)

func TestScrumCardJobFieldsHaveOneAuthoritativeAction(t *testing.T) {
	tests := []struct {
		field  ScrumCardJobField
		column string
		action string
	}{
		{ScrumCardTagsJob, "tags_job_id", scrumcardllm.ActionTagsSuggest},
		{ScrumCardTicketJob, "ticket_job_id", scrumcardllm.ActionCardTicket},
	}
	for _, tc := range tests {
		column, err := tc.field.column()
		if err != nil {
			t.Fatal(err)
		}
		action, err := tc.field.action()
		if err != nil {
			t.Fatal(err)
		}
		if column != tc.column || action != tc.action {
			t.Fatalf("field=%d column=%q action=%q", tc.field, column, action)
		}
	}
}

func TestScrumCardMutationsAreAtomicAndConcurrencyChecked(t *testing.T) {
	sourceBytes, err := os.ReadFile("projects.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(sourceBytes)
	for _, required := range []string{
		"WITH inserted_card AS",
		"WITH updated_card AS",
		"AND updated_at = $26",
		"WITH deleted_card AS",
		"touched_project AS",
	} {
		if !strings.Contains(source, required) {
			t.Errorf("Scrum card mutation authority is missing %q", required)
		}
	}
	if strings.Contains(source, "_ = r.TouchProject") {
		t.Fatal("Scrum card mutations must not silently ignore project touch failures")
	}
	flowSource, err := os.ReadFile("scrum_flow.go")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(flowSource), "SET flow_metrics = $3::jsonb, updated_at = NOW()") {
		t.Fatal("derived flow metrics must not create false optimistic-lock conflicts")
	}
}

func TestScrumCardJobFieldRejectsUnknownValue(t *testing.T) {
	unknown := ScrumCardJobField(255)
	if _, err := unknown.column(); err == nil {
		t.Fatal("unknown field column must fail")
	}
	if _, err := unknown.action(); err == nil {
		t.Fatal("unknown field action must fail")
	}
}
