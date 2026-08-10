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

func TestReplanLocksJobThenStepsThenLedgerAndRetiresNodesBeforeAuthority(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("repository_replan.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	jobLock := strings.Index(source, "lockedJobTx(ctx, tx, command.JobID)")
	stepLock := strings.Index(source, "lockCurrentReplanTailTx(ctx, tx, command.JobID")
	ledgerLock := strings.Index(source, "loadTaskLedgerHeaderTx(ctx, tx, command.JobID, true)")
	if jobLock < 0 || stepLock < 0 || ledgerLock < 0 || !(jobLock < stepLock && stepLock < ledgerLock) {
		t.Fatalf("replan lock order is not job then step then ledger: %d/%d/%d", jobLock, stepLock, ledgerLock)
	}
	createGeneration := strings.Index(source, "createReplanGenerationTx(")
	retireNodes := strings.Index(source, "supersedeCurrentCognitionObligationsTx(")
	retireSteps := strings.Index(source, "retireReplanTailTx(")
	advanceJob := strings.Index(source, "advanceReplannedJobTx(")
	if createGeneration < 0 || retireNodes < 0 || retireSteps < 0 || advanceJob < 0 ||
		!(createGeneration < retireNodes && retireNodes < retireSteps && retireSteps < advanceJob) {
		t.Fatalf("replan generation-retirement order is invalid: %d/%d/%d/%d",
			createGeneration, retireNodes, retireSteps, advanceJob)
	}

	helperRaw, err := os.ReadFile("task_generation_retirement.go")
	if err != nil {
		t.Fatal(err)
	}
	helper := string(helperRaw)
	for _, required := range []string{
		"FROM cognition_obligations AS obligations",
		"obligations.job_generation=$3",
		"ORDER BY obligations.node_id ASC",
		"FOR UPDATE OF nodes",
		"taskstate.SupersedeNodeGenerationCommand{",
		"applyQueueOwnedTaskCommandTx(",
	} {
		if !strings.Contains(helper, required) {
			t.Fatalf("queue-owned generation retirement omitted %q", required)
		}
	}
}
