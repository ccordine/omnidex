package cognitiongauntlet

import (
	"encoding/json"
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/labyrinth"
	"github.com/gryph/omnidex/internal/taskstate"
	"github.com/gryph/omnidex/internal/workingset"
)

type ablationContext struct {
	Materials []ablationMaterial
	Evidence  []cognition.EvidenceRef
}

type transcriptContext struct {
	Goal         cognition.GoalExpression `json:"goal"`
	Observations []cognition.Observation  `json:"observations"`
	Actions      []ablationActionHistory  `json:"actions"`
}

type compactedContext struct {
	Goal               cognition.GoalExpression  `json:"goal"`
	CurrentObservation cognition.Observation     `json:"current_observation"`
	PriorActions       []cognition.ActionRequest `json:"prior_actions"`
}

type workingSetContext struct {
	Snapshot workingset.Snapshot `json:"snapshot"`
	Items    []string            `json:"resident_content"`
}

type oraclePacketContext struct {
	Contaminated bool                         `json:"contaminated"`
	NextAction   *cognition.ActionRequest     `json:"next_action,omitempty"`
	Evidence     []labyrinth.EvidenceIdentity `json:"evidence"`
}

func (state *ablationState) context(
	call uint32,
	authority ContaminatedEvidencePacket,
) (ablationContext, error) {
	task, err := state.taskMaterial()
	if err != nil {
		return ablationContext{}, err
	}
	contextMaterial, evidence, err := state.variantContextMaterial(call, authority)
	if err != nil {
		return ablationContext{}, err
	}
	materials := []ablationMaterial{task}
	materials = append(materials, contextMaterial...)
	return ablationContext{Materials: materials, Evidence: evidence}, nil
}

func (state *ablationState) variantContextMaterial(
	call uint32,
	authority ContaminatedEvidencePacket,
) ([]ablationMaterial, []cognition.EvidenceRef, error) {
	if len(state.observations) == 0 {
		return nil, nil, fmt.Errorf("ablation context has no legal observation")
	}
	switch state.variant {
	case VariantRawObservation:
		latest := state.observations[len(state.observations)-1]
		return []ablationMaterial{rawObservationMaterial(latest)}, []cognition.EvidenceRef{latest.EvidenceRef()}, nil
	case VariantFullTranscript:
		value := transcriptContext{
			Goal:         state.goal.Clone(),
			Observations: append([]cognition.Observation{}, state.observations...),
			Actions:      append([]ablationActionHistory{}, state.actions...),
		}
		return state.summaryMaterial(call, value, state.observations)
	case VariantTranscriptCompacted:
		prior := make([]cognition.ActionRequest, len(state.actions))
		for index := range state.actions {
			prior[index] = state.actions[index].Request.Clone()
		}
		latest := state.observations[len(state.observations)-1]
		value := compactedContext{
			Goal: state.goal.Clone(), CurrentObservation: latest, PriorActions: prior,
		}
		return state.summaryMaterial(call, value, []cognition.Observation{latest})
	case VariantTaskLedger:
		return state.summaryMaterial(call, state.ledger.MaterializedState(), state.observations)
	case VariantLedgerWorkingSet:
		resident, observations, err := state.residentContext()
		if err != nil {
			return nil, nil, err
		}
		return state.summaryMaterial(call, resident, observations)
	case VariantLedgerProjection:
		materials, observations, err := state.projectedWorkingMaterials()
		return materials, evidenceForObservations(observations), err
	case VariantOracleEvidence:
		packet := oraclePacketContext{Contaminated: true, Evidence: []labyrinth.EvidenceIdentity{}}
		if len(state.actions) < len(authority.Witness) {
			next := authority.Witness[len(state.actions)].Request.Clone()
			packet.NextAction = &next
			for _, use := range authority.EvidenceUses {
				if use.RequiredByActionID == authority.Witness[len(state.actions)].ID {
					packet.Evidence = append(packet.Evidence, use.Evidence)
				}
			}
		}
		return state.summaryMaterial(call, packet, state.observations)
	case VariantRawShell:
		value := struct {
			Contract string            `json:"shell_contract"`
			History  transcriptContext `json:"history"`
		}{
			Contract: rawShellContract,
			History: transcriptContext{
				Goal:         state.goal.Clone(),
				Observations: append([]cognition.Observation{}, state.observations...),
				Actions:      append([]ablationActionHistory{}, state.actions...),
			},
		}
		return state.summaryMaterial(call, value, state.observations)
	default:
		return nil, nil, fmt.Errorf("ablation context variant %q is not registered", state.variant)
	}
}

func rawObservationMaterial(observation cognition.Observation) ablationMaterial {
	return ablationMaterial{
		Ref: ablationObservationRef(observation), SourceRefs: []taskstate.Ref{},
		Role: workingset.RoleEvidence, Authority: taskstate.AuthorityToolEvidence,
		Content: observation.Content, Priority: 100,
	}
}

func (state *ablationState) summaryMaterial(
	call uint32,
	value any,
	sources []cognition.Observation,
) ([]ablationMaterial, []cognition.EvidenceRef, error) {
	content, err := marshalAblationContext(value)
	if err != nil {
		return nil, nil, err
	}
	refs, err := sourceRefsForObservations(sources)
	if err != nil {
		return nil, nil, err
	}
	material := ablationMaterial{
		Ref: ablationContentRef(
			fmt.Sprintf("cognition:episode/%s/material/call-%03d", state.episode.ID, call),
			fmt.Sprint(call), content, taskstate.RefEvidence,
		),
		SourceRefs: refs, Role: workingset.RoleEvidence,
		Authority: taskstate.AuthorityCode, Content: content, Priority: 100,
	}
	return []ablationMaterial{material}, evidenceForObservations(sources), nil
}

func marshalAblationContext(value any) (string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("encode ablation context: %w", err)
	}
	if len(raw) == 0 {
		return "", fmt.Errorf("ablation context is empty")
	}
	return string(raw), nil
}

func sourceRefsForObservations(observations []cognition.Observation) ([]taskstate.Ref, error) {
	if len(observations) > 32 {
		return nil, fmt.Errorf("ablation context source lineage exceeds 32 exact observations")
	}
	refs := make([]taskstate.Ref, len(observations))
	for index, observation := range observations {
		refs[index] = ablationObservationRef(observation)
	}
	return refs, nil
}

func evidenceForObservations(observations []cognition.Observation) []cognition.EvidenceRef {
	refs := make([]cognition.EvidenceRef, len(observations))
	for index, observation := range observations {
		refs[index] = observation.EvidenceRef()
	}
	return refs
}
