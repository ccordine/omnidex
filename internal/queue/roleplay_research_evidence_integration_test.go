package queue

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/evidence"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/roleplay"
	"github.com/gryph/omnidex/internal/webresearch"
)

func TestRoleplayResearchUsesExactQuestionMessageAndRecordedWebEvidence(t *testing.T) {
	databaseURL := evidenceDatabaseURL(t)
	for _, question := range []string{"What does the current timetable say?", "Which edition does the archive list?"} {
		t.Run(question, func(t *testing.T) {
			ctx := context.Background()
			pool, repository := freshEvidenceRepository(t, databaseURL)
			job, claim, research, store := roleplayResearchEvidenceJob(t, repository, question)
			if research.Question != question || job.Instruction != "/research "+strconv.Quote(question) {
				t.Fatalf("the exact question changed: %#v / %#v", research, job)
			}
			prepared, err := store.LoadSimulationTurnForJob(ctx, research.PreparationID, job.ID)
			if err != nil {
				t.Fatal(err)
			}
			tx, err := pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			command, matched, err := roleplay.ParseResearchCommand(job.Instruction)
			if err != nil || !matched {
				t.Fatalf("parse research command: %v", err)
			}
			if _, err := roleplay.AuthorizeResearchPreparationTx(ctx, tx, prepared, command); err != nil {
				t.Fatal(err)
			}
			changed, _, err := roleplay.ParseResearchCommand(`/research "A different question."`)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := roleplay.AuthorizeResearchPreparationTx(ctx, tx, prepared, changed); !errors.Is(err, roleplay.ErrResearchAuthorityConflict) {
				t.Fatalf("question replay did not compare exact text: %v", err)
			}
			if err := tx.Rollback(ctx); err != nil {
				t.Fatal(err)
			}
			httpFixture := newWebEvidenceHTTPFixture(t, question, "The requested information is recorded in this source.")
			stored, err := repository.RecordWebEvidence(ctx, job.ID, httpFixture.acquire(t))
			if err != nil {
				t.Fatal(err)
			}
			citation := webEvidenceCitation(t, stored, claim.Step.ID)
			for key, value := range map[string]string{
				"authority_namespace":                   string(roleplay.AuthorityRealWorld),
				"roleplay_research_preparation_id":      research.PreparationID,
				"roleplay_research_world_id":            research.WorldID,
				"roleplay_research_character_id":        research.CharacterID,
				"roleplay_research_capability_grant_id": research.CapabilityGrantID,
			} {
				citation.Metadata[key] = value
			}
			sources, _, err := stored.Acquired.Project()
			if err != nil {
				t.Fatal(err)
			}
			artifact, err := webresearch.BuildGroundedCompletionArtifact([]webresearch.GroundedParagraph{{
				Text: "The source records the requested information.", EvidenceIDs: []webresearch.EvidenceID{sources[0].ID},
			}}, sources, 1, 1024)
			if err != nil {
				t.Fatal(err)
			}
			operationID, err := NewLifecycleOperationID()
			if err != nil {
				t.Fatal(err)
			}
			completion := CompleteStepEvidenceCommand{CompleteStepCommand: CompleteStepCommand{
				OperationID: operationID, Authority: claim.Authority, StepID: claim.Step.ID,
				Output: artifact.Rendered, ContextKey: "objective_result",
			}, Evidence: []evidence.Record{citation}}
			if err := repository.CompleteStepWithEvidence(ctx, completion); err != nil {
				t.Fatal(err)
			}
			if err := repository.CompleteStepWithEvidence(ctx, completion); err != nil {
				t.Fatalf("research completion replay: %v", err)
			}
			altered := completion
			altered.Output += " Different message."
			if err := repository.CompleteStepWithEvidence(ctx, altered); err == nil {
				t.Fatal("altered completion replay was accepted")
			}
			var actualText string
			var completions, citations, canon, advances int
			if err := pool.QueryRow(ctx, `SELECT message.content,
				(SELECT count(*) FROM roleplay_research_completions WHERE job_id=$1),
				(SELECT count(*) FROM roleplay_research_completion_citations WHERE operation_id=$2),
				(SELECT count(*) FROM roleplay_canon_events WHERE world_id=$3),
				(SELECT count(*) FROM roleplay_simulation_turn_advances WHERE job_id=$1)
				FROM roleplay_research_completions AS completion
				JOIN ai_channel_messages AS message ON message.id=completion.source_message_id
				WHERE completion.job_id=$1`, job.ID, operationID, research.WorldID).Scan(
				&actualText, &completions, &citations, &canon, &advances,
			); err != nil {
				t.Fatal(err)
			}
			if actualText != artifact.Rendered || completions != 1 || citations != 1 || canon != 0 || advances != 1 || httpFixture.requests.Load() != 2 {
				t.Fatalf("research publication or replay changed authoritative reality: text=%q completion=%d citations=%d canon=%d advances=%d requests=%d", actualText, completions, citations, canon, advances, httpFixture.requests.Load())
			}
			var oldColumns int
			if err := pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.columns
				WHERE table_schema=current_schema() AND table_name IN
				('roleplay_research_turns','roleplay_research_completions','roleplay_research_completion_citations')
				AND column_name IN ('question_sha256','rendered_sha256','source_sha256','narrative_fingerprint')`).Scan(&oldColumns); err != nil || oldColumns != 0 {
				t.Fatalf("retired research hash columns remain: count=%d error=%v", oldColumns, err)
			}
			encoded, err := json.Marshal(research)
			if err != nil || strings.Contains(string(encoded), "question_sha256") || strings.Contains(string(encoded), "narrative_fingerprint") {
				t.Fatalf("research still requires a question digest: %s / %v", encoded, err)
			}
		})
	}
}

func roleplayResearchEvidenceJob(t *testing.T, repository *Repository, question string) (model.Job, *model.ClaimedStep, roleplay.ResearchTurnAuthority, *roleplay.Store) {
	t.Helper()
	ctx := context.Background()
	channel, world, store := roleplayEvidenceScene(t, repository)
	characterID := string(channel.RoleplayViewpointCharacterID)
	if _, err := repository.ConfigureRoleplayCharacterCapability(ctx, world.ID, characterID, roleplay.CapabilityWebResearch, true); err != nil {
		t.Fatal(err)
	}
	_, job, err := repository.EnqueueRoleplayChannelTurn(ctx, channel.ID, "/research "+strconv.Quote(question), roleplay.UserTurnRequest{
		PersonaKind: roleplay.UserPersonaNarrator, ContributionKind: roleplay.UserContributionCommand,
	})
	if err != nil {
		t.Fatal(err)
	}
	claim, err := repository.ClaimNextStep(ctx, "roleplay-research-evidence-test")
	if err != nil || claim == nil || claim.Job.ID != job.ID {
		t.Fatalf("claim = %#v / %v", claim, err)
	}
	research, err := repository.LoadRoleplayResearchTurn(ctx, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	return job, claim, research, store
}
