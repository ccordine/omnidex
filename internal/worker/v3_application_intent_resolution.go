package worker

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/model"
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

func resolveApprovedDirectCodingApplicationIntent(
	runtime typedWorkerRuntime,
	models directCodingApplicationIntentModels,
	authority assemblyline.ApplicationIntentInput,
	approved []assemblyline.ApplicationRequirement,
	identities []assemblyline.ArtifactIdentity,
) (assemblyline.ApplicationIntentResolution, error) {
	var zero assemblyline.ApplicationIntentResolution
	if err := models.validate(); err != nil {
		return zero, err
	}
	if len(approved) == 0 {
		return assemblyline.ApplicationIntentResolution{
			RequestSHA256: authority.Context.RequestSHA256,
			Requirements:  []assemblyline.ApplicationRequirement{},
		}, nil
	}
	if len(approved) > assemblyline.MaxApplicationRequirementLeaves {
		return zero, fmt.Errorf(
			"approved coding plan exceeds the %d-leaf execution limit",
			assemblyline.MaxApplicationRequirementLeaves,
		)
	}
	resolvedRequirements := make([]assemblyline.ApplicationRequirement, len(approved))
	for index, requirement := range approved {
		if requirement.ID != fmt.Sprintf("requirement_%03d", index+1) {
			return zero, fmt.Errorf("approved coding plan requirement %d has noncanonical identity", index)
		}
		if err := requirement.ResultRelation.ValidateAcceptedFor(requirement.Statement); err != nil {
			return zero, fmt.Errorf("approved coding plan requirement %s: %w", requirement.ID, err)
		}
		resolvedRequirements[index] = requirement
		resolvedRequirements[index].RequestSHA256 = authority.Context.RequestSHA256
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
	return assemblyline.ApplicationIntentResolution{
		ProductContext: productContext,
		RequestSHA256:  authority.Context.RequestSHA256,
		Requirements:   resolvedRequirements,
	}, nil
}

type directCodingApplicationRequirementProposal struct {
	Statement      string
	Annotation     model.CodingPlanAnnotation
	ResultRelation assemblyline.ApplicationRequirementCandidateResultRelationResult
}

func resolveDirectCodingApplicationPlan(
	runtime typedWorkerRuntime,
	models directCodingApplicationIntentModels,
	authority assemblyline.ApplicationIntentInput,
	scopeMode model.CodingScopeMode,
	identities []assemblyline.ArtifactIdentity,
) ([]directCodingApplicationRequirementProposal, error) {
	if err := models.validate(); err != nil {
		return nil, err
	}
	if err := scopeMode.Validate(); err != nil {
		return nil, err
	}
	inventoryInput := assemblyline.ApplicationRequirementInventoryInput{
		UserRequest: authority.UserRequest, Context: authority.Context, ScopeMode: scopeMode,
	}
	inventoryJob, err := assemblyline.NewApplicationRequirementInventoryJob(inventoryInput)
	if err != nil {
		return nil, err
	}
	inventory, err := runDirectCodingSemanticLeafCall(
		runtime, models.Requirements, "application_requirement_inventory", inventoryJob, identities,
		func(raw string) (assemblyline.ApplicationRequirementInventory, error) {
			return assemblyline.DecodeApplicationRequirementInventory(inventoryInput, raw)
		},
	)
	if err != nil {
		return nil, err
	}
	candidateQueue := newDirectCodingApplicationRequirementCandidateQueue(inventory)
	accepted := make([]assemblyline.ApplicationIntentCandidateRequirement, 0, assemblyline.MaxApplicationRequirementLeaves)
	proposals := make([]directCodingApplicationRequirementProposal, 0, len(inventory.Candidates))
	processedCandidates := make([]string, 0, len(inventory.Candidates))
	enqueuedCandidates := len(candidateQueue)
	for len(candidateQueue) > 0 && len(accepted) < assemblyline.MaxApplicationRequirementLeaves {
		current := candidateQueue[0]
		candidateQueue = candidateQueue[1:]
		currentCandidate := current.Candidate
		if err := assemblyline.ValidatePathFreeModelContextWithProvenance(
			"application requirement candidate", runtime.PathProvenance, currentCandidate,
		); err != nil {
			processedCandidates = append(processedCandidates, currentCandidate)
			continue
		}
		resolved, err := resolveDirectCodingApplicationRequirementCandidate(
			runtime, models.Requirements, models.ResultRelation, inventoryInput, current,
			accepted, processedCandidates, identities,
		)
		if err != nil {
			return nil, err
		}
		if resolved.Candidate == "" {
			return nil, fmt.Errorf("application requirement candidate resolution is empty")
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
				return nil, fmt.Errorf("discarded application requirement unexpectedly carries retained state")
			}
		case directCodingApplicationRequirementPartitioned:
			if resolved.ResultRelation != (assemblyline.ApplicationRequirementCandidateResultRelationResult{}) {
				return nil, fmt.Errorf("partitioned application requirement unexpectedly carries a result-relation receipt")
			}
			children := current.partitionChildren(resolved.Partition)
			if enqueuedCandidates+len(children) > assemblyline.MaxApplicationRequirementCandidateQueueNodes {
				continue
			}
			enqueuedCandidates += len(children)
			candidateQueue = append(children, candidateQueue...)
		case directCodingApplicationRequirementRetained:
			if !directCodingApplicationRequirementPartitionIsZero(resolved.Partition) {
				return nil, fmt.Errorf("retained application requirement unexpectedly carries a partition receipt")
			}
			if err := resolved.Annotation.Validate(); err != nil {
				return nil, fmt.Errorf("retained application requirement annotation is invalid: %v", err)
			}
			if err := resolved.ResultRelation.ValidateAcceptedFor(resolved.Candidate); err != nil {
				return nil, fmt.Errorf("retained application requirement result relation: %w", err)
			}
			if len(proposals) < model.MaxCodingPlanLeaves {
				proposals = append(proposals, directCodingApplicationRequirementProposal{
					Statement: resolved.Candidate, Annotation: resolved.Annotation,
					ResultRelation: resolved.ResultRelation,
				})
				accepted = append(accepted, assemblyline.ApplicationIntentCandidateRequirement{
					Statement: resolved.Candidate, ResultRelation: resolved.ResultRelation,
				})
			}
		default:
			return nil, fmt.Errorf("application requirement candidate has unregistered disposition %d", resolved.Disposition)
		}
	}
	return proposals, nil
}

func directCodingApplicationRequirementPartitionIsZero(
	partition assemblyline.ApplicationRequirementCandidatePartition,
) bool {
	return partition.Schema == "" &&
		partition.AuthoritySHA256 == "" &&
		partition.RawSHA256 == "" &&
		partition.Candidates == nil
}
