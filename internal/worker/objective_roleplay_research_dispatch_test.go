package worker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/roleplay"
	"github.com/gryph/omnidex/internal/webresearch"
)

func TestRoleplayResearchDispatchBypassesFictionalResponseAndCanon(t *testing.T) {
	authority, research, narrative := objectiveRoleplayResearchFixture(0, "How long is a Martian year?")
	narrative.Meters = []roleplay.NarrativeMeter{}
	narrative.Inventory = []roleplay.NarrativeInventoryItem{}
	narrative.VisibleFacts = []string{}
	narrative.Memories = []string{}
	narrative.RecentEvents = []string{}
	narrativeAuthority := roleplay.SimulationNarrativeAuthority{
		WorldID: authority.RoleplayWorldID, SceneID: authority.RoleplaySceneID,
		SceneRevision: authority.RoleplaySceneRevision,
		ViewpointID:   string(authority.RoleplayViewpointCharacterID),
		ParticipantIDs: []string{
			string(authority.RoleplayViewpointCharacterID),
		},
		MeterKeys: []string{}, InventoryItemIDs: []string{}, CanonEventIDs: []string{},
		MemoryIDs: []string{}, TransitionIDs: []string{},
	}
	fingerprint := roleplayNarrativeFixtureFingerprint(t, narrative, narrativeAuthority)
	narrativeAuthority.Fingerprint = fingerprint
	authority.RoleplayNarrativeFingerprint = fingerprint
	authority.RoleplayResponders[0].NarrativeFingerprint = fingerprint
	research.NarrativeFingerprint = fingerprint
	userTurn := narratorCommandTurn(authority.Instruction)
	preparation := roleplay.SimulationTurnAuthority{
		PreparationID: research.PreparationID, ChannelID: research.ChannelID,
		UserMessageID: research.UserMessageID, WorldID: research.WorldID, SceneID: research.SceneID,
		BaseSceneRevision: research.SceneRevision, SceneRevision: research.SceneRevision,
		ActiveCharacterID: research.CharacterID, InputKind: roleplay.SimulationTurnExternalCommand,
		UserTurn:                userTurn,
		ParticipantCharacterIDs: []string{research.CharacterID},
		GenerationConfig:        *authority.RoleplayGenerationConfig,
		NarrativeProjection:     narrative, NarrativeAuthority: narrativeAuthority,
		NarrativeFingerprint: fingerprint, CreatedAt: research.CreatedAt,
		Responders: []roleplay.SimulationResponderAuthority{{
			Position: 0, CharacterID: research.CharacterID,
			GenerationConfig:    *authority.RoleplayGenerationConfig,
			NarrativeProjection: narrative, NarrativeAuthority: narrativeAuthority,
			NarrativeFingerprint: fingerprint,
		}},
		ResponderRoutes: append(
			[]roleplay.SimulationResponderRoute(nil), authority.RoleplayResponders...,
		),
	}
	metadata, err := json.Marshal(map[string]any{
		"channel_id": authority.ChannelID, "channel_mode": authority.ChannelMode,
		"roleplay_viewpoint_character_id":    authority.RoleplayViewpointCharacterID,
		"roleplay_simulation_preparation_id": authority.RoleplaySimulationPreparationID,
		"roleplay_world_id":                  authority.RoleplayWorldID, "roleplay_scene_id": authority.RoleplaySceneID,
		"roleplay_scene_revision":            authority.RoleplaySceneRevision,
		"roleplay_input_kind":                authority.RoleplayInputKind,
		"roleplay_participant_character_ids": authority.RoleplayParticipantCharacterIDs,
		"roleplay_narrative_fingerprint":     authority.RoleplayNarrativeFingerprint,
		"roleplay_generation_config":         authority.RoleplayGenerationConfig,
		"roleplay_responders":                authority.RoleplayResponders,
		"roleplay_user_turn":                 userTurn,
	})
	if err != nil {
		t.Fatal(err)
	}
	evidenceItem, err := newObjectiveEvidence(
		"E01", "A Martian year lasts about 687 Earth days.",
		"web_document", "https://science.example/mars-year",
	)
	if err != nil {
		t.Fatal(err)
	}
	evidenceItem.SourceSHA256 = strings.Repeat("d", 64)
	evidenceItem.ObservedAt = time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC)
	evidenceItem.ParagraphMask = 1
	rendered := "Mars takes about 687 Earth days to orbit the Sun. [1]\n\n[1] https://science.example/mars-year"
	renderedDigest := sha256.Sum256([]byte(rendered))
	conversation := &scriptedObjectiveConversationStation{}
	canon := &scriptedRoleplayCanonStation{}
	kind := answerObjectiveKindStation()
	provider := &roleplayContextProviderProbe{}
	contextSieve := emptyContextSieveStation()
	result, err := runObjectiveTurn(
		context.Background(),
		model.Job{ID: authority.JobID, Pipeline: model.PipelineChat, Instruction: authority.Instruction, Metadata: metadata},
		provider, contextSieve, kind, conversation,
		&scriptedObjectiveAnswerStation{}, objectiveWorkflows{
			RoleplaySimulation: func(
				_ context.Context,
				preparationID string,
				jobID int64,
			) (roleplay.SimulationTurnAuthority, roleplay.NarrativeSimulationProjection, error) {
				if preparationID != research.PreparationID || jobID != authority.JobID {
					t.Fatalf("preparation=%q job=%d", preparationID, jobID)
				}
				return preparation, narrative, nil
			},
			RoleplayCanon: canon,
			RoleplayResearch: func(context.Context, turnAuthority) (objectiveRoleplayResearchAnswer, error) {
				return objectiveRoleplayResearchAnswer{
					Research: research, Text: "Mars takes about 687 Earth days to orbit the Sun.",
					Rendered: rendered, RenderedSHA256: hex.EncodeToString(renderedDigest[:]),
					Paragraphs: []webresearch.GroundedParagraph{{Text: "Mars takes about 687 Earth days to orbit the Sun."}},
					Evidence:   []objectiveEvidence{evidenceItem}, EvidenceIDs: []string{"E01"}, ModelCalls: 3,
				}, nil
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Kind != assemblyline.ObjectiveKindExternalAnswer || !result.Complete ||
		result.RoleplayResearch == nil || len(result.Citations) != 1 ||
		len(result.RoleplayResponses) != 0 ||
		result.ModelCalls != 4 {
		t.Fatalf("research result=%#v", result)
	}
	if kind.calls != 0 || conversation.calls != 0 || canon.calls != 0 {
		t.Fatalf("research reached fictional stations: kind=%d response=%d canon=%d", kind.calls, conversation.calls, canon.calls)
	}
	if provider.contextCalls != 1 || contextSieve.termCalls != 1 ||
		contextSieve.relevanceCalls != 0 || contextSieve.minificationCalls != 0 {
		t.Fatalf(
			"research context sieve provider=%d stations=(%d,%d,%d)",
			provider.contextCalls, contextSieve.termCalls,
			contextSieve.relevanceCalls, contextSieve.minificationCalls,
		)
	}
	_, records, err := prepareObjectiveTurnCompletion(result)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].Metadata["authority_namespace"] != string(roleplay.AuthorityRealWorld) ||
		records[0].Metadata["roleplay_research_preparation_id"] != research.PreparationID {
		t.Fatalf("research evidence receipt=%#v", records)
	}
}
