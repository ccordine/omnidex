package worker

import "github.com/gryph/omnidex/internal/assemblyline"

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
	PartitionInput assemblyline.ApplicationRequirementCandidatePartitionInput
	Partition      assemblyline.ApplicationRequirementCandidatePartition
}

func resolveDirectCodingApplicationRequirementCandidate(
	runtime typedWorkerRuntime,
	intentModel string,
	authority assemblyline.ApplicationRequirementInventoryInput,
	entry directCodingApplicationRequirementCandidateQueueEntry,
	acceptedRequirements []assemblyline.ApplicationIntentCandidateRequirement,
	processedCandidates []string,
	identities []assemblyline.ArtifactIdentity,
) (directCodingApplicationRequirementCandidateResolution, error) {
	var zero directCodingApplicationRequirementCandidateResolution
	candidate, _, err := entry.validateFor(authority)
	if err != nil {
		return zero, err
	}
	for {
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
				func(value assemblyline.ApplicationRequirementCandidateAuthorizationResult) error {
					return value.ValidateFor(authorizationInput)
				},
			)
			if err != nil {
				return zero, err
			}
		}
		if authorization.Relation == assemblyline.ApplicationRequirementCandidateNotEntailed {
			return directCodingApplicationRequirementCandidateResolution{
				Candidate:   candidate,
				Disposition: directCodingApplicationRequirementUnrequested,
			}, nil
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
			if len(entry.Lineage) >= assemblyline.MaxApplicationRequirementCandidatePartitionDepth {
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
				Candidate:      candidate,
				Disposition:    directCodingApplicationRequirementPartitioned,
				PartitionInput: partitionInput,
				Partition:      partition,
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
			if len(entry.Lineage) >= assemblyline.MaxApplicationRequirementCandidatePartitionDepth {
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
				Candidate:      candidate,
				Disposition:    directCodingApplicationRequirementPartitioned,
				PartitionInput: partitionInput,
				Partition:      partition,
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

		return directCodingApplicationRequirementCandidateResolution{
			Candidate: candidate, Disposition: directCodingApplicationRequirementRetained,
		}, nil
	}
}
