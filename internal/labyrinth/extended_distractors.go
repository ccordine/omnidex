package labyrinth

import (
	"fmt"
	"sort"
)

func (builder *extendedBuilder) materializeDistractors(target int) error {
	if len(builder.records) >= target {
		return nil
	}
	locations := make([]EntityID, 0)
	for _, entity := range builder.entities {
		if entity.Kind == stageKind && entity.Public {
			locations = append(locations, entity.ID)
		}
	}
	sort.Slice(locations, func(left, right int) bool { return locations[left] < locations[right] })
	if len(locations) == 0 {
		return fmt.Errorf("%w: extended world has no public location", ErrGeneration)
	}
	for index := len(builder.records); index < target; index++ {
		record := builder.entity(fmt.Sprintf("background-record-%03d", index), recordKind, true)
		mixed := mixSeed(builder.config.Seed ^ uint64(index+1)*0x9e3779b97f4a7c15)
		location := locations[int(mixed%uint64(len(locations)))]
		if err := builder.fact("record.at", record, location); err != nil {
			return err
		}
		content := fmt.Sprintf("Archived notation %016x.", mixSeed(mixed^0xd15ea5e))
		if _, err := builder.record(record, location, content); err != nil {
			return err
		}
	}
	return nil
}
