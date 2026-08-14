package queue

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/station"
)

func TestPostgresConversationContextSelectionUsesExactGapAuthority(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(t.Context(), loadMigrationBundleThroughPrefix(t, "096")); err != nil {
		t.Fatal(err)
	}
	jobRecord, err := repository.EnqueueJob(t.Context(), "context station", model.PipelineCoding, nil)
	if err != nil {
		t.Fatal(err)
	}
	claim, err := repository.ClaimNextStep(t.Context(), "context-station-worker")
	if err != nil {
		t.Fatal(err)
	}
	if claim == nil || claim.Job.ID != jobRecord.ID {
		t.Fatalf("claim=%+v want job %d", claim, jobRecord.ID)
	}
	job, err := assemblyline.NewConversationContextSelectionJob(
		assemblyline.ConversationContextSelectionInput{
			ExactInstruction: "Use the second one.",
			MaxSelectedBytes: assemblyline.MaxSelectedConversationProjectionBytes,
			CandidateAuthorities: []assemblyline.ConversationContextTurn{
				{MessageID: 11, Role: assemblyline.ConversationContextUser, Content: "Compare the two implementations."},
				{
					MessageID: 12, Role: assemblyline.ConversationContextAssistant, PairedUserMessageID: 11,
					Content: "The second has lower latency.",
				},
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.OpenStationGap(t.Context(), StationGapOpenRecord{
		Authority: claim.Authority, Job: job, Station: station.ConversationObjectiveKind,
		ContextTokens: 8192, MaxOutputTokens: 512,
		OutputLimitMode: llm.ExactPreparedOutputLimitExplicit,
	}); err == nil {
		t.Fatal("context-selection work opened under objective-kind station")
	}
	opening, err := repository.OpenStationGap(t.Context(), StationGapOpenRecord{
		Authority: claim.Authority, Job: job, Station: station.ConversationContextSelection,
		ContextTokens: 8192, MaxOutputTokens: 512,
		OutputLimitMode: llm.ExactPreparedOutputLimitExplicit,
	})
	if err != nil {
		t.Fatal(err)
	}
	if opening.Station != station.ConversationContextSelection || opening.WorkKind != string(job.Kind) {
		t.Fatalf("opening=%+v", opening)
	}
}

func TestPostgresConversationContextSelectionMigrationRejectsChangedPriorFunction(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(t.Context(), loadMigrationBundleThroughPrefix(t, "072")); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(t.Context(), `
		CREATE OR REPLACE FUNCTION station_owns_portable_work(station TEXT, work_kind TEXT, payload JSONB)
		RETURNS BOOLEAN AS 'SELECT FALSE' LANGUAGE SQL IMMUTABLE STRICT
	`); err != nil {
		t.Fatal(err)
	}
	err := repository.EnsureSchema(t.Context(), loadMigrationBundleThroughPrefix(t, "073"))
	if err == nil || !strings.Contains(err.Error(), "prior station function hash") {
		t.Fatalf("migration error=%v", err)
	}
	var installed bool
	if err := pool.QueryRow(t.Context(), `
		SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE filename='073_conversation_context_selection_station.sql')
	`).Scan(&installed); err != nil {
		t.Fatal(err)
	}
	if installed {
		t.Fatal("rejected migration wrote its ledger entry")
	}
}
