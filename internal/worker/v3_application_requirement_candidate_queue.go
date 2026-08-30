package worker

import (
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
)

type directCodingApplicationRequirementCandidateLineageStep struct {
	Input      assemblyline.ApplicationRequirementCandidatePartitionInput
	Partition  assemblyline.ApplicationRequirementCandidatePartition
	ChildIndex int
}

type directCodingApplicationRequirementCandidateQueueEntry struct {
	Inventory assemblyline.ApplicationRequirementInventory
	RootIndex int
	Lineage   []directCodingApplicationRequirementCandidateLineageStep
}

func newDirectCodingApplicationRequirementCandidateQueue(
	input assemblyline.ApplicationRequirementInventoryInput,
	inventory assemblyline.ApplicationRequirementInventory,
) ([]directCodingApplicationRequirementCandidateQueueEntry, error) {
	if err := inventory.ValidateFor(input); err != nil {
		return nil, err
	}
	owned := inventory
	owned.Candidates = append([]string(nil), inventory.Candidates...)
	queue := make([]directCodingApplicationRequirementCandidateQueueEntry, len(owned.Candidates))
	for index := range owned.Candidates {
		queue[index] = directCodingApplicationRequirementCandidateQueueEntry{
			Inventory: owned,
			RootIndex: index,
		}
	}
	return queue, nil
}

func (entry directCodingApplicationRequirementCandidateQueueEntry) validateFor(
	input assemblyline.ApplicationRequirementInventoryInput,
) (string, []string, error) {
	if err := entry.Inventory.ValidateFor(input); err != nil {
		return "", nil, fmt.Errorf("validate application requirement queue inventory: %w", err)
	}
	if entry.RootIndex < 0 || entry.RootIndex >= len(entry.Inventory.Candidates) {
		return "", nil, fmt.Errorf("application requirement queue root index is outside inventory")
	}
	current := entry.Inventory.Candidates[entry.RootIndex]
	ancestry := make([]string, 0, len(entry.Lineage))
	for index, step := range entry.Lineage {
		if step.Input.Candidate != current {
			return "", nil, fmt.Errorf(
				"application requirement queue lineage step %d does not bind its parent",
				index,
			)
		}
		if err := step.Partition.ValidateFor(step.Input); err != nil {
			return "", nil, fmt.Errorf(
				"validate application requirement queue lineage step %d: %w",
				index,
				err,
			)
		}
		if step.ChildIndex < 0 || step.ChildIndex >= len(step.Partition.Candidates) {
			return "", nil, fmt.Errorf(
				"application requirement queue lineage child index %d is outside partition",
				index,
			)
		}
		ancestry = append(ancestry, current)
		current = step.Partition.Candidates[step.ChildIndex]
		for _, ancestor := range ancestry {
			if current == ancestor {
				return "", nil, fmt.Errorf(
					"application requirement candidate partition creates an ancestry cycle",
				)
			}
		}
	}
	return current, ancestry, nil
}

func (entry directCodingApplicationRequirementCandidateQueueEntry) partitionChildren(
	input assemblyline.ApplicationRequirementInventoryInput,
	partitionInput assemblyline.ApplicationRequirementCandidatePartitionInput,
	partition assemblyline.ApplicationRequirementCandidatePartition,
) ([]directCodingApplicationRequirementCandidateQueueEntry, error) {
	current, _, err := entry.validateFor(input)
	if err != nil {
		return nil, err
	}
	if partitionInput.Candidate != current {
		return nil, fmt.Errorf("application requirement partition does not bind its queue parent")
	}
	if err := partition.ValidateFor(partitionInput); err != nil {
		return nil, err
	}
	if len(entry.Lineage) == assemblyline.MaxApplicationRequirementCandidatePartitionDepth {
		return nil, fmt.Errorf(
			"application requirement candidate remains compound at the code-owned partition depth %d",
			assemblyline.MaxApplicationRequirementCandidatePartitionDepth,
		)
	}
	children := make([]directCodingApplicationRequirementCandidateQueueEntry, len(partition.Candidates))
	for index := range partition.Candidates {
		ownedPartition := partition
		ownedPartition.Candidates = append([]string(nil), partition.Candidates...)
		lineage := append(
			[]directCodingApplicationRequirementCandidateLineageStep(nil),
			entry.Lineage...,
		)
		lineage = append(lineage, directCodingApplicationRequirementCandidateLineageStep{
			Input:      partitionInput,
			Partition:  ownedPartition,
			ChildIndex: index,
		})
		children[index] = directCodingApplicationRequirementCandidateQueueEntry{
			Inventory: entry.Inventory,
			RootIndex: entry.RootIndex,
			Lineage:   lineage,
		}
		if _, _, err := children[index].validateFor(input); err != nil {
			return nil, err
		}
	}
	return children, nil
}
