package cognitiongauntlet

import (
	"fmt"

	"github.com/gryph/omnidex/internal/contextbuilder"
	"github.com/gryph/omnidex/internal/workingset"
)

func verifySemanticProjectionWorkingSet(
	projection contextbuilder.Projection,
	set *workingset.Set,
) error {
	if set == nil {
		return fmt.Errorf("semantic Context Projection lacks its Working Set")
	}
	resident := set.ResidentItems()
	items := make(map[workingset.ItemID]workingset.Item, len(resident))
	for _, item := range resident {
		items[item.ID] = item
	}
	seen := make(map[workingset.ItemID]struct{}, len(resident))
	accept := func(id workingset.ItemID, ref workingset.Ref, role workingset.Role) error {
		item, exists := items[id]
		if !exists || item.Ref != ref || item.Role != role {
			return fmt.Errorf(
				"semantic Context Projection item %q differs from the replayed Working Set",
				id,
			)
		}
		if _, duplicate := seen[id]; duplicate {
			return fmt.Errorf("semantic Context Projection item %q is duplicated", id)
		}
		seen[id] = struct{}{}
		return nil
	}
	for _, selected := range projection.Selected {
		if err := accept(selected.ItemID, selected.Ref, selected.Role); err != nil {
			return err
		}
	}
	for _, omitted := range projection.Omitted {
		if err := accept(omitted.ItemID, omitted.Ref, omitted.Role); err != nil {
			return err
		}
	}
	if len(seen) != len(resident) {
		return fmt.Errorf(
			"semantic Context Projection does not account for every resident Working Set item",
		)
	}
	return nil
}
