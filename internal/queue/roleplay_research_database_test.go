package queue

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/evidence"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/roleplay"
)

func TestRoleplayResearchCanonicalQueueCompletionReplayAndNoCanon(t *testing.T) {
	ctx := t.Context()
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(ctx, loadCheckedMigrationBundle(t)); err != nil {
		t.Fatal(err)
	}
	channel, err := repository.CreateRoleplayChannel(ctx, model.Channel{
		ID: "research-story", Scope: model.ChannelScopeUser, Name: "Research story",
		WorkspaceRoot: "/srv/workspaces/research-story", Mode: model.ChannelModeRoleplay,
	}, "Observatory", "Ada")
	if err != nil {
		t.Fatal(err)
	}
	store, err := roleplay.NewStore(repository.pool)
	if err != nil {
		t.Fatal(err)
	}
	world, found, err := store.FindWorldByChannel(ctx, string(channel.ID))
	if err != nil || !found {
		t.Fatalf("resolve roleplay world: found=%t err=%v", found, err)
	}
	configureRoleplayQueueTestScene(
		t, store, world.ID, string(channel.RoleplayViewpointCharacterID),
	)

	const exact = `/research "How do ocean currents redistribute heat?"`
	if _, _, err := enqueueNarratorRoleplayTurn(ctx, repository, channel.ID, exact); !errors.Is(err, roleplay.ErrResearchCapabilityDenied) {
		t.Fatalf("missing capability enqueue error=%v", err)
	}
	assertRoleplayResearchQueueCounts(t, repository, channel.ID, 0, 0, 0)
	if _, _, err := enqueueNarratorRoleplayTurn(
		ctx, repository, channel.ID, `/research ocean currents`,
	); err == nil || !strings.Contains(err.Error(), "canonical quoted question") {
		t.Fatalf("malformed reserved syntax error=%v", err)
	}
	assertRoleplayResearchQueueCounts(t, repository, channel.ID, 0, 0, 0)

	capability, err := repository.ConfigureRoleplayCharacterCapability(
		ctx, world.ID, string(channel.RoleplayViewpointCharacterID),
		roleplay.CapabilityWebResearch, true,
	)
	if err != nil || !capability.WebResearch {
		t.Fatalf("enable research capability=%+v err=%v", capability, err)
	}
	message, job, err := enqueueNarratorRoleplayTurn(ctx, repository, channel.ID, exact)
	if err != nil {
		t.Fatal(err)
	}
	if message.Content != exact || job.Instruction != exact {
		t.Fatalf("research command was rewritten: message=%q job=%q", message.Content, job.Instruction)
	}
	var binding channelTurnMetadata
	if err := json.Unmarshal(job.Metadata, &binding); err != nil {
		t.Fatal(err)
	}
	if binding.ChannelMode != model.ChannelModeRoleplay ||
		binding.RoleplayInputKind != roleplay.SimulationTurnExternalCommand ||
		binding.RoleplayViewpointCharacterID != channel.RoleplayViewpointCharacterID {
		t.Fatalf("research job binding=%+v", binding)
	}
	research, err := repository.LoadRoleplayResearchTurn(ctx, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if research.Authority != roleplay.AuthorityRealWorld ||
		research.CapabilityGrantID != capability.WebResearchGrant ||
		research.CharacterID != string(channel.RoleplayViewpointCharacterID) {
		t.Fatalf("research authority=%+v capability=%+v", research, capability)
	}
	claim, err := repository.ClaimNextStep(ctx, "roleplay-research-proof-worker")
	if err != nil {
		t.Fatal(err)
	}
	if claim == nil || claim.Job.ID != job.ID {
		t.Fatalf("claim=%+v job=%d", claim, job.ID)
	}
	operationID := testLifecycleOperationID(t, "roleplay-research-complete", claim.Step.ID)
	output := "Ocean currents move heat between regions, shaping climate patterns. [1]\n\n" +
		"Sources:\n[1] Ocean circulation — https://science.example/ocean-circulation"
	record := roleplayResearchQueueEvidenceFixture(
		t, job, claim.Step.ID, research, "https://science.example/ocean-circulation",
	)
	command := CompleteStepCommand{
		OperationID: operationID, Authority: claim.Authority, StepID: claim.Step.ID,
		Output: output, ContextKey: "objective_result", ContextValue: "roleplay-research-proof",
	}
	evidenceCommand := CompleteStepEvidenceCommand{
		CompleteStepCommand: command, Evidence: []evidence.Record{record},
	}
	if err := repository.CompleteStepWithEvidence(ctx, evidenceCommand); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.ConfigureRoleplayCharacterCapability(
		ctx, world.ID, string(channel.RoleplayViewpointCharacterID),
		roleplay.CapabilityWebResearch, false,
	); err != nil {
		t.Fatal(err)
	}
	if err := repository.CompleteStepWithEvidence(ctx, evidenceCommand); err != nil {
		t.Fatalf("exact research completion replay after grant revocation: %v", err)
	}

	if _, err := repository.ConfigureRoleplayCharacterCapability(
		ctx, world.ID, string(channel.RoleplayViewpointCharacterID),
		roleplay.CapabilityWebResearch, true,
	); err != nil {
		t.Fatal(err)
	}
	secondMessage, secondJob, err := enqueueNarratorRoleplayTurn(ctx, repository, channel.ID, exact)
	if err != nil || secondMessage.Content != exact {
		t.Fatalf("second research enqueue message=%+v error=%v", secondMessage, err)
	}
	secondResearch, err := repository.LoadRoleplayResearchTurn(ctx, secondJob.ID)
	if err != nil {
		t.Fatal(err)
	}
	secondClaim, err := repository.ClaimNextStep(ctx, "roleplay-research-revocation-worker")
	if err != nil || secondClaim == nil || secondClaim.Job.ID != secondJob.ID {
		t.Fatalf("second research claim=%+v error=%v", secondClaim, err)
	}
	secondOperationID := testLifecycleOperationID(t, "roleplay-research-revoked", secondClaim.Step.ID)
	secondCommand := CompleteStepEvidenceCommand{
		CompleteStepCommand: CompleteStepCommand{
			OperationID: secondOperationID, Authority: secondClaim.Authority,
			StepID: secondClaim.Step.ID, Output: output,
			ContextKey: "objective_result", ContextValue: "roleplay-research-revoked-proof",
		},
		Evidence: []evidence.Record{roleplayResearchQueueEvidenceFixture(
			t, secondJob, secondClaim.Step.ID, secondResearch,
			"https://science.example/ocean-circulation",
		)},
	}
	if _, err := repository.ConfigureRoleplayCharacterCapability(
		ctx, world.ID, string(channel.RoleplayViewpointCharacterID),
		roleplay.CapabilityWebResearch, false,
	); err != nil {
		t.Fatal(err)
	}
	if err := repository.CompleteStepWithEvidence(ctx, secondCommand); !errors.Is(err, roleplay.ErrResearchCapabilityDenied) {
		t.Fatalf("completion after grant revocation error=%v", err)
	}

	var researchCompletions, citations, fictionalCompletions, canonEvents, knowledge int
	var assistantContent, citationAuthority string
	if err := repository.pool.QueryRow(ctx, `
		SELECT
		  (SELECT COUNT(*) FROM roleplay_research_completions WHERE operation_id=$1),
		  (SELECT COUNT(*) FROM roleplay_research_completion_citations WHERE operation_id=$1),
		  (SELECT COUNT(*) FROM roleplay_turn_completions WHERE operation_id=$1),
		  (SELECT COUNT(*) FROM roleplay_canon_events),
		  (SELECT COUNT(*) FROM roleplay_character_knowledge),
		  (SELECT content FROM ai_channel_messages
		   WHERE channel_id=$2 AND role='assistant' ORDER BY id DESC LIMIT 1),
		  (SELECT authority_namespace FROM roleplay_research_completion_citations
		   WHERE operation_id=$1 AND completion_index=0)
	`, operationID, channel.ID).Scan(
		&researchCompletions, &citations, &fictionalCompletions, &canonEvents, &knowledge,
		&assistantContent, &citationAuthority,
	); err != nil {
		t.Fatal(err)
	}
	if researchCompletions != 1 || citations != 1 || fictionalCompletions != 0 ||
		canonEvents != 0 || knowledge != 0 || assistantContent != output ||
		citationAuthority != string(roleplay.AuthorityRealWorld) {
		t.Fatalf("research completion counts research=%d citations=%d fictional=%d canon=%d knowledge=%d authority=%q output=%q",
			researchCompletions, citations, fictionalCompletions, canonEvents, knowledge,
			citationAuthority, assistantContent)
	}
	page, err := repository.ListChannelMessages(ctx, channel.ID, 10, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Messages) != 3 || page.Messages[0].Role != model.ChannelMessageRoleUser ||
		page.Messages[0].SpeakerName != "Narrator" ||
		page.Messages[1].Role != model.ChannelMessageRoleAssistant ||
		page.Messages[1].SpeakerName != "Ada" ||
		page.Messages[2].Role != model.ChannelMessageRoleUser || page.Messages[2].SpeakerName != "Narrator" {
		t.Fatalf("canonical research transcript=%+v", page.Messages)
	}
	var deniedAssistant, deniedCanon, deniedAdvance, deniedLifecycle, deniedEvidence int
	if err := repository.pool.QueryRow(ctx, `
		SELECT
		  (SELECT COUNT(*) FROM ai_channel_messages
		   WHERE channel_id=$1 AND role='assistant'),
		  (SELECT COUNT(*) FROM roleplay_canon_events),
		  (SELECT COUNT(*) FROM roleplay_simulation_turn_advances),
		  (SELECT COUNT(*) FROM job_lifecycle_operations WHERE operation_id=$2),
		  (SELECT COUNT(*) FROM step_completion_evidence_sets WHERE operation_id=$2)
	`, channel.ID, secondOperationID).Scan(
		&deniedAssistant, &deniedCanon, &deniedAdvance, &deniedLifecycle, &deniedEvidence,
	); err != nil {
		t.Fatal(err)
	}
	if deniedAssistant != 1 || deniedCanon != 0 || deniedAdvance != 1 ||
		deniedLifecycle != 0 || deniedEvidence != 0 {
		t.Fatalf("revoked completion mutated assistant/canon/advance/lifecycle/evidence=%d/%d/%d/%d/%d",
			deniedAssistant, deniedCanon, deniedAdvance, deniedLifecycle, deniedEvidence)
	}
}

func roleplayResearchQueueEvidenceFixture(
	t *testing.T,
	job model.Job,
	stepID int64,
	research roleplay.ResearchTurnAuthority,
	sourceURL string,
) evidence.Record {
	t.Helper()
	excerpt := "Ocean circulation transports heat from warmer regions toward cooler regions."
	projectionDigest := sha256.Sum256([]byte(excerpt))
	sourceContent := "Observed ocean circulation transfers thermal energy across latitudes."
	sourceDigest := sha256.Sum256([]byte(sourceContent))
	instructionDigest := sha256.Sum256([]byte(job.Instruction))
	requirementID := "requirement-roleplay-research-proof"
	return evidence.Record{
		JobID: job.ID, StepID: stepID, Kind: evidence.KindObjectiveCitation,
		SourceType: "web_document", SourceRef: sourceURL, Excerpt: excerpt,
		Summary: "The roleplay research response cited one acquired real-world document.",
		Hash:    hex.EncodeToString(sourceDigest[:]), Confidence: 1,
		RequirementAuthorityBindings: []string{requirementID + "#paragraph-1"},
		Metadata: map[string]any{
			"capsule_id":         "evidence_" + strings.Repeat("a", 64),
			"instruction_sha256": hex.EncodeToString(instructionDigest[:]),
			"objective_id":       "objective-roleplay-research-proof",
			"objective_kind":     "external_answer", "requirement_id": requirementID,
			"projection_sha256":                     hex.EncodeToString(projectionDigest[:]),
			"source_sha256":                         hex.EncodeToString(sourceDigest[:]),
			"paragraph_indexes":                     []int{0},
			"source_observed_at":                    time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC).Format(time.RFC3339Nano),
			"source_truncated":                      false,
			"authority_namespace":                   string(roleplay.AuthorityRealWorld),
			"roleplay_research_preparation_id":      research.PreparationID,
			"roleplay_research_world_id":            research.WorldID,
			"roleplay_research_character_id":        research.CharacterID,
			"roleplay_research_question_sha256":     research.QuestionSHA256,
			"roleplay_research_capability_grant_id": research.CapabilityGrantID,
		},
	}
}

func assertRoleplayResearchQueueCounts(
	t *testing.T,
	repository *Repository,
	channelID model.ChannelID,
	wantMessages, wantJobs, wantPreparations int,
) {
	t.Helper()
	var messages, jobs, preparations int
	if err := repository.pool.QueryRow(t.Context(), `
		SELECT
		  (SELECT COUNT(*) FROM ai_channel_messages WHERE channel_id=$1),
		  (SELECT COUNT(*) FROM jobs WHERE metadata->>'channel_id'=$1),
		  (SELECT COUNT(*) FROM roleplay_research_turns WHERE channel_id=$1)
	`, channelID).Scan(&messages, &jobs, &preparations); err != nil {
		t.Fatal(err)
	}
	if messages != wantMessages || jobs != wantJobs || preparations != wantPreparations {
		t.Fatalf("rolled-back queue state messages=%d jobs=%d research_preparations=%d",
			messages, jobs, preparations)
	}
}
