package cognitiongauntlet

import (
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/taskstate"
)

func (state *ablationState) residentContext() (workingSetContext, []cognition.Observation, error) {
	if state.workingSet == nil {
		return workingSetContext{}, nil, fmt.Errorf("ablation Working Set is unavailable")
	}
	resident := state.workingSet.ResidentItems()
	contents := make([]string, 0, len(resident))
	sourceRefs := make([]taskstate.Ref, 0, len(resident))
	for _, item := range resident {
		material, exists := state.workingMaterials[item.ID]
		if !exists {
			return workingSetContext{}, nil, fmt.Errorf("resident ablation item %q has no exact material", item.ID)
		}
		contents = append(contents, material.Content)
		sourceRefs = append(sourceRefs, material.SourceRefs...)
	}
	observations, err := state.observationsForSourceRefs(sourceRefs)
	if err != nil {
		return workingSetContext{}, nil, err
	}
	return workingSetContext{
		Snapshot: state.workingSet.Snapshot(), Items: contents,
	}, observations, nil
}

func (state *ablationState) projectedWorkingMaterials() (
	[]ablationMaterial,
	[]cognition.Observation,
	error,
) {
	if state.workingSet == nil {
		return nil, nil, fmt.Errorf("ablation Working Set is unavailable")
	}
	resident := state.workingSet.ResidentItems()
	materials := make([]ablationMaterial, 0, len(resident))
	sources := make([]taskstate.Ref, 0, len(resident))
	for _, item := range resident {
		material, exists := state.workingMaterials[item.ID]
		if !exists {
			return nil, nil, fmt.Errorf("resident ablation item %q has no exact material", item.ID)
		}
		materials = append(materials, material)
		sources = append(sources, material.SourceRefs...)
	}
	if len(materials) == 0 {
		return nil, nil, fmt.Errorf("ablation Working Set has no resident evidence")
	}
	observations, err := state.observationsForSourceRefs(sources)
	return materials, observations, err
}

func (state *ablationState) observationsForSourceRefs(
	refs []taskstate.Ref,
) ([]cognition.Observation, error) {
	wanted := make(map[string]struct{}, len(refs))
	for _, ref := range refs {
		wanted[taskstate.RefIdentity(ref)] = struct{}{}
	}
	result := make([]cognition.Observation, 0, len(wanted))
	for _, observation := range state.observations {
		identity := taskstate.RefIdentity(ablationObservationRef(observation))
		if _, exists := wanted[identity]; exists {
			result = append(result, observation)
			delete(wanted, identity)
		}
	}
	if len(wanted) != 0 {
		return nil, fmt.Errorf("ablation Working Set source lineage does not resolve to legal observations")
	}
	return result, nil
}
