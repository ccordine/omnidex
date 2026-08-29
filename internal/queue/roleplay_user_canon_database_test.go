package queue

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/roleplay"
)

func TestRoleplayUserCanonPersistsExactCharacterAndNarratorProvenanceAndReplays(t *testing.T) {
	ctx := t.Context()
	pool := openIsolatedDatabasePool(t)
	repository := New(pool)
	if err := repository.ResetDatabase(ctx, loadCurrentDatabaseSetup(t)); err != nil {
		t.Fatal(err)
	}

	t.Run("narrator frozen participants", func(t *testing.T) {
		channel, world, other := setupUserCanonDatabaseChannel(
			t, repository, "user-canon-narrator", "Narrator provenance",
		)
		request := roleplay.UserTurnRequest{
			PersonaKind:      roleplay.UserPersonaNarrator,
			ContributionKind: roleplay.UserContributionNarration,
			Parts: []roleplay.UserTurnPart{{
				Kind: roleplay.UserTurnPartEvent, Text: "The bronze bell cracked.",
			}},
		}
		exact, err := roleplay.ComposeUserTurn(request)
		if err != nil {
			t.Fatal(err)
		}
		message, job, err := repository.EnqueueRoleplayChannelTurn(ctx, channel.ID, exact, request)
		if err != nil {
			t.Fatal(err)
		}
		fact := "The bronze bell cracked."
		recipients := []model.RoleplayCharacterID{
			channel.RoleplayViewpointCharacterID, model.RoleplayCharacterID(other.ID),
		}
		operationID := completeUserCanonDatabaseTurn(
			t, repository, job, fact, recipients,
		)
		assertUserCanonDatabaseReceipt(
			t, repository, operationID, message.ID, world.ID,
			roleplay.UserPersonaNarrator, "", fact, recipients, "",
		)
	})

	t.Run("character explicit actor", func(t *testing.T) {
		channel, world, other := setupUserCanonDatabaseChannel(
			t, repository, "user-canon-character", "Character provenance",
		)
		actorID := string(channel.RoleplayViewpointCharacterID)
		exact, request := characterRoleplayTurn(t, actorID, "I keep the brass key.")
		message, job, err := repository.EnqueueRoleplayChannelTurn(ctx, channel.ID, exact, request)
		if err != nil {
			t.Fatal(err)
		}
		fact := "Mara kept the brass key."
		recipients := []model.RoleplayCharacterID{channel.RoleplayViewpointCharacterID}
		operationID := completeUserCanonDatabaseTurn(
			t, repository, job, fact, recipients,
		)
		assertUserCanonDatabaseReceipt(
			t, repository, operationID, message.ID, world.ID,
			roleplay.UserPersonaCharacter, actorID, fact, recipients, other.ID,
		)
	})
}

func setupUserCanonDatabaseChannel(
	t *testing.T,
	repository *Repository,
	channelID string,
	worldName string,
) (model.Channel, roleplay.World, roleplay.Character) {
	t.Helper()
	channel, err := repository.CreateRoleplayChannel(t.Context(), model.Channel{
		ID: model.ChannelID(channelID), Scope: model.ChannelScopeUser, Name: worldName,
		WorkspaceRoot: "/srv/workspaces/" + channelID, Mode: model.ChannelModeRoleplay,
	}, worldName, "Mara")
	if err != nil {
		t.Fatal(err)
	}
	store, err := roleplay.NewStore(repository.pool)
	if err != nil {
		t.Fatal(err)
	}
	world, found, err := store.FindWorldByChannel(t.Context(), string(channel.ID))
	if err != nil || !found {
		t.Fatalf("world found=%t err=%v", found, err)
	}
	other, err := store.CreateCharacter(t.Context(), world.ID, "Ivo")
	if err != nil {
		t.Fatal(err)
	}
	configureRoleplayQueueTestScene(
		t, store, world.ID, string(channel.RoleplayViewpointCharacterID), other.ID,
	)
	return channel, world, other
}

func completeUserCanonDatabaseTurn(
	t *testing.T,
	repository *Repository,
	job model.Job,
	fact string,
	recipients []model.RoleplayCharacterID,
) LifecycleOperationID {
	t.Helper()
	var metadata channelTurnMetadata
	if err := json.Unmarshal(job.Metadata, &metadata); err != nil {
		t.Fatal(err)
	}
	responses := make([]RoleplayResponseCompletion, len(metadata.RoleplayResponders))
	outputs := make([]string, len(responses))
	for index, responder := range metadata.RoleplayResponders {
		output := fmt.Sprintf("%s acknowledges the accepted contribution.", responder.CharacterID)
		responses[index] = RoleplayResponseCompletion{
			Position: index, CharacterID: model.RoleplayCharacterID(responder.CharacterID),
			Output: output,
		}
		outputs[index] = output
	}
	claim, err := repository.ClaimNextStep(t.Context(), "user-canon-worker")
	if err != nil {
		t.Fatal(err)
	}
	if claim == nil || claim.Job.ID != job.ID {
		t.Fatalf("claim=%+v want job %d", claim, job.ID)
	}
	operationID := testLifecycleOperationID(t, "user-canon-completion", claim.Step.ID)
	command := CompleteStepEvidenceCommand{CompleteStepCommand: CompleteStepCommand{
		OperationID: operationID, Authority: claim.Authority, StepID: claim.Step.ID,
		Output: strings.Join(outputs, "\n\n"), ContextKey: "objective_result",
		ContextValue: "user-canon-proof", RoleplayResponses: responses,
		RoleplayUserCanon: &RoleplayUserCanonCompletion{
			Facts: []string{fact}, KnowledgeCharacterIDs: append(
				[]model.RoleplayCharacterID{}, recipients...,
			),
		},
	}}
	if err := repository.CompleteStepWithEvidence(t.Context(), command); err != nil {
		t.Fatal(err)
	}
	if err := repository.CompleteStepWithEvidence(t.Context(), command); err != nil {
		t.Fatalf("exact user canon lifecycle replay: %v", err)
	}
	return operationID
}

func assertUserCanonDatabaseReceipt(
	t *testing.T,
	repository *Repository,
	operationID LifecycleOperationID,
	sourceMessageID int64,
	worldID string,
	persona roleplay.UserPersonaKind,
	actorID string,
	fact string,
	recipients []model.RoleplayCharacterID,
	nonrecipientID string,
) {
	t.Helper()
	var storedMessageID int64
	var storedPersona string
	var storedActor *string
	var factsJSON, recipientsJSON []byte
	if err := repository.pool.QueryRow(t.Context(), `
		SELECT source_message_id,persona_kind,actor_character_id,
		       facts,knowledge_character_ids
		FROM roleplay_user_canon_completions WHERE operation_id=$1
	`, operationID).Scan(
		&storedMessageID, &storedPersona, &storedActor, &factsJSON, &recipientsJSON,
	); err != nil {
		t.Fatal(err)
	}
	var storedFacts []string
	var storedRecipients []model.RoleplayCharacterID
	if json.Unmarshal(factsJSON, &storedFacts) != nil ||
		json.Unmarshal(recipientsJSON, &storedRecipients) != nil ||
		storedMessageID != sourceMessageID || storedPersona != string(persona) ||
		!slices.Equal(storedFacts, []string{fact}) ||
		!slices.Equal(storedRecipients, recipients) ||
		(actorID == "" && storedActor != nil) ||
		(actorID != "" && (storedActor == nil || *storedActor != actorID)) {
		t.Fatalf(
			"user canon receipt message/persona/actor/facts/recipients=%d/%s/%v/%v/%v",
			storedMessageID, storedPersona, storedActor, storedFacts, storedRecipients,
		)
	}
	for _, recipient := range recipients {
		var knowledge, memory int
		if err := repository.pool.QueryRow(t.Context(), `
			SELECT
			 (SELECT COUNT(*) FROM roleplay_canon_events AS event
			  JOIN roleplay_character_knowledge AS knowledge
			    ON knowledge.world_id=event.world_id AND knowledge.canon_event_id=event.id
			  WHERE event.world_id=$1 AND event.source_message_id=$2
			    AND event.content=$3 AND knowledge.character_id=$4),
			 (SELECT COUNT(*) FROM roleplay_canon_events AS event
			  JOIN roleplay_character_memories AS memory
			    ON memory.world_id=event.world_id AND memory.source_event_id=event.id
			  WHERE event.world_id=$1 AND event.source_message_id=$2
			    AND event.content=$3 AND memory.character_id=$4 AND memory.content=$3)
		`, worldID, sourceMessageID, fact, recipient).Scan(&knowledge, &memory); err != nil {
			t.Fatal(err)
		}
		if knowledge != 1 || memory != 1 {
			t.Fatalf("recipient %s knowledge/memory=%d/%d", recipient, knowledge, memory)
		}
	}
	if nonrecipientID != "" {
		var knowledge, memory int
		if err := repository.pool.QueryRow(t.Context(), `
			SELECT
			 (SELECT COUNT(*) FROM roleplay_canon_events AS event
			  JOIN roleplay_character_knowledge AS knowledge
			    ON knowledge.world_id=event.world_id AND knowledge.canon_event_id=event.id
			  WHERE event.world_id=$1 AND event.source_message_id=$2
			    AND knowledge.character_id=$3),
			 (SELECT COUNT(*) FROM roleplay_canon_events AS event
			  JOIN roleplay_character_memories AS memory
			    ON memory.world_id=event.world_id AND memory.source_event_id=event.id
			  WHERE event.world_id=$1 AND event.source_message_id=$2
			    AND memory.character_id=$3)
		`, worldID, sourceMessageID, nonrecipientID).Scan(&knowledge, &memory); err != nil {
			t.Fatal(err)
		}
		if knowledge != 0 || memory != 0 {
			t.Fatalf(
				"nonrecipient %s received user canon knowledge/memory=%d/%d",
				nonrecipientID, knowledge, memory,
			)
		}
	}
}
