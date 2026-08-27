package queue

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

const exactLifecycleFeedbackMigration = "088_exact_lifecycle_feedback_authority.sql"

func TestPostgresScrumChannelReplanPreservesExactMessageBytes(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(t.Context(), loadCheckedMigrationBundle(t)); err != nil {
		t.Fatal(err)
	}
	project, card := newScrumChannelOperationCard(t, repository, "exact-replan")
	job := enqueueScrumChannelJobForTest(t, repository, card, "Initial exact Scrum channel job.")
	card = setScrumCardRunningForTest(t, pool, project.ID, card.ID, job.ID)
	exact := "  Replace the pending work without normalizing me.  \n"
	request := ScrumChannelOperationRequest{
		OperationID: testLifecycleOperationID(t, "scrum-channel-exact-replan", job.ID),
		ProjectID:   project.ID, CardID: card.ID, Message: exact,
	}
	command := ScrumChannelOperationCommand{
		Request: request, ExpectedCardUpdatedAt: card.UpdatedAt,
		Effect:       ScrumChannelEffect{Kind: ScrumChannelReplanJob, JobID: job.ID},
		ResultAction: "replanned",
	}
	builder := func(current DBScrumCard, resultJob model.Job) (ScrumChannelCardUpdate, error) {
		return scrumChannelTestUpdate(t, current, request, resultJob), nil
	}
	first, err := repository.ExecuteScrumChannelOperation(t.Context(), command, builder)
	if err != nil {
		t.Fatal(err)
	}
	second, err := repository.ExecuteScrumChannelOperation(t.Context(), command, builder)
	if err != nil {
		t.Fatal(err)
	}
	if !first.Applied || second.Applied || first.Job.CurrentGeneration != 2 {
		t.Fatalf("exact replan first=%+v second=%+v", first, second)
	}
	var generationFeedback, entryContent, operationMessage string
	if err := pool.QueryRow(t.Context(), `
		SELECT generation.feedback, entry.content,
		       registry.command_payload->>'message'
		FROM job_generations generation
		JOIN task_entries entry ON entry.job_id=generation.job_id
		  AND entry.kind='feedback' AND entry.feedback_purpose='replan'
		JOIN scrum_channel_operations operation ON operation.job_id=generation.job_id
		JOIN lifecycle_operation_registry registry
		  ON registry.operation_id=operation.operation_id
		WHERE generation.job_id=$1 AND generation.generation=2
	`, job.ID).Scan(&generationFeedback, &entryContent, &operationMessage); err != nil {
		t.Fatal(err)
	}
	if generationFeedback != exact || entryContent != exact || operationMessage != exact {
		t.Fatalf("persisted exact bytes generation=%q entry=%q operation=%q", generationFeedback, entryContent, operationMessage)
	}
	assertAppliedMigrationCount(t, pool, exactLifecycleFeedbackMigration, 1)
}

func TestPostgresLifecycleFeedbackAuthorityRejectsInvalidRawValues(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	if err := New(pool).EnsureSchema(t.Context(), loadMigrationBundleThroughPrefix(t, "088")); err != nil {
		t.Fatal(err)
	}
	for name, value := range map[string]string{
		"empty":     "",
		"blank":     " \t\r\n",
		"oversized": strings.Repeat("x", maxReplanFeedbackBytes+1),
	} {
		t.Run(name, func(t *testing.T) {
			var accepted bool
			if err := pool.QueryRow(t.Context(), `SELECT lifecycle_feedback_is_valid($1,65536)`, value).Scan(&accepted); err != nil {
				t.Fatal(err)
			}
			if accepted {
				t.Fatalf("invalid raw lifecycle feedback %q accepted", name)
			}
		})
	}
	var literalEscapeAccepted bool
	if err := pool.QueryRow(
		t.Context(), `SELECT lifecycle_feedback_is_valid($1,65536)`, `literal \000 text`,
	).Scan(&literalEscapeAccepted); err != nil {
		t.Fatal(err)
	}
	if !literalEscapeAccepted {
		t.Fatal("literal backslash-zero text was mistaken for a NUL byte")
	}
	for name, expression := range map[string]string{
		"invalid UTF-8": `convert_from(decode('80','hex'),'UTF8')`,
		"NUL":           `convert_from(decode('00','hex'),'UTF8')`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := pool.Exec(t.Context(), `SELECT lifecycle_feedback_is_valid(`+expression+`,65536)`); err == nil {
				t.Fatalf("raw %s lifecycle feedback accepted", name)
			}
		})
	}
}

func TestPostgresLifecycleFeedbackMigrationRefusesOversizedHistoryAtomically(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(t.Context(), loadMigrationBundleThroughPrefix(t, "087")); err != nil {
		t.Fatal(err)
	}
	fixture := seedPreInlineExecutionMigrationJob(
		t, t.Context(), pool, "historical feedback audit",
		model.PipelineCoding, "v3_coding", json.RawMessage(`{}`),
	)
	content := strings.Repeat("x", maxReplanFeedbackBytes+1)
	if _, err := pool.Exec(t.Context(), `
		INSERT INTO task_entries (
			ledger_id,job_id,id,kind,feedback_purpose,status,authority,
			content,content_sha256,created_by,metadata,created_version,updated_version
		)
		SELECT id,$1,'historical-oversized-feedback','feedback','replan','active','user',
		       $2,encode(digest($2,'sha256'),'hex'),'user','{}'::jsonb,version,version
		FROM task_ledgers WHERE job_id=$1
	`, fixture.Job.ID, content); err != nil {
		t.Fatal(err)
	}
	err := repository.EnsureSchema(t.Context(), loadMigrationBundleThroughPrefix(t, "088"))
	if err == nil || !strings.Contains(err.Error(), "historical lifecycle feedback") {
		t.Fatalf("oversized history migration error=%v", err)
	}
	assertAppliedMigrationCount(t, pool, exactLifecycleFeedbackMigration, 0)
	var oldConstraint int
	if err := pool.QueryRow(t.Context(), `
		SELECT COUNT(*) FROM pg_constraint
		WHERE conrelid='task_entries'::regclass
		  AND conname='task_entries_content_check'
		  AND pg_get_constraintdef(oid,true) LIKE '%task_ledger_text_is_exact(content)%'
	`).Scan(&oldConstraint); err != nil {
		t.Fatal(err)
	}
	if oldConstraint != 1 {
		t.Fatalf("rejected migration changed predecessor constraint count=%d", oldConstraint)
	}
}
