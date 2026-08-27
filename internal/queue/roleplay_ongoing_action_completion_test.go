package queue

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/roleplay"
)

func TestRoleplayCompletionPreservesHistoricalNilLeavesAndExplicitActorLeaf(t *testing.T) {
	t.Parallel()
	action := "Mara is hauling the anchor line toward the rail."
	responses := []RoleplayResponseCompletion{
		{
			Position: 0, CharacterID: model.RoleplayCharacterID("rpc_11111111111111111111111111111111"),
			Output: "Mara keeps hauling the line.", Facts: []string{},
			KnowledgeCharacterIDs: []model.RoleplayCharacterID{}, OngoingAction: &action,
		},
		{
			Position: 1, CharacterID: model.RoleplayCharacterID("rpc_22222222222222222222222222222222"),
			Output: "Ivo finishes his knot.", Facts: []string{},
			KnowledgeCharacterIDs: []model.RoleplayCharacterID{}, OngoingAction: nil,
		},
	}
	normalized, err := normalizeRoleplayResponseCompletions(responses)
	if err != nil {
		t.Fatal(err)
	}
	action = "mutated caller text"
	if normalized[0].OngoingAction == nil ||
		*normalized[0].OngoingAction != "Mara is hauling the anchor line toward the rail." ||
		normalized[1].OngoingAction != nil {
		t.Fatalf("normalized actions=%v %v", normalized[0].OngoingAction, normalized[1].OngoingAction)
	}
	raw, err := json.Marshal(normalized)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), `"ongoing_action":null`) ||
		strings.Contains(string(raw), `"previous_ongoing_action":null`) {
		t.Fatalf("nil response leaves changed historical lifecycle bytes: %s", raw)
	}
	actorRaw, err := json.Marshal(CompleteStepCommand{
		OperationID: LifecycleOperationID("lifecycle_operation_" + strings.Repeat("a", 64)),
		StepID:      1, Output: "response", ContextKey: "objective_result", ContextValue: "proof",
		RoleplayUserOngoingAction: &RoleplayUserOngoingActionCompletion{
			CharacterID: model.RoleplayCharacterID("rpc_33333333333333333333333333333333"),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, exact := range []string{
		`"roleplay_user_ongoing_action":{`, `"previous_ongoing_action":null`,
		`"ongoing_action":null`,
	} {
		if !strings.Contains(string(actorRaw), exact) {
			t.Fatalf("actor leaf omitted exact null authority %q: %s", exact, actorRaw)
		}
	}

	blank := " "
	responses[0].OngoingAction = &blank
	if _, err := normalizeRoleplayResponseCompletions(responses); err == nil ||
		!strings.Contains(err.Error(), "ongoing action") {
		t.Fatalf("blank ongoing action error=%v", err)
	}
}

func TestRoleplayCompletionRejectsOutputBeyondNarrativeBound(t *testing.T) {
	t.Parallel()
	response := RoleplayResponseCompletion{
		Position: 0, CharacterID: model.RoleplayCharacterID("rpc_11111111111111111111111111111111"),
		Output: strings.Repeat("x", roleplay.MaxNarrativeResponseBytes+1),
		Facts:  []string{}, KnowledgeCharacterIDs: []model.RoleplayCharacterID{},
	}
	if _, err := normalizeRoleplayResponseCompletions([]RoleplayResponseCompletion{response}); err == nil ||
		!strings.Contains(err.Error(), "2048-byte narrative bound") {
		t.Fatalf("oversized roleplay response error=%v", err)
	}
	if err := model.ValidateChannelMessage(model.ChannelMessageRoleAssistant, response.Output); err != nil {
		t.Fatalf("generic assistant bound was narrowed: %v", err)
	}
}

func TestRoleplayOngoingActionMigrationSerializesEveryCharacterChainWrite(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("../../migrations/149_roleplay_ongoing_action_authority.sql")
	if err != nil {
		t.Fatal(err)
	}
	source := strings.Join(strings.Fields(string(raw)), " ")
	lock := "FROM roleplay_characters AS character WHERE character.world_id=NEW.world_id AND character.id=NEW.character_id FOR UPDATE;"
	if count := strings.Count(source, lock); count != 2 {
		t.Fatalf("ongoing-action character serialization locks=%d want 2", count)
	}
}
