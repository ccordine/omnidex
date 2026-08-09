package contextbuilder

import (
	"fmt"
	"sort"

	"github.com/gryph/omnidex/internal/workingset"
)

type buildState struct {
	input     BuildInput
	materials map[workingset.ItemID]Material
	items     []workingset.Item
	selected  []selectedMaterial
	omissions map[workingset.ItemID]Omission
	selectors map[workingset.Role]string
}

func Build(input BuildInput) (Projection, error) {
	materials, err := validateBuildInput(input)
	if err != nil {
		return Projection{}, err
	}
	state := buildState{
		input: input, materials: materials,
		items:     sortedItems(input.WorkingSet.ResidentItems()),
		omissions: make(map[workingset.ItemID]Omission),
		selectors: make(map[workingset.Role]string),
	}
	for _, selector := range input.Spec.Required {
		state.selectors[selector.Role] = selector.ID
	}
	for _, selector := range input.Spec.Optional {
		state.selectors[selector.Role] = selector.ID
	}
	requiredExtras := make([]struct {
		selectorID string
		candidate  selectedMaterial
	}, 0)
	for _, selector := range sortedSelectors(input.Spec.Required) {
		extras, err := state.selectRequiredMinimum(selector)
		if err != nil {
			return Projection{}, err
		}
		for _, candidate := range extras {
			requiredExtras = append(requiredExtras, struct {
				selectorID string
				candidate  selectedMaterial
			}{selectorID: selector.ID, candidate: candidate})
		}
	}
	if len(state.selected) > input.Spec.MaxItems {
		return Projection{}, fmt.Errorf("%w: required selectors need %d items but the ceiling is %d",
			ErrBudgetExceeded, len(state.selected), input.Spec.MaxItems)
	}
	if _, err := renderMaterials(input.Spec, state.selected); err != nil {
		return Projection{}, fmt.Errorf("%w: %v", ErrBudgetExceeded, err)
	}
	for _, extra := range requiredExtras {
		state.trySelect(extra.selectorID, extra.candidate)
	}
	for _, selector := range sortedSelectors(input.Spec.Optional) {
		state.selectOptional(selector)
	}
	state.omitUnselectedRoles()
	return state.projection()
}

func (state *buildState) selectRequiredMinimum(selector Selector) ([]selectedMaterial, error) {
	eligible := state.eligible(selector)
	if len(eligible) < selector.MinItems {
		return nil, fmt.Errorf("%w: selector %q resolved %d of %d required items",
			ErrRequiredSelector, selector.ID, len(eligible), selector.MinItems)
	}
	limit := selector.MaxItems
	if len(eligible) < limit {
		limit = len(eligible)
	}
	state.selected = append(state.selected, eligible[:selector.MinItems]...)
	state.sortSelected()
	for _, item := range eligible[limit:] {
		state.omit(item.item, selector.ID, OmittedSelectorLimit, &item.material)
	}
	return append([]selectedMaterial(nil), eligible[selector.MinItems:limit]...), nil
}

func (state *buildState) selectOptional(selector Selector) {
	eligible := state.eligible(selector)
	selected := 0
	for _, candidate := range eligible {
		if selected == selector.MaxItems {
			state.omit(candidate.item, selector.ID, OmittedSelectorLimit, &candidate.material)
			continue
		}
		if state.trySelect(selector.ID, candidate) {
			selected++
		}
	}
}

func (state *buildState) trySelect(selectorID string, candidate selectedMaterial) bool {
	if len(state.selected) == state.input.Spec.MaxItems {
		state.omit(candidate.item, selectorID, OmittedItemBudget, &candidate.material)
		return false
	}
	trial := append(append([]selectedMaterial(nil), state.selected...), candidate)
	sortSelectedMaterials(trial)
	if _, err := renderMaterials(state.input.Spec, trial); err != nil {
		state.omit(candidate.item, selectorID, OmittedItemBudget, &candidate.material)
		return false
	}
	state.selected = trial
	return true
}

func (state *buildState) eligible(selector Selector) []selectedMaterial {
	result := make([]selectedMaterial, 0)
	for _, item := range state.items {
		if item.Role != selector.Role {
			continue
		}
		material, exists := state.materials[item.ID]
		if !exists {
			state.omit(item, selector.ID, OmittedMissingMaterial, nil)
			continue
		}
		if !authorityAllowed(state.input.Spec, material.Authority) {
			state.omit(item, selector.ID, OmittedAuthority, &material)
			continue
		}
		result = append(result, selectedMaterial{item: item, material: material})
	}
	return result
}

func (state *buildState) omit(
	item workingset.Item,
	selectorID string,
	reason OmissionReason,
	material *Material,
) {
	if _, selected := state.selectedItem(item.ID); selected {
		return
	}
	if _, exists := state.omissions[item.ID]; exists {
		return
	}
	omission := Omission{
		ItemID: item.ID, Ref: item.Ref, Role: item.Role,
		SelectorID: selectorID, Reason: reason, SourceFreshness: SourceFreshnessUnresolved,
	}
	if material != nil {
		omission.Authority = material.Authority
		omission.SourceFreshness = SourceFreshnessValidatedCurrent
	}
	state.omissions[item.ID] = omission
}

func (state *buildState) omitUnselectedRoles() {
	for _, item := range state.items {
		if _, selected := state.selectedItem(item.ID); selected {
			continue
		}
		if _, omitted := state.omissions[item.ID]; omitted {
			continue
		}
		omission := Omission{
			ItemID: item.ID, Ref: item.Ref, Role: item.Role,
			Reason: OmittedRoleNotSelected, SourceFreshness: SourceFreshnessUnresolved,
		}
		if material, exists := state.materials[item.ID]; exists {
			omission.Authority = material.Authority
			omission.SourceFreshness = SourceFreshnessValidatedCurrent
		}
		state.omissions[item.ID] = omission
	}
}

func (state *buildState) selectedItem(id workingset.ItemID) (selectedMaterial, bool) {
	for _, item := range state.selected {
		if item.item.ID == id {
			return item, true
		}
	}
	return selectedMaterial{}, false
}

func (state *buildState) sortSelected() { sortSelectedMaterials(state.selected) }

func sortSelectedMaterials(items []selectedMaterial) {
	sort.Slice(items, func(left, right int) bool {
		leftItem, rightItem := items[left].item, items[right].item
		if roleRank(leftItem.Role) != roleRank(rightItem.Role) {
			return roleRank(leftItem.Role) < roleRank(rightItem.Role)
		}
		if leftItem.Priority != rightItem.Priority {
			return leftItem.Priority > rightItem.Priority
		}
		if leftItem.LastUsedTick != rightItem.LastUsedTick {
			return leftItem.LastUsedTick > rightItem.LastUsedTick
		}
		return leftItem.ID < rightItem.ID
	})
}
