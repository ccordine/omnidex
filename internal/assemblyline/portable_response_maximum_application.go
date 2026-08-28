package assemblyline

import (
	"fmt"
	"strconv"
)

func portableApplicationResponseMaximum(job PortableJob) (int, bool, error) {
	switch job.Kind {
	case WorkApplicationContextNeedCoverage:
		return maximumStringBytes(
			ApplicationContextNeedRemains, ApplicationNoUncoveredContextNeed,
		), true, nil
	case WorkApplicationContextNeedQuestion:
		return maxApplicationEvidenceQuestionBytes, true, nil
	case WorkApplicationProductContext:
		return maxApplicationProductBytes, true, nil
	case WorkApplicationRequirementCoverage:
		return maximumStringBytes(
			ApplicationRequirementRemains, ApplicationNoUncoveredRequirement,
		), true, nil
	case WorkApplicationRequirement:
		return maxRequirementQuoteBytes, true, nil
	case WorkApplicationProjectStackConstraint:
		var input ApplicationProjectStackConstraintInput
		if err := decodePortablePayload(job.Payload, &input); err != nil {
			return 0, true, err
		}
		maximum := maximumStringBytes(
			ApplicationProjectStackUnconstrained, ApplicationProjectStackUnsupported,
		)
		for _, candidate := range input.Candidates {
			maximum = max(maximum, len(candidate.CandidateID))
		}
		return maximum, true, nil
	case WorkApplicationServiceContinuedAvailability:
		return maximumStringBytes(
			ApplicationServiceAvailabilityNotRequiredCandidate,
			ApplicationServiceAvailabilityRequiredCandidate,
		), true, nil
	case WorkApplicationServicePersistenceDestination:
		return maximumStringBytes(
			ApplicationServiceBuildEnvironmentDestinationCandidate,
			ApplicationServiceBuildEnvironmentNotEstablishedCandidate,
		), true, nil
	case WorkApplicationServiceStateLifetime:
		return maximumStringBytes(
			ApplicationServiceStateRequestLocalOnly,
			ApplicationServiceStateCrossRequestAuthorityRequired,
		), true, nil
	case WorkApplicationStateFieldCoverage:
		return maximumStringBytes(
			ApplicationStateFieldRemains, ApplicationNoUncoveredStateField,
		), true, nil
	case WorkApplicationStateFieldName, WorkApplicationRecordFieldName:
		return MaxApplicationServiceStateFieldNameBytes, true, nil
	case WorkApplicationStateFieldKind:
		return maximumStringBytes(
			ApplicationServiceStateString, ApplicationServiceStateInteger,
			ApplicationServiceStateNumber, ApplicationServiceStateBoolean,
			ApplicationServiceStateStringList, ApplicationServiceStateIntegerList,
			ApplicationServiceStateNumberList, ApplicationServiceStateBooleanList,
			ApplicationServiceStateRecordList,
		), true, nil
	case WorkApplicationRecordFieldCoverage:
		return maximumStringBytes(
			ApplicationRecordFieldRemains, ApplicationNoUncoveredRecordField,
		), true, nil
	case WorkApplicationRecordFieldKind:
		return maximumStringBytes(
			ApplicationServiceStateString, ApplicationServiceStateInteger,
			ApplicationServiceStateNumber, ApplicationServiceStateBoolean,
		), true, nil
	case WorkApplicationServiceEndpointRequirement:
		return maximumStringBytes(
			ApplicationServiceEndpointRequired, ApplicationServiceSupportOnly,
		), true, nil
	case WorkApplicationServiceEndpointExposure:
		return maximumStringBytes(
			ApplicationServiceEndpointPublic,
			ApplicationServiceEndpointAuthenticated,
			ApplicationServiceEndpointInternal,
		), true, nil
	case WorkApplicationServiceEndpointMethod:
		return maximumStringBytes(
			ApplicationServiceEndpointGET, ApplicationServiceEndpointPOST,
			ApplicationServiceEndpointPUT, ApplicationServiceEndpointPATCH,
			ApplicationServiceEndpointDELETE,
		), true, nil
	case WorkApplicationServiceEndpointRouteTemplate:
		return maxApplicationServiceRouteBytes, true, nil
	case WorkApplicationServiceEndpointRequestMedia:
		maximum, err := applicationEndpointRequestMediaMaximum(job)
		return maximum, true, err
	case WorkApplicationServiceEndpointResponseMedia:
		values := applicationServiceResponseMediaValues()
		return maximumStringBytes(values...), true, nil
	case WorkApplicationServiceEndpointSuccessStatus:
		maximum, err := applicationEndpointStatusMaximum(job)
		return maximum, true, err
	case WorkApplicationClassify:
		return maximumStringBytes(
			ApplicationSurfaceBrowser, ApplicationSurfaceCommandLine,
			ApplicationSurfaceService, ApplicationSurfaceUnsupported,
		), true, nil
	case WorkApplicationJobObjective:
		return maxApplicationObjectiveRunes * 4, true, nil
	case WorkApplicationBehaviorCoverage:
		return maximumStringBytes(
			ApplicationBehaviorRemains, ApplicationNoUncoveredBehavior,
		), true, nil
	case WorkApplicationBehavior:
		return maxApplicationBehaviorRunes * 4, true, nil
	case WorkApplicationCriterionCoverage:
		return maximumStringBytes(
			ApplicationCriterionRemains, ApplicationNoUncoveredCriterion,
		), true, nil
	case WorkApplicationCriterion:
		return maxApplicationCriterionRunes * 4, true, nil
	case WorkApplicationTargetTree:
		maximum, err := targetTreeResponseMaximum(job)
		return maximum, true, err
	default:
		return 0, false, nil
	}
}

func applicationEndpointRequestMediaMaximum(job PortableJob) (int, error) {
	var input ApplicationServiceEndpointRequestMediaInput
	if err := decodePortablePayload(job.Payload, &input); err != nil {
		return 0, err
	}
	candidates, err := ApplicationServiceEndpointRequestMediaCandidates(input)
	if err != nil {
		return 0, err
	}
	return maximumStringBytes(candidates...), nil
}

func applicationEndpointStatusMaximum(job PortableJob) (int, error) {
	var input ApplicationServiceEndpointSuccessStatusInput
	if err := decodePortablePayload(job.Payload, &input); err != nil {
		return 0, err
	}
	candidates, err := ApplicationServiceEndpointSuccessStatusCandidates(input)
	if err != nil {
		return 0, err
	}
	maximum := 0
	for _, candidate := range candidates {
		maximum = max(maximum, len(strconv.Itoa(candidate)))
	}
	if maximum == 0 {
		return 0, fmt.Errorf("service endpoint success status has no accepted candidate")
	}
	return maximum, nil
}

func targetTreeResponseMaximum(job PortableJob) (int, error) {
	var input TargetTreeInput
	if err := decodePortablePayload(job.Payload, &input); err != nil {
		return 0, err
	}
	depth := MaxTargetTreeDepth
	if input.Constraints.RootFilesOnly {
		depth = 1
	}
	branchMaximum := targetTreeBranchMaximumBytes(depth)
	maximum := len(targetTreeRootLine) + 1 +
		input.Constraints.ExactPathCount*branchMaximum
	return cappedResponseMaximum(maximum, MaxPortableSemanticCandidateBytes), nil
}

func targetTreeBranchMaximumBytes(maximumDepth int) int {
	maximum := 0
	for depth := 1; depth <= maximumDepth; depth++ {
		// A depth-d path spends d-1 of the path budget on slash separators.
		nameBytes := MaxTargetTreePathBytes - (depth - 1)
		indentBytes := depth * (depth + 1)
		markerAndLFBytes := 3 * depth
		maximum = max(maximum, nameBytes+indentBytes+markerAndLFBytes)
	}
	return maximum
}
