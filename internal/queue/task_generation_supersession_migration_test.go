package queue

import (
	"os"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/taskstate"
)

func TestTaskGenerationSupersessionMigrationDefinesImmutableEventBoundHistory(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("../../migrations/038_task_generation_supersession.sql")
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	for _, required := range []string{
		"CREATE TABLE task_node_generation_supersessions",
		"PRIMARY KEY (ledger_id,node_id)",
		"superseded_at_generation=retiring_generation+1",
		"FOREIGN KEY (job_id,superseded_at_generation)",
		"REFERENCES job_generations(job_id,generation) ON DELETE RESTRICT",
		"generation supersession requires an exact terminal non-goal task node",
		"events.job_generation=NEW.retiring_generation",
		"event_kind='node_generation_superseded'",
		"node supersession event and normalized projection disagree",
		"task node generation supersessions are immutable",
		"BEFORE UPDATE OR DELETE ON task_node_generation_supersessions",
		"BEFORE TRUNCATE ON task_node_generation_supersessions",
		string(taskstate.CommandSupersedeNodeGeneration),
		string(taskstate.EventNodeGenerationSuperseded),
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("task-generation supersession migration omitted %q", required)
		}
	}
	for _, forbidden := range []string{
		"events.job_generation=NEW.superseded_at_generation",
		"ON CONFLICT",
	} {
		if strings.Contains(source, forbidden) {
			t.Fatalf("task-generation supersession migration contains forbidden path %q", forbidden)
		}
	}
}
