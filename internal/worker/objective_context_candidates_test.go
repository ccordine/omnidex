package worker

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
	"github.com/gryph/omnidex/internal/roleplay"
)

func TestBuildContextCandidateAuthoritiesPreservesEveryRequiredChunk(t *testing.T) {
	requiredText := strings.Repeat("r", assemblyline.MaxContextCandidateContentBytes+1)
	required, optional, err := buildContextCandidateAuthorities(
		[]queue.ContextSearchRecord{{
			Namespace: "simulation_transition", SourceID: "transition-1", Content: requiredText,
		}},
		[]queue.ContextSearchRecord{{
			Namespace: "fictional_canon", SourceID: "fact-1", Content: "A separate optional fact.",
		}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(required) != 2 {
		t.Fatalf("required chunks=%d, want 2", len(required))
	}
	var reconstructed strings.Builder
	for _, authority := range required {
		parts := strings.SplitN(authority.Content, "\n", 2)
		if len(parts) != 2 || !strings.HasPrefix(parts[0], "Segment ") {
			t.Fatalf("required chunk lacks deterministic framing: %q", authority.Content)
		}
		reconstructed.WriteString(parts[1])
	}
	if reconstructed.String() != requiredText {
		t.Fatal("required chunks did not preserve the exact source text")
	}
	if len(optional) != 1 {
		t.Fatalf("optional candidates=%d, want 1", len(optional))
	}
}

func TestBuildContextCandidateAuthoritiesPagesOversizedRequiredContextWithoutLoss(t *testing.T) {
	content := strings.Repeat("r", assemblyline.MaxContextCandidateProjectionBytes+1)
	required, optional, err := buildContextCandidateAuthorities([]queue.ContextSearchRecord{{
		Namespace: "simulation_transition", SourceID: "transition-1",
		Content: content,
	}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(required) < 4 || len(optional) != 0 {
		t.Fatalf("required/optional chunks=%d/%d", len(required), len(optional))
	}
	var reconstructed strings.Builder
	for _, authority := range required {
		parts := strings.SplitN(authority.Content, "\n", 2)
		if len(parts) != 2 {
			t.Fatalf("required chunk lacks framing: %q", authority.Content)
		}
		reconstructed.WriteString(parts[1])
	}
	if reconstructed.String() != content {
		t.Fatal("paged required context lost source bytes")
	}
}

func TestBuildContextCandidateAuthoritiesPreservesOptionalContextBeyondOneRelevancePage(t *testing.T) {
	records := make([]queue.ContextSearchRecord, assemblyline.MaxContextCandidateAuthorities+1)
	for index := range records {
		records[index] = queue.ContextSearchRecord{
			Namespace: "conversation_exchange",
			SourceID:  fmt.Sprintf("exchange-%d", index),
			Content:   fmt.Sprintf("Distinct optional context %d.", index),
		}
	}
	required, optional, err := buildContextCandidateAuthorities(nil, records)
	if err != nil {
		t.Fatal(err)
	}
	if len(required) != 0 || len(optional) != len(records) {
		t.Fatalf("required/optional=%d/%d, want 0/%d", len(required), len(optional), len(records))
	}
	for index, authority := range optional {
		if authority.CandidateID != fmt.Sprintf("CTX_%d", index+1) {
			t.Fatalf("optional %d ID=%q", index, authority.CandidateID)
		}
	}
}

func TestBuildContextCandidateAuthoritiesExactDeduplicatesAcrossNamespaces(t *testing.T) {
	const duplicate = "The pressure door is sealed."
	required, optional, err := buildContextCandidateAuthorities(nil, []queue.ContextSearchRecord{
		{Namespace: "fictional_canon", SourceID: "canon-1", Content: duplicate},
		{Namespace: "character_memory", SourceID: "memory-1", Content: duplicate},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(required) != 0 || len(optional) != 1 {
		t.Fatalf("required/optional=%d/%d, want 0/1", len(required), len(optional))
	}
}

func TestInterleaveContextRecordGroupsPreventsOneProviderFromStarvingOthers(t *testing.T) {
	recent := []queue.ContextSearchRecord{
		{Namespace: "conversation_exchange", SourceID: "recent-1", Content: "recent one"},
		{Namespace: "conversation_exchange", SourceID: "recent-2", Content: "recent two"},
	}
	searched := []queue.ContextSearchRecord{
		{Namespace: "conversation_user", SourceID: "searched-1", Content: "searched one"},
		{Namespace: "conversation_user", SourceID: "searched-2", Content: "searched two"},
	}
	memory := []queue.ContextSearchRecord{
		{Namespace: "durable_memory", SourceID: "memory-1", Content: "memory one"},
	}
	records := interleaveContextRecordGroups(recent, searched, memory)
	want := []string{"recent-1", "searched-1", "memory-1", "recent-2", "searched-2"}
	if len(records) != len(want) {
		t.Fatalf("records=%#v", records)
	}
	for index, sourceID := range want {
		if records[index].SourceID != sourceID {
			t.Fatalf("record %d source=%q want %q", index, records[index].SourceID, sourceID)
		}
	}
}

func TestRoundRobinDurableMemoryMatchesPreventsTermStarvation(t *testing.T) {
	groups := [][]model.MemoryMatch{
		{
			{ID: 1, Content: "alpha one"}, {ID: 2, Content: "alpha two"},
			{ID: 3, Content: "alpha three"}, {ID: 4, Content: "alpha four"},
			{ID: 5, Content: "alpha five"}, {ID: 6, Content: "alpha six"},
			{ID: 7, Content: "alpha seven"}, {ID: 8, Content: "alpha eight"},
		},
		{
			{ID: 101, Content: "beta one"}, {ID: 102, Content: "beta two"},
			{ID: 103, Content: "beta three"}, {ID: 104, Content: "beta four"},
			{ID: 105, Content: "beta five"}, {ID: 106, Content: "beta six"},
			{ID: 107, Content: "beta seven"}, {ID: 108, Content: "beta eight"},
		},
	}
	merged, err := roundRobinMemoryMatches(groups, contextSearchedRecordLimit)
	if err != nil {
		t.Fatal(err)
	}
	want := []int64{1, 101, 2, 102, 3, 103, 4, 104}
	if len(merged) != len(want) {
		t.Fatalf("merged memories=%#v", merged)
	}
	for index, id := range want {
		if merged[index].ID != id {
			t.Fatalf("merged memory %d=%d want %d", index, merged[index].ID, id)
		}
	}
}

func TestRoundRobinMemoryMatchesSkipsOverlapWithinEachFairTurn(t *testing.T) {
	merged, err := roundRobinMemoryMatches([][]model.MemoryMatch{
		{{ID: 1}, {ID: 2}, {ID: 3}, {ID: 4}, {ID: 5}, {ID: 6}, {ID: 7}, {ID: 8}},
		{{ID: 1}, {ID: 2}, {ID: 101}, {ID: 102}},
	}, contextSearchedRecordLimit)
	if err != nil {
		t.Fatal(err)
	}
	seen := make(map[int64]struct{}, len(merged))
	for _, match := range merged {
		seen[match.ID] = struct{}{}
	}
	if _, exists := seen[101]; !exists {
		t.Fatalf("overlapping first query starved second query: %#v", merged)
	}
	if _, exists := seen[102]; !exists {
		t.Fatalf("overlapping first query starved second query: %#v", merged)
	}
}

func TestExactNewRoleplayFactsRemovesOnlyMechanicalZeroDeltas(t *testing.T) {
	got := exactNewRoleplayFacts(
		[]string{"The pressure door is sealed.", "A beacon begins transmitting.", "A beacon begins transmitting."},
		[]string{"The pressure door is sealed."},
	)
	if len(got) != 1 || got[0] != "A beacon begins transmitting." {
		t.Fatalf("new facts=%#v", got)
	}
}

func TestRecentConversationContextRecordsGroupsExactExchangesForAnaphora(t *testing.T) {
	records, err := recentConversationContextRecords([]queue.ConversationCandidateTurn{
		{MessageID: 10, Role: queue.ConversationCandidateUser, Content: "Rotate the rover antenna toward Earth."},
		{MessageID: 11, Role: queue.ConversationCandidateAssistant, PairedUserMessageID: 10, Content: "The antenna now points toward Earth."},
		{MessageID: 12, Role: queue.ConversationCandidateUser, Content: "Inventory the sample drawer."},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 || records[0].SourceID != "channel-message-12" ||
		records[1].SourceID != "channel-message-10-through-11" {
		t.Fatalf("records=%#v", records)
	}
	if !strings.Contains(records[1].Content, "Rotate the rover antenna") ||
		!strings.Contains(records[1].Content, "antenna now points") {
		t.Fatalf("grouped exchange=%q", records[1].Content)
	}
}

func TestRecentConversationContextRecordsRejectsUnpairedAssistant(t *testing.T) {
	_, err := recentConversationContextRecords([]queue.ConversationCandidateTurn{{
		MessageID: 11, Role: queue.ConversationCandidateAssistant,
		PairedUserMessageID: 10, Content: "An orphaned response.",
	}})
	if err == nil {
		t.Fatal("unpaired assistant authority was accepted")
	}
}

func TestRecentConversationContextRecordsPreservesRoleplaySpeakerAndContributionOwnership(t *testing.T) {
	userTurn := roleplay.UserTurnAuthority{
		PersonaKind: roleplay.UserPersonaCharacter,
		CharacterID: "rpc_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		PersonaName: "Gryph", PersonaSummary: "An artificer from afar.",
		ContributionKind: roleplay.UserContributionActionDialogue,
		ExactText:        `I set down the key. "Keep this safe," I tell Mara.`,
	}
	turns := []queue.ConversationCandidateTurn{
		{MessageID: 20, Role: queue.ConversationCandidateUser, SpeakerName: "Gryph", RoleplayUserTurn: &userTurn, Content: userTurn.ExactText},
		{MessageID: 21, Role: queue.ConversationCandidateAssistant, PairedUserMessageID: 20, SpeakerName: "Mara Vey", Content: "Mara closes her hand around the key."},
	}
	records, err := recentConversationContextRecords(turns)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || !strings.Contains(records[0].Content, "Gryph [action_dialogue] contribution:") ||
		!strings.Contains(records[0].Content, "Mara Vey response:") {
		t.Fatalf("roleplay exchange=%#v", records)
	}
}

func TestCompletedConversationTurnsExcludeFailedUnansweredContribution(t *testing.T) {
	turns, err := completedConversationCandidateTurns([]queue.ConversationCandidateTurn{
		{MessageID: 30, Role: queue.ConversationCandidateUser, Content: "This turn failed."},
		{MessageID: 31, Role: queue.ConversationCandidateUser, Content: "This turn completed."},
		{MessageID: 32, Role: queue.ConversationCandidateAssistant, PairedUserMessageID: 31, Content: "Completed response."},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(turns) != 2 || turns[0].MessageID != 31 || turns[1].MessageID != 32 {
		t.Fatalf("completed turns=%#v", turns)
	}
}

func TestEmptyContextTermsAcquireRequiredRoleplayStateWithoutOptionalCanon(t *testing.T) {
	const (
		preparationID = "rpt_22222222222222222222222222222222"
		worldID       = "rpw_11111111111111111111111111111111"
		sceneID       = "rps_33333333333333333333333333333333"
		characterID   = "rpc_44444444444444444444444444444444"
	)
	createdAt := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	projection := roleplay.NarrativeSimulationProjection{
		Schema: roleplay.NarrativeSimulationProjectionSchemaV1,
		Scene: roleplay.NarrativeScene{
			Title: "Archive", Description: "A quiet gallery beneath the observatory.",
			ActiveCharacterName: "Mara",
		},
		Participants: []string{"Mara"},
		Viewpoint: roleplay.NarrativePersona{
			Name: "Mara", Summary: "A practical archivist.", Voice: "Plainspoken.",
			Traits: []string{}, Goals: []string{},
		},
		Meters: []roleplay.NarrativeMeter{}, Inventory: []roleplay.NarrativeInventoryItem{},
		VisibleFacts: []string{"IRRELEVANT_FROZEN_CANON_SENTINEL remains remote."},
		Memories:     []string{}, RecentEvents: []string{},
	}
	narrative := roleplay.SimulationNarrativeAuthority{
		WorldID: worldID, SceneID: sceneID, SceneRevision: 2, ViewpointID: characterID,
		ParticipantIDs: []string{characterID}, MeterKeys: []string{}, InventoryItemIDs: []string{},
		CanonEventIDs: []string{"rpe_55555555555555555555555555555555"}, MemoryIDs: []string{},
		TransitionIDs: []string{},
	}
	narrative.Fingerprint = roleplayNarrativeFixtureFingerprint(t, projection, narrative)
	transition := &roleplay.SimulationTransitionResult{
		Schema: roleplay.SimulationTransitionSchemaV1, OperationID: preparationID,
		WorldID: worldID, SceneID: sceneID, ActorCharacterID: characterID,
		BeforeRevision: 1, AfterRevision: 2,
		Action:          roleplay.SimulationAction{Kind: roleplay.SimulationActionInteraction, CommandKey: "open"},
		Effects:         []roleplay.SimulationEffect{{Sequence: 1, Kind: "scene_changed"}},
		NarrativeEvents: []string{"The west gate opened."}, CreatedAt: createdAt,
	}
	preparation := roleplay.SimulationTurnAuthority{
		PreparationID: preparationID, ChannelID: "empty-roleplay-terms", UserMessageID: 9,
		WorldID: worldID, SceneID: sceneID, BaseSceneRevision: 1, SceneRevision: 2,
		ActiveCharacterID: characterID, InputKind: roleplay.SimulationTurnAction, ExplicitAction: true,
		UserTurn:          narratorCommandTurn("/open"),
		PendingTransition: transition, ParticipantCharacterIDs: []string{characterID},
		GenerationConfig:    roleplayGenerationFixture("66666666666666666666666666666666"),
		NarrativeProjection: projection, NarrativeAuthority: narrative,
		NarrativeFingerprint: narrative.Fingerprint, CreatedAt: createdAt,
	}
	responder := roleplay.SimulationResponderAuthority{
		Position: 0, CharacterID: characterID,
		GenerationConfig:    preparation.GenerationConfig,
		NarrativeProjection: projection, NarrativeAuthority: narrative,
		NarrativeFingerprint: narrative.Fingerprint,
	}
	conversation := []queue.ContextSearchRecord{{
		Namespace: "conversation_exchange", SourceID: "channel-message-7-through-8",
		Content: "IRRELEVANT_PRIOR_RESPONSE_SENTINEL must not replace the current self-contained turn.",
	}}
	required, optionalConversation, rankable, err := roleplayContextRecordGroups(
		preparation, responder, conversation, []string{}, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	set, err := requiredContextCandidateSet(required, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(optionalConversation) != 0 || len(rankable) != 1 ||
		len(set.Required) != 3 || len(set.Optional) != 0 || set.Replan != nil ||
		set.Required[0].Namespace != "simulation_transition" ||
		set.Required[0].Content != "The west gate opened." ||
		set.Required[1].Namespace != "scene_state" ||
		set.Required[2].Namespace != "scene_participants" {
		t.Fatalf("empty-term roleplay acquisition=%#v", set)
	}
	for _, authority := range set.Required {
		if strings.Contains(authority.Content, "SENTINEL") {
			t.Fatalf("empty-term roleplay acquisition projected unrelated context: %#v", set)
		}
	}
}
