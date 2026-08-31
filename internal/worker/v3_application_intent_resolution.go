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
	)
	if err != nil {
		return zero, err
	}

	queue := newDirectCodingApplicationRequirementCandidateQueue(inventory)
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
		currentCandidate := current.Candidate
		if err := assemblyline.ValidatePathFreeModelContextWithProvenance(
			"application requirement candidate",
			runtime.PathProvenance,
			currentCandidate,
		); err != nil {
			processedCandidates = append(processedCandidates, currentCandidate)
			continue
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
			continue
		case directCodingApplicationRequirementPartitioned:
			children := current.partitionChildren(resolved.Partition)
			if enqueuedCandidates+len(children) > assemblyline.MaxApplicationRequirementCandidateQueueNodes {
				// This candidate's proposed partition cannot fit the bounded queue.
				// Discard only that proposal; it has no authority to stop already
				// accepted work or any independent candidate still in the queue.
				continue
			}
			enqueuedCandidates += len(children)
			queue = append(children, queue...)
		case directCodingApplicationRequirementRetained:
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
			value, err := assemblyline.DecodeApplicationProductContextLeaf(productInput, raw)
			if err != nil {
				return "", err
			}
			if err := assemblyline.ValidatePathFreeModelContextWithProvenance(
				"application product context", runtime.PathProvenance, value,
			); err != nil {
				return "", err
			}
			return value, nil
		},
	)
	if err != nil {
		return zero, err
	}
	resolvedRequirements := make([]assemblyline.ApplicationRequirement, len(requirements))
	for index, requirement := range requirements {
		resolvedRequirements[index] = assemblyline.ApplicationRequirement{
			ID:            fmt.Sprintf("requirement_%03d", index+1),
			Statement:     requirement.Statement,
			RequestSHA256: authority.Context.RequestSHA256,
		}
	}
	return assemblyline.ApplicationIntentResolution{
		ProductContext: productContext,
		RequestSHA256:  authority.Context.RequestSHA256,
		Requirements:   resolvedRequirements,
	}, nil
}
