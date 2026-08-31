package worker

import (
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func resolveDirectCodingApplicationIntent(
	runtime typedWorkerRuntime,
	intentModel string,
	authority assemblyline.ApplicationIntentInput,
	identities []assemblyline.ArtifactIdentity,
) (assemblyline.ApplicationIntentResolution, error) {
	var zero assemblyline.ApplicationIntentResolution
	inventoryInput := assemblyline.ApplicationRequirementInventoryInput{
		UserRequest: authority.UserRequest,
		Context:     authority.Context,
	}
	inventoryJob, err := assemblyline.NewApplicationRequirementInventoryJob(inventoryInput)
	if err != nil {
		return zero, err
	}
	inventory, err := runDirectCodingSemanticLeafCall(
		runtime, intentModel, "application_requirement_inventory", inventoryJob, identities,
		func(raw string) (assemblyline.ApplicationRequirementInventory, error) {
			return assemblyline.DecodeApplicationRequirementInventory(inventoryInput, raw)
		},
		func(value assemblyline.ApplicationRequirementInventory) error {
			if err := value.ValidateFor(inventoryInput); err != nil {
				return err
			}
			return assemblyline.ValidatePathFreeModelContextWithProvenance(
				"application requirement inventory",
				runtime.PathProvenance,
				value.Candidates...,
			)
		},
	)
	if err != nil {
		return zero, err
	}

	queue, err := newDirectCodingApplicationRequirementCandidateQueue(
		inventoryInput,
		inventory,
	)
	if err != nil {
		return zero, err
	}
	requirements := make(
		[]assemblyline.ApplicationIntentCandidateRequirement,
		0,
		assemblyline.MaxApplicationRequirementLeaves,
	)
	processedCandidates := make([]string, 0, len(inventory.Candidates))
	enqueuedCandidates := len(queue)
	// Once the accepted workload is full, no queued candidate can acquire
	// authority in this bounded iteration. Preserve the accepted leaves and
	// leave any further capability for a later explicit user objective without
	// spending inference on an unconsumable queue tail.
	for len(queue) > 0 && len(requirements) < assemblyline.MaxApplicationRequirementLeaves {
		current := queue[0]
		queue = queue[1:]
		currentCandidate, _, err := current.validateFor(inventoryInput)
		if err != nil {
			return zero, err
		}
		resolved, err := resolveDirectCodingApplicationRequirementCandidate(
			runtime,
			intentModel,
			inventoryInput,
			current,
			requirements,
			processedCandidates,
			identities,
		)
		if err != nil {
			if isDirectCodingSemanticLeafRejection(err) {
				processedCandidates = append(processedCandidates, currentCandidate)
				continue
			}
			return zero, err
		}
		if resolved.Candidate == "" {
			return zero, fmt.Errorf("application requirement candidate resolution is empty")
		}
		processedCandidates = append(processedCandidates, currentCandidate)
		if resolved.Candidate != currentCandidate {
			processedCandidates = append(processedCandidates, resolved.Candidate)
		}

		switch resolved.Disposition {
		case directCodingApplicationRequirementExcluded,
			directCodingApplicationRequirementUnrequested,
			directCodingApplicationRequirementDuplicate,
			directCodingApplicationRequirementUnresolved:
			if resolved.PartitionInput != (assemblyline.ApplicationRequirementCandidatePartitionInput{}) ||
				!directCodingApplicationRequirementPartitionIsZero(resolved.Partition) {
				return zero, fmt.Errorf(
					"discarded application requirement unexpectedly carries retained state",
				)
			}
		case directCodingApplicationRequirementPartitioned:
			if len(current.Lineage) == assemblyline.MaxApplicationRequirementCandidatePartitionDepth {
				return zero, fmt.Errorf(
					"application requirement candidate crossed the partition-depth preflight",
				)
			}
			children, err := current.partitionChildren(
				inventoryInput,
				resolved.PartitionInput,
				resolved.Partition,
			)
			if err != nil {
				return zero, err
			}
			if enqueuedCandidates+len(children) > assemblyline.MaxApplicationRequirementCandidateQueueNodes {
				// This candidate's proposed partition cannot fit the bounded queue.
				// Discard only that proposal; it has no authority to stop already
				// accepted work or any independent candidate still in the queue.
				continue
			}
			enqueuedCandidates += len(children)
			queue = append(children, queue...)
		case directCodingApplicationRequirementRetained:
			if resolved.PartitionInput != (assemblyline.ApplicationRequirementCandidatePartitionInput{}) ||
				!directCodingApplicationRequirementPartitionIsZero(resolved.Partition) {
				return zero, fmt.Errorf(
					"retained application requirement unexpectedly carries a partition receipt",
				)
			}
			requirements = append(requirements, assemblyline.ApplicationIntentCandidateRequirement{
				Statement: resolved.Candidate,
			})
		default:
			return zero, fmt.Errorf(
				"application requirement candidate has unregistered disposition %d",
				resolved.Disposition,
			)
		}
	}
	if len(requirements) == 0 {
		return assemblyline.ApplicationIntentResolution{
			RequestSHA256: authority.Context.RequestSHA256,
			Requirements:  []assemblyline.ApplicationRequirement{},
		}, nil
	}
	productInput := assemblyline.ApplicationProductContextInput{
		UserRequest: authority.UserRequest,
		Context:     authority.Context,
	}
	productJob, err := assemblyline.NewApplicationProductContextJob(productInput)
	if err != nil {
		return zero, err
	}
	productContext, err := runDirectCodingSemanticLeafCall(
		runtime, intentModel, "application_product_context", productJob, identities,
		func(raw string) (string, error) {
			return assemblyline.DecodeApplicationProductContextLeaf(productInput, raw)
		},
		func(value string) error {
			return assemblyline.ValidatePathFreeModelContextWithProvenance(
				"application product context", runtime.PathProvenance, value,
			)
		},
	)
	if err != nil {
		return zero, err
	}
	candidate := assemblyline.ApplicationIntentCandidate{
		Schema:         assemblyline.ApplicationIntentCandidateSchemaV1,
		ProductContext: productContext,
		Requirements: append(
			[]assemblyline.ApplicationIntentCandidateRequirement(nil),
			requirements...,
		),
	}
	return assemblyline.ResolveApplicationIntent(authority, candidate)
}

func directCodingApplicationRequirementPartitionIsZero(
	partition assemblyline.ApplicationRequirementCandidatePartition,
) bool {
	return partition.Schema == "" &&
		partition.AuthoritySHA256 == "" &&
		partition.RawSHA256 == "" &&
		partition.Candidates == nil
}
