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
	contextCalls int
	contextSet   contextcompiler.CandidateSet
	terms        []string
	authority    turnAuthority
	preparation  *roleplay.SimulationTurnAuthority
	projection   *roleplay.NarrativeSimulationProjection
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
	provider.terms = append([]string(nil), terms...)
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
	calls int
	input assemblyline.RoleplayCanonExtractionInput
	facts []string
}

func (station *scriptedRoleplayCanonStation) ExtractCanon(
	_ context.Context,
	input assemblyline.RoleplayCanonExtractionInput,
) (assemblyline.RoleplayCanonExtractionDecision, objectiveStationReceipt, error) {
	station.calls++
	station.input = input
	return assemblyline.RoleplayCanonExtractionDecision{
		Schema: assemblyline.RoleplayCanonExtractionSchemaV1,
		Facts:  append([]string(nil), station.facts...),
	}, objectiveStationReceipt{Calls: 1}, nil
}

func roleplayNarrativeFixtureFingerprint(
	t *testing.T,
	projection roleplay.NarrativeSimulationProjection,
	authority roleplay.SimulationNarrativeAuthority,
) string {
	t.Helper()
	authority.Fingerprint = ""
	payload, err := json.Marshal(struct {
		Content   roleplay.NarrativeSimulationProjection `json:"content"`
		Authority roleplay.SimulationNarrativeAuthority  `json:"authority"`
	}{Content: projection, Authority: authority})
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(payload)
	return fmt.Sprintf("%x", digest[:])
}

func narratorDirectionTurn(exactText string) roleplay.UserTurnAuthority {
	return roleplay.UserTurnAuthority{
		PersonaKind: roleplay.UserPersonaNarrator, PersonaName: roleplay.NarratorPersonaName,
		ContributionKind: roleplay.UserContributionDirection, ExactText: exactText,
	}
}

func narratorCommandTurn(exactText string) roleplay.UserTurnAuthority {
	return roleplay.UserTurnAuthority{
		PersonaKind: roleplay.UserPersonaNarrator, PersonaName: roleplay.NarratorPersonaName,
		ContributionKind: roleplay.UserContributionCommand, ExactText: exactText,
	}
}
