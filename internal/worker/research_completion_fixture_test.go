package worker

import (
	"context"
	"maps"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/evidence"
	"github.com/gryph/omnidex/internal/queue"
)

func assertResearchWorkflowCompletion(t *testing.T, run researchWorkflowFixture, result objectiveTurnResult) {
	t.Helper()
	ctx := context.Background()
	output, records, err := prepareObjectiveTurnCompletion(result)
	if err != nil || len(records) != 2 {
		t.Fatalf("prepare research completion: records=%+v error=%v", records, err)
	}
	for index := range records {
		records[index].JobID = run.claim.Job.ID
		records[index].StepID = run.claim.Step.ID
	}
	operation, err := queue.NewLifecycleOperationID()
	if err != nil {
		t.Fatal(err)
	}
	command := queue.CompleteStepEvidenceCommand{CompleteStepCommand: queue.CompleteStepCommand{
		OperationID: operation, Authority: run.claim.Authority, StepID: run.claim.Step.ID,
		Output: output, ContextKey: "objective_result",
	}, Evidence: records}
	for _, mutation := range []struct {
		name  string
		apply func(*evidence.Record)
	}{
		{"text", func(record *evidence.Record) { record.Excerpt = "Invented source text." }},
		{"URL", func(record *evidence.Record) { record.SourceRef = "https://other.example/document" }},
		{"observation", func(record *evidence.Record) { record.Metadata["source_observed_at"] = "2026-01-01T00:00:00Z" }},
		{"truncation", func(record *evidence.Record) { record.Metadata["source_truncated"] = true }},
	} {
		invalid := command
		invalid.Evidence = append([]evidence.Record(nil), records...)
		invalid.Evidence[0].Metadata = maps.Clone(records[0].Metadata)
		mutation.apply(&invalid.Evidence[0])
		if err := run.repository.CompleteStepWithEvidence(ctx, invalid); err == nil || !strings.Contains(err.Error(), "web citation differs from its recorded source") {
			t.Fatalf("changed %s did not fail at the actual recorded-source check: %v", mutation.name, err)
		}
	}
	var publications int
	if err := run.pool.QueryRow(ctx, "SELECT count(*) FROM roleplay_research_completions WHERE job_id=$1", run.claim.Job.ID).Scan(&publications); err != nil || publications != 0 {
		t.Fatalf("invalid citations published partial completion: %d / %v", publications, err)
	}
	for range 2 {
		if err := run.repository.CompleteStepWithEvidence(ctx, command); err != nil {
			t.Fatalf("publish or replay exact research result: %v", err)
		}
	}
	changed := command
	changed.Output += " Unobserved additional output."
	if err := run.repository.CompleteStepWithEvidence(ctx, changed); err == nil {
		t.Fatal("publication replay accepted different answer text")
	}
	var text string
	var citations, canon, advances, acquisitions int
	if err := run.pool.QueryRow(ctx, `SELECT message.content,
		(SELECT count(*) FROM roleplay_research_completions WHERE job_id=$1),
		(SELECT count(*) FROM roleplay_research_completion_citations WHERE operation_id=$2),
		(SELECT count(*) FROM roleplay_canon_events WHERE world_id=$3),
		(SELECT count(*) FROM roleplay_simulation_turn_advances WHERE job_id=$1),
		(SELECT count(*) FROM web_evidence WHERE job_id=$1)
		FROM roleplay_research_completions AS completion
		JOIN ai_channel_messages AS message ON message.id=completion.source_message_id
		WHERE completion.job_id=$1`, run.claim.Job.ID, operation, run.authority.RoleplayWorldID).Scan(
		&text, &publications, &citations, &canon, &advances, &acquisitions,
	); err != nil {
		t.Fatal(err)
	}
	if text != output || publications != 1 || citations != 2 || canon != 0 || advances != 1 || acquisitions != 2 ||
		run.http.requests.Load() != 6 || run.provider.calls != 19 {
		t.Fatalf("publication changed observed work: text=%q publications=%d citations=%d canon=%d advances=%d acquisitions=%d HTTP=%d models=%d",
			text, publications, citations, canon, advances, acquisitions, run.http.requests.Load(), run.provider.calls)
	}
}
