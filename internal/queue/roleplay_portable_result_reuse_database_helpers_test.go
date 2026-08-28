package queue

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/roleplay"
	"github.com/gryph/omnidex/internal/station"
	"github.com/jackc/pgx/v5/pgxpool"
)

type roleplayPortableReuseDatabaseFixture struct {
	Repository   *Repository
	Pool         *pgxpool.Pool
	Channel      model.Channel
	Store        *roleplay.Store
	WorldID      string
	CharacterIDs []string
}

func newRoleplayPortableReuseDatabaseFixture(
	t *testing.T,
	marker string,
) roleplayPortableReuseDatabaseFixture {
	t.Helper()
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(t.Context(), loadMigrationBundleThroughPrefix(t, "153")); err != nil {
		t.Fatal(err)
	}
	installRoleplayPortableReuseMigrationsForTest(t, pool)
	channel, err := repository.CreateRoleplayChannel(t.Context(), model.Channel{
		ID: model.ChannelID(marker), Scope: model.ChannelScopeUser, Name: "Portable reuse story",
		WorkspaceRoot: "/srv/workspaces/" + marker, Mode: model.ChannelModeRoleplay,
	}, "Portable reuse world", "Mara")
	if err != nil {
		t.Fatal(err)
	}
	store, err := roleplay.NewStore(pool)
	if err != nil {
		t.Fatal(err)
	}
	world, found, err := store.FindWorldByChannel(t.Context(), string(channel.ID))
	if err != nil || !found {
		t.Fatalf("resolve roleplay reuse world: found=%t err=%v", found, err)
	}
	other, err := store.CreateCharacter(t.Context(), world.ID, "Ivo")
	if err != nil {
		t.Fatal(err)
	}
	characters := []string{string(channel.RoleplayViewpointCharacterID), other.ID}
	configureRoleplayQueueTestScene(t, store, world.ID, characters...)
	return roleplayPortableReuseDatabaseFixture{
		Repository: repository, Pool: pool, Channel: channel, Store: store,
		WorldID: world.ID, CharacterIDs: characters,
	}
}

func installRoleplayPortableReuseMigrationsForTest(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	for _, migration := range []struct {
		name      string
		installed string
	}{
		{roleplayPortableResultReuseMigration,
			"to_regclass('roleplay_portable_result_reuses') IS NOT NULL"},
		{"157_roleplay_user_canon_modality_authority.sql",
			"to_regprocedure('roleplay_user_turn_requires_canon(text,text,jsonb)') IS NOT NULL"},
	} {
		var installed bool
		if err := pool.QueryRow(
			t.Context(), "SELECT "+migration.installed,
		).Scan(&installed); err != nil {
			t.Fatal(err)
		}
		if installed {
			continue
		}
		raw, err := os.ReadFile("../../migrations/" + migration.name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(t.Context(), string(raw)); err != nil {
			t.Fatalf("apply roleplay reuse migration %s: %v", migration.name, err)
		}
	}
}

func roleplayPortableReuseRootJob(t *testing.T, exact string) assemblyline.PortableJob {
	t.Helper()
	job, err := assemblyline.NewConversationResponseJob(assemblyline.ConversationResponseInput{
		Kind: assemblyline.ObjectiveKindAnswer, ExactInstruction: exact,
	})
	if err != nil {
		t.Fatal(err)
	}
	return job
}

func persistRoleplayPortableReuseLeaf(
	t *testing.T,
	repository *Repository,
	claim *model.ClaimedStep,
	portable assemblyline.PortableJob,
	candidate string,
) StationGapOutcome {
	t.Helper()
	const contextTokens = 32768
	owner, err := StationForPortableJob(portable)
	if err != nil {
		t.Fatal(err)
	}
	gap, err := repository.OpenStationGap(t.Context(), StationGapOpenRecord{
		Authority: claim.Authority, Job: portable, Station: owner,
		ContextTokens: contextTokens,
		MaxOutputTokens: portableStationTestMaxOutputTokens(
			t, portable, contextTokens,
		),
		OutputLimitMode: llm.ExactPreparedOutputLimitNatural,
	})
	if err != nil {
		t.Fatal(err)
	}
	prepared := stationCallTestPrepared(t, gap)
	discovery := persistStationDiscoverySuccess(t, repository, claim.Authority, gap, prepared)
	call, err := repository.OpenStationCall(t.Context(), StationCallOpenRecord{
		Authority: claim.Authority, Gap: gap, Discovery: discovery, Prepared: prepared,
	})
	if err != nil {
		t.Fatal(err)
	}
	generated := stationCallSuccessWithContent(t, prepared, call, candidate)
	receipt, err := repository.RecordStationCallReceipt(t.Context(), StationCallReceiptRecord{
		Authority: claim.Authority, OpeningID: call.ID, GapID: gap.GapID, Result: generated,
	})
	if err != nil {
		t.Fatal(err)
	}
	outcome, err := repository.CloseStationGap(t.Context(), StationGapTerminalRecord{
		Authority: claim.Authority, OpeningID: gap.ID, GapID: gap.GapID,
		Status: StationGapResolved, Response: candidate,
		Projection: stationGapExactResponseProjection(receipt.GenerationSHA256, candidate),
	})
	if err != nil {
		t.Fatal(err)
	}
	return outcome
}

func persistRejectedRoleplayPortableReuseLeaf(
	t *testing.T,
	repository *Repository,
	claim *model.ClaimedStep,
	portable assemblyline.PortableJob,
	candidate string,
) StationGapOutcome {
	t.Helper()
	const contextTokens = 32768
	owner, err := StationForPortableJob(portable)
	if err != nil {
		t.Fatal(err)
	}
	gap, err := repository.OpenStationGap(t.Context(), StationGapOpenRecord{
		Authority: claim.Authority, Job: portable, Station: owner,
		ContextTokens: contextTokens,
		MaxOutputTokens: portableStationTestMaxOutputTokens(
			t, portable, contextTokens,
		),
		OutputLimitMode: llm.ExactPreparedOutputLimitNatural,
	})
	if err != nil {
		t.Fatal(err)
	}
	prepared := stationCallTestPrepared(t, gap)
	discovery := persistStationDiscoverySuccess(t, repository, claim.Authority, gap, prepared)
	call, err := repository.OpenStationCall(t.Context(), StationCallOpenRecord{
		Authority: claim.Authority, Gap: gap, Discovery: discovery, Prepared: prepared,
	})
	if err != nil {
		t.Fatal(err)
	}
	generated := stationCallSuccessWithContent(t, prepared, call, candidate)
	if _, err := repository.RecordStationCallReceipt(t.Context(), StationCallReceiptRecord{
		Authority: claim.Authority, OpeningID: call.ID, GapID: gap.GapID, Result: generated,
	}); err != nil {
		t.Fatal(err)
	}
	outcome, err := repository.CloseStationGap(t.Context(), StationGapTerminalRecord{
		Authority: claim.Authority, OpeningID: gap.ID, GapID: gap.GapID,
		Status: StationGapFailed, Error: "injected deterministic candidate rejection",
	})
	if err != nil {
		t.Fatal(err)
	}
	return outcome
}

func failRoleplayPortableReuseJob(
	t *testing.T,
	repository *Repository,
	claim *model.ClaimedStep,
	marker string,
) {
	t.Helper()
	if err := repository.FailStep(t.Context(), FailStepCommand{
		OperationID: testLifecycleOperationID(t, marker, claim.Step.ID),
		Authority:   claim.Authority, StepID: claim.Step.ID, Error: "injected later roleplay failure",
	}); err != nil {
		t.Fatal(err)
	}
}

func completeRoleplayPortableReuseJob(
	t *testing.T,
	repository *Repository,
	claim *model.ClaimedStep,
	marker string,
) {
	t.Helper()
	var metadata channelTurnMetadata
	if err := json.Unmarshal(claim.Job.Metadata, &metadata); err != nil {
		t.Fatal(err)
	}
	responses := make([]RoleplayResponseCompletion, len(metadata.RoleplayResponders))
	output := ""
	for index, responder := range metadata.RoleplayResponders {
		text := responder.CharacterID + " answers in order."
		if index > 0 {
			output += "\n\n"
		}
		output += text
		responses[index] = RoleplayResponseCompletion{
			Position: index, CharacterID: model.RoleplayCharacterID(responder.CharacterID), Output: text,
		}
	}
	if metadata.RoleplayUserTurn == nil {
		t.Fatal("completed roleplay reuse fixture has no exact user-turn authority")
	}
	_, hasCanonContribution, err := metadata.RoleplayUserTurn.CanonContribution()
	if err != nil {
		t.Fatal(err)
	}
	var userCanon *RoleplayUserCanonCompletion
	if hasCanonContribution {
		userCanon = &RoleplayUserCanonCompletion{
			Facts: []string{}, KnowledgeCharacterIDs: []model.RoleplayCharacterID{},
		}
	}
	if err := repository.CompleteStepWithEvidence(t.Context(), CompleteStepEvidenceCommand{
		CompleteStepCommand: CompleteStepCommand{
			OperationID: testLifecycleOperationID(t, marker, claim.Step.ID),
			Authority:   claim.Authority, StepID: claim.Step.ID, Output: output,
			ContextKey: "objective_result", ContextValue: marker,
			RoleplayUserCanon: userCanon,
			RoleplayResponses: responses,
		},
	}); err != nil {
		t.Fatal(err)
	}
}

func roleplayPortableReuseStation(t *testing.T, job assemblyline.PortableJob) station.ID {
	t.Helper()
	owner, err := StationForPortableJob(job)
	if err != nil {
		t.Fatal(err)
	}
	return owner
}
