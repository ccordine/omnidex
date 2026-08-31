package worker

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/gryph/omnidex/internal/assemblyline"
)

type directCodingApplicationIntentModels struct {
	Requirements   string
	ResultRelation string
}

func (models directCodingApplicationIntentModels) validate() error {
	if err := validateDirectCodingApplicationIntentModel("requirements", models.Requirements); err != nil {
		return err
	}
	return validateDirectCodingApplicationIntentModel("result-relation", models.ResultRelation)
}

func validateDirectCodingApplicationIntentModel(label, model string) error {
	if model == "" || model != strings.TrimSpace(model) || !utf8.ValidString(model) ||
		strings.ContainsRune(model, '\x00') {
		return fmt.Errorf("application intent %s model must be one exact canonical name", label)
	}
	return nil
}

func resolveDirectCodingApplicationIntent(
	runtime typedWorkerRuntime,
	models directCodingApplicationIntentModels,
	authority assemblyline.ApplicationIntentInput,
	identities []assemblyline.ArtifactIdentity,
) (assemblyline.ApplicationIntentResolution, error) {
	var zero assemblyline.ApplicationIntentResolution
	if err := models.validate(); err != nil {
		return zero, err
	}
	inventoryInput := assemblyline.ApplicationRequirementInventoryInput{
		UserRequest: authority.UserRequest,
		Context:     authority.Context,
	}
	inventoryJob, err := assemblyline.NewApplicationRequirementInventoryJob(inventoryInput)
	if err != nil {
		return zero, err
	}
	inventory, err := runDirectCodingSemanticLeafCall(
		runtime, models.Requirements, "application_requirement_inventory", inventoryJob, identities,
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
			models.Requirements,
			models.ResultRelation,
			inventoryInput,
			current,
			requirements,
			processedCandidates,
			identities,
		)
		if err != nil {
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
			if resolved.ResultRelation != (assemblyline.ApplicationRequirementCandidateResultRelationResult{}) ||
				!directCodingApplicationRequirementPartitionIsZero(resolved.Partition) {
				return zero, fmt.Errorf("discarded application requirement unexpectedly carries retained state")
			}
			continue
		case directCodingApplicationRequirementPartitioned:
			if resolved.ResultRelation != (assemblyline.ApplicationRequirementCandidateResultRelationResult{}) {
				return zero, fmt.Errorf("partitioned application requirement unexpectedly carries a result-relation receipt")
			}
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
			if !directCodingApplicationRequirementPartitionIsZero(resolved.Partition) {
				return zero, fmt.Errorf("retained application requirement unexpectedly carries a partition receipt")
			}
			if err := resolved.ResultRelation.ValidateAcceptedFor(resolved.Candidate); err != nil {
				return zero, fmt.Errorf("retained application requirement result relation: %w", err)
			}
			requirements = append(requirements, assemblyline.ApplicationIntentCandidateRequirement{
				Statement: resolved.Candidate, ResultRelation: resolved.ResultRelation,
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
		runtime, models.Requirements, "application_product_context", productJob, identities,
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
			ID:             fmt.Sprintf("requirement_%03d", index+1),
			Statement:      requirement.Statement,
			RequestSHA256:  authority.Context.RequestSHA256,
			ResultRelation: requirement.ResultRelation,
		}
	}
	return assemblyline.ApplicationIntentResolution{
		ProductContext: productContext,
		RequestSHA256:  authority.Context.RequestSHA256,
		Requirements:   resolvedRequirements,
	}, nil
}

func directCodingApplicationRequirementPartitionIsZero(
	partition assemblyline.ApplicationRequirementCandidatePartition,
) bool {
	return partition.Schema == "" &&
		partition.AuthoritySHA256 == "" &&
		partition.RawSHA256 == "" &&
		partition.Candidates == nil
}
