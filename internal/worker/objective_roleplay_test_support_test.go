package worker

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/contextcompiler"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/roleplay"
)

type roleplayContextProviderProbe struct {
	availabilityCalls int
	availability      contextcompiler.SearchAvailability
	contextCalls      int
	contextSet        contextcompiler.CandidateSet
	terms             []string
	authority         turnAuthority
	preparation       *roleplay.SimulationTurnAuthority
	projection        *roleplay.NarrativeSimulationProjection
}

func (provider *roleplayContextProviderProbe) ContextSearchAvailability(
	_ context.Context,
	_ model.Job,
	authority turnAuthority,
	preparation *roleplay.SimulationTurnAuthority,
	projection *roleplay.NarrativeSimulationProjection,
) (contextcompiler.SearchAvailability, error) {
	provider.availabilityCalls++
	provider.authority = authority
	if preparation != nil {
		copy := *preparation
		provider.preparation = &copy
	}
	if projection != nil {
		copy := roleplay.CloneNarrativeSimulationProjection(*projection)
		provider.projection = &copy
	}
	if provider.availability == "" {
		return contextcompiler.SearchAvailable, nil
	}
	return provider.availability, nil
}

func (provider *roleplayContextProviderProbe) ContextCandidates(
	_ context.Context,
	_ model.Job,
	authority turnAuthority,
	preparation *roleplay.SimulationTurnAuthority,
	projection *roleplay.NarrativeSimulationProjection,
	terms []string,
) (contextcompiler.CandidateSet, error) {
	provider.contextCalls++
	provider.terms = append([]string{}, terms...)
	provider.authority = authority
	if preparation != nil {
		copy := *preparation
		provider.preparation = &copy
	}
	if projection != nil {
		copy := roleplay.CloneNarrativeSimulationProjection(*projection)
		provider.projection = &copy
	}
	return provider.contextSet, nil
}

type scriptedRoleplayCanonStation struct {
	calls     int
	input     assemblyline.RoleplayCanonExtractionInput
	inputs    []assemblyline.RoleplayCanonExtractionInput
	facts     []string
	userFacts []string
}

func (station *scriptedRoleplayCanonStation) ExtractCanon(
	_ context.Context,
	input assemblyline.RoleplayCanonExtractionInput,
) (assemblyline.RoleplayCanonExtractionDecision, objectiveStationReceipt, error) {
	station.calls++
	station.input = input
	station.inputs = append(station.inputs, input)
	facts := station.facts
	if input.Source.Kind == assemblyline.RoleplayCanonSourceUserContribution {
		facts = station.userFacts
	}
	return assemblyline.RoleplayCanonExtractionDecision{
		Schema: assemblyline.RoleplayCanonExtractionSchemaV1,
		Facts:  append([]string{}, facts...),
	}, objectiveStationReceipt{Calls: 1}, nil
}

func acceptAllRoleplayCanonFacts(
	_ context.Context,
	_ string,
	candidates []string,
) ([]string, error) {
	return append([]string{}, candidates...), nil
}

func roleplayNarrativeFixtureFingerprint(
	t *testing.T,
	projection roleplay.NarrativeSimulationProjection,
	authority roleplay.SimulationNarrativeAuthority,
) string {
	t.Helper()
	authority.Fingerprint = ""
	type fingerprintScene struct {
		Title               string `json:"title"`
		Description         string `json:"description"`
		ActiveCharacterName string `json:"active_character_name"`
	}
	type fingerprintContent struct {
		Schema         string                            `json:"schema"`
		Scene          fingerprintScene                  `json:"scene"`
		Participants   []string                          `json:"participants"`
		Viewpoint      roleplay.NarrativePersona         `json:"viewpoint"`
		OngoingActions []roleplay.NarrativeOngoingAction `json:"ongoing_actions,omitempty"`
		Meters         []roleplay.NarrativeMeter         `json:"meters"`
		Inventory      []roleplay.NarrativeInventoryItem `json:"inventory"`
		VisibleFacts   []string                          `json:"visible_facts"`
		Memories       []string                          `json:"memories"`
		RecentEvents   []string                          `json:"recent_events"`
	}
	payload, err := json.Marshal(struct {
		Content   fingerprintContent                    `json:"content"`
		Authority roleplay.SimulationNarrativeAuthority `json:"authority"`
	}{Content: fingerprintContent{
		Schema: projection.Schema,
		Scene: fingerprintScene{
			Title: projection.Scene.Title, Description: projection.Scene.Description,
			ActiveCharacterName: projection.Scene.ActiveCharacterName,
		},
		Participants: projection.Participants, Viewpoint: projection.Viewpoint,
		OngoingActions: projection.OngoingActions,
		Meters:         projection.Meters, Inventory: projection.Inventory,
		VisibleFacts: projection.VisibleFacts, Memories: projection.Memories,
		RecentEvents: projection.RecentEvents,
	}, Authority: authority})
	if err != nil {
		t.Fatal(err)
	}
	baseDigest := sha256.Sum256(payload)
	clock := projection.Scene.Initiative
	digest := sha256.Sum256([]byte(fmt.Sprintf(
		"%x:%d:%d:%d", baseDigest[:], clock.Round, clock.Turn, clock.FictionalTimeTick,
	)))
	return fmt.Sprintf("%x", digest[:])
}

func narratorDirectionTurn(exactText string) roleplay.UserTurnAuthority {
	return roleplay.UserTurnAuthority{
		PersonaKind: roleplay.UserPersonaNarrator, PersonaName: roleplay.NarratorPersonaName,
		ContributionKind: roleplay.UserContributionDirection, ExactText: exactText,
	}
}

func narratorNarrationTurn(exactText string) roleplay.UserTurnAuthority {
	return roleplay.UserTurnAuthority{
		PersonaKind: roleplay.UserPersonaNarrator, PersonaName: roleplay.NarratorPersonaName,
		ContributionKind: roleplay.UserContributionNarration, ExactText: exactText,
	}
}

func narratorCommandTurn(exactText string) roleplay.UserTurnAuthority {
	return roleplay.UserTurnAuthority{
		PersonaKind: roleplay.UserPersonaNarrator, PersonaName: roleplay.NarratorPersonaName,
		ContributionKind: roleplay.UserContributionCommand, ExactText: exactText,
	}
}
