package worker

import "github.com/gryph/omnidex/internal/assemblyline"

// directCodingApplicationRequirementCandidateQueueEntry is code-owned queue
// state. Model receipts are decoded and validated at their call boundary; the
// queue does not create a second validation authority for them.
type directCodingApplicationRequirementCandidateQueueEntry struct {
	Candidate string
	Ancestors []string
}

func newDirectCodingApplicationRequirementCandidateQueue(
	inventory assemblyline.ApplicationRequirementInventory,
) []directCodingApplicationRequirementCandidateQueueEntry {
	queue := make([]directCodingApplicationRequirementCandidateQueueEntry, len(inventory.Candidates))
	for index, candidate := range inventory.Candidates {
		queue[index] = directCodingApplicationRequirementCandidateQueueEntry{Candidate: candidate}
	}
	return queue
}

func (entry directCodingApplicationRequirementCandidateQueueEntry) partitionChildren(
	partition assemblyline.ApplicationRequirementCandidatePartition,
) []directCodingApplicationRequirementCandidateQueueEntry {
	if len(entry.Ancestors) >= assemblyline.MaxApplicationRequirementCandidatePartitionDepth {
		return nil
	}
	ancestors := append([]string(nil), entry.Ancestors...)
	ancestors = append(ancestors, entry.Candidate)
	seen := make(map[string]struct{}, len(ancestors)+len(partition.Candidates))
	for _, ancestor := range ancestors {
		seen[ancestor] = struct{}{}
	}
	children := make([]directCodingApplicationRequirementCandidateQueueEntry, 0, len(partition.Candidates))
	for _, candidate := range partition.Candidates {
		if _, duplicate := seen[candidate]; duplicate {
			continue
		}
		seen[candidate] = struct{}{}
		children = append(children, directCodingApplicationRequirementCandidateQueueEntry{
			Candidate: candidate,
			Ancestors: append([]string(nil), ancestors...),
		})
	}
	return children
}
