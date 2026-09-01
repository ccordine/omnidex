package worker

import (
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/model"
)

type directCodingApplicationRequirementDisposition uint8

const (
	directCodingApplicationRequirementRetained directCodingApplicationRequirementDisposition = iota + 1
	directCodingApplicationRequirementExcluded
	directCodingApplicationRequirementUnrequested
	directCodingApplicationRequirementDuplicate
	directCodingApplicationRequirementPartitioned
	directCodingApplicationRequirementUnresolved
)

type directCodingApplicationRequirementCandidateResolution struct {
	Candidate      string
	Disposition    directCodingApplicationRequirementDisposition
	ResultRelation assemblyline.ApplicationRequirementCandidateResultRelationResult
	Partition      assemblyline.ApplicationRequirementCandidatePartition
	Annotation     model.CodingPlanAnnotation
}

func resolveDirectCodingApplicationRequirementCandidate(
	runtime typedWorkerRuntime,
	intentModel string,
	resultRelationModel string,
	authority assemblyline.ApplicationRequirementInventoryInput,
	entry directCodingApplicationRequirementCandidateQueueEntry,
	acceptedRequirements []assemblyline.ApplicationIntentCandidateRequirement,
	processedCandidates []string,
	identities []assemblyline.ArtifactIdentity,
) (directCodingApplicationRequirementCandidateResolution, error) {
	var zero directCodingApplicationRequirementCandidateResolution
	candidate := entry.Candidate
	exactDuplicate := directCodingApplicationRequirementExactDuplicate(
		candidate,
		acceptedRequirements,
		processedCandidates,
	)
	if exactDuplicate != directCodingApplicationRequirementNotExactDuplicate {
		return directCodingApplicationRequirementCandidateResolution{
			Candidate:   candidate,
			Disposition: directCodingApplicationRequirementDuplicate,
		}, nil
	}

	authorizationInput := assemblyline.ApplicationRequirementCandidateAuthorizationInput{
		UserRequest: authority.UserRequest,
		Context:     authority.Context,
		Candidate:   candidate,
	}
	authorization, authorizationResolved, err := assemblyline.ResolveExactSourceApplicationRequirementCandidateAuthorization(
		authorizationInput,
	)
	if err != nil {
		return zero, err
	}
	if !authorizationResolved {
		authorizationJob, err := assemblyline.NewApplicationRequirementCandidateAuthorizationJob(
			authorizationInput,
		)
		if err != nil {
			return zero, err
		}
		authorization, err = runDirectCodingSemanticLeafCall(
			runtime,
			intentModel,
			"application_requirement_candidate_authorization",
			authorizationJob,
			identities,
			func(raw string) (assemblyline.ApplicationRequirementCandidateAuthorizationResult, error) {
				return assemblyline.DecodeApplicationRequirementCandidateAuthorizationResult(
					authorizationInput,
					raw,
				)
			},
		)
		if err != nil {
			return zero, err
		}
	}
	annotation := model.CodingPlanAnnotationGrounded
	if authorization.Relation == assemblyline.ApplicationRequirementCandidateNotEntailed {
		scopeInput := assemblyline.ApplicationRequirementCandidateScopeRelationInput{
			UserRequest:   authority.UserRequest,
			Context:       authority.Context,
			Candidate:     candidate,
			Authorization: authorization,
			ScopeMode:     authority.ScopeMode,
		}
		scopeJob, err := assemblyline.NewApplicationRequirementCandidateScopeRelationJob(scopeInput)
		if err != nil {
			return zero, err
		}
		scope, err := runDirectCodingSemanticLeafCall(
			runtime,
			intentModel,
			"application_requirement_candidate_scope_relation",
			scopeJob,
			identities,
			func(raw string) (assemblyline.ApplicationRequirementCandidateScopeRelationResult, error) {
				return assemblyline.DecodeApplicationRequirementCandidateScopeRelationResult(scopeInput, raw)
			},
		)
		if err != nil {
			return zero, err
		}
		switch scope.Relation {
		case assemblyline.ApplicationRequirementCandidateScopeReasonableDerivation:
			annotation = model.CodingPlanAnnotationReasonableDerivation
		case assemblyline.ApplicationRequirementCandidateScopeSpeculativeReview:
			annotation = model.CodingPlanAnnotationSpeculativeReview
		case assemblyline.ApplicationRequirementCandidateScopeConcreteConflict:
			annotation = model.CodingPlanAnnotationConcreteConflict
		default:
			return zero, fmt.Errorf("application requirement scope relation %q is not registered", scope.Relation)
		}
	}

	kind, kindResolved, err := classifyDirectCodingApplicationRequirementCandidate(
		runtime,
		intentModel,
		candidate,
		identities,
	)
	if err != nil {
		return zero, err
	}
	if !kindResolved {
		return directCodingApplicationRequirementCandidateResolution{
			Candidate: candidate, Disposition: directCodingApplicationRequirementUnresolved,
		}, nil
	}
	if kind.Relation == assemblyline.ApplicationRequirementCandidateMixed {
		if len(entry.Ancestors) >= assemblyline.MaxApplicationRequirementCandidatePartitionDepth {
			return directCodingApplicationRequirementCandidateResolution{
				Candidate: candidate, Disposition: directCodingApplicationRequirementUnresolved,
			}, nil
		}
		partitionInput := assemblyline.ApplicationRequirementCandidatePartitionInput{
			Candidate: candidate,
			Kind:      &kind,
		}
		partition, err := partitionDirectCodingApplicationRequirementCandidate(
			runtime,
			intentModel,
			partitionInput,
			identities,
		)
		if err != nil {
			return zero, err
		}
		return directCodingApplicationRequirementCandidateResolution{
			Candidate: candidate, Disposition: directCodingApplicationRequirementPartitioned,
			Partition: partition,
		}, nil
	}
	if kind.Relation == assemblyline.ApplicationRequirementCandidateNonRuntime {
		return directCodingApplicationRequirementCandidateResolution{
			Candidate:   candidate,
			Disposition: directCodingApplicationRequirementExcluded,
		}, nil
	}

	cardinality, err := classifyDirectCodingApplicationRequirementCandidateCardinality(
		runtime,
		intentModel,
		candidate,
		identities,
	)
	if err != nil {
		return zero, err
	}
	if cardinality.Relation == assemblyline.ApplicationRequirementMultipleRuntimeOutcomes {
		if len(entry.Ancestors) >= assemblyline.MaxApplicationRequirementCandidatePartitionDepth {
			return directCodingApplicationRequirementCandidateResolution{
				Candidate: candidate, Disposition: directCodingApplicationRequirementUnresolved,
			}, nil
		}
		partitionInput := assemblyline.ApplicationRequirementCandidatePartitionInput{
			Candidate:   candidate,
			Cardinality: &cardinality,
		}
		partition, err := partitionDirectCodingApplicationRequirementCandidate(
			runtime,
			intentModel,
			partitionInput,
			identities,
		)
		if err != nil {
			return zero, err
		}
		return directCodingApplicationRequirementCandidateResolution{
			Candidate: candidate, Disposition: directCodingApplicationRequirementPartitioned,
			Partition: partition,
		}, nil
	}

	duplicate, err := directCodingApplicationRequirementSemanticDuplicate(
		runtime,
		intentModel,
		candidate,
		kind,
		cardinality,
		acceptedRequirements,
		identities,
	)
	if err != nil {
		return zero, err
	}
	if duplicate {
		return directCodingApplicationRequirementCandidateResolution{
			Candidate:   candidate,
			Disposition: directCodingApplicationRequirementDuplicate,
		}, nil
	}

	resultRelation, err := classifyDirectCodingApplicationRequirementCandidateResultRelation(
		runtime,
		resultRelationModel,
		candidate,
		kind,
		cardinality,
		identities,
	)
	if err != nil {
		return zero, err
	}
	if resultRelation.Relation == assemblyline.ApplicationRequirementMissingResultRelation {
		return directCodingApplicationRequirementCandidateResolution{
			Candidate: candidate, Disposition: directCodingApplicationRequirementUnresolved,
		}, nil
	}
	if err := resultRelation.ValidateAcceptedFor(candidate); err != nil {
		return zero, err
	}

	return directCodingApplicationRequirementCandidateResolution{
		Candidate: candidate, Disposition: directCodingApplicationRequirementRetained,
		ResultRelation: resultRelation, Annotation: annotation,
	}, nil
}
