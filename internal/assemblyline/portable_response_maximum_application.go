package assemblyline

import "strconv"

func portableApplicationResponseMaximum(job PortableJob) (int, bool, error) {
	switch job.Kind {
	case WorkApplicationContextQuestionInventory:
		return applicationContextQuestionInventoryMaximum(), true, nil
	case WorkApplicationContextQuestionNecessity:
		return maximumStringBytes(
			ApplicationContextQuestionNecessary,
			ApplicationContextQuestionNotNecessary,
		), true, nil
	case WorkApplicationContextQuestionRelation:
		return maximumStringBytes(
			ApplicationContextQuestionsSameFact,
			ApplicationContextQuestionsDistinctFact,
		), true, nil
	case WorkApplicationProductContext:
		return maxApplicationProductBytes, true, nil
	case WorkApplicationRequirementInventory:
		return max(maxApplicationRequirementInventoryBytes, len(ApplicationNoRuntimeRequirementCandidates)), true, nil
	case WorkApplicationRequirementCandidateCardinality:
		return maximumStringBytes(
			ApplicationRequirementOneRuntimeOutcome,
			ApplicationRequirementMultipleRuntimeOutcomes,
		), true, nil
	case WorkApplicationRequirementCandidateKind:
		return maximumStringBytes(
			string(ApplicationRequirementCandidateContentPresent),
			string(ApplicationRequirementCandidateContentAbsent),
		), true, nil
	case WorkApplicationRequirementCandidateAuthorization:
		return maximumStringBytes(
			ApplicationRequirementCandidateEntailed,
			ApplicationRequirementCandidateNotEntailed,
		), true, nil
	case WorkApplicationRequirementCandidateOutcomeRelation:
		return maximumStringBytes(
			ApplicationRequirementSameRuntimeOutcome,
			ApplicationRequirementDistinctRuntimeOutcomes,
		), true, nil
	case WorkApplicationRequirementCandidateResultRelation:
		return maximumStringBytes(
			string(ApplicationRequirementCandidateResultPresent),
			string(ApplicationRequirementCandidateResultAbsent),
		), true, nil
	case WorkApplicationRequirementCandidateResultRelationGrounding:
		return maximumStringBytes(
			ApplicationRequirementExactlyOneDeterminingRelationEntailed,
			ApplicationRequirementNoExactlyOneDeterminingRelationEntailed,
		), true, nil
	case WorkApplicationRequirementCandidateResultRelationCorrection:
		return maxRequirementQuoteBytes, true, nil
	case WorkApplicationRequirementCandidatePartition:
		var input ApplicationRequirementCandidatePartitionInput
		if err := decodePortablePayload(job.Payload, &input); err != nil {
			return 0, true, err
		}
		if err := input.validate(); err != nil {
			return 0, true, err
		}
		if input.Kind != nil {
			return 2*maxRequirementQuoteBytes + 1, true, nil
		}
		return maxApplicationRequirementCandidatePartitionBytes, true, nil
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
	case WorkApplicationStateFieldPurposeInventory,
		WorkApplicationRecordFieldPurposeInventory:
		return maxApplicationServiceStatePurposeInventoryBytes, true, nil
	case WorkApplicationStateFieldKind:
		return maximumStringBytes(
			string(ApplicationServiceStateString),
			string(ApplicationServiceStateInteger),
			string(ApplicationServiceStateNumber),
			string(ApplicationServiceStateBoolean),
			string(ApplicationServiceStateStringList),
			string(ApplicationServiceStateIntegerList),
			string(ApplicationServiceStateNumberList),
			string(ApplicationServiceStateBooleanList),
			string(ApplicationServiceStateRecordList),
		), true, nil
	case WorkApplicationRecordFieldKind:
		return maximumStringBytes(
			string(ApplicationServiceStateString),
			string(ApplicationServiceStateInteger),
			string(ApplicationServiceStateNumber),
			string(ApplicationServiceStateBoolean),
		), true, nil
	case WorkApplicationServiceStatePurposeNecessity:
		return maximumStringBytes(
			ApplicationServiceStatePurposeNecessary,
			ApplicationServiceStatePurposeNotNecessary,
		), true, nil
	case WorkApplicationServiceStatePurposeRelation:
		return maximumStringBytes(
			ApplicationServiceStateSamePurpose,
			ApplicationServiceStateDistinctPurposes,
		), true, nil
	case WorkApplicationServiceEndpointRequirement:
		return maximumStringBytes(
			ApplicationServiceEndpointRequired, ApplicationServiceSupportOnly,
		), true, nil
	case WorkApplicationServiceEndpointExposure:
		return maximumStringBytes(applicationServiceEndpointExposureValues()...), true, nil
	case WorkApplicationServiceEndpointMethod:
		return maximumStringBytes(applicationServiceEndpointMethodValues()...), true, nil
	case WorkApplicationServiceEndpointRouteTemplate:
		return maxApplicationServiceRouteBytes, true, nil
	case WorkApplicationServiceEndpointRequestMedia:
		var input ApplicationServiceEndpointRequestMediaInput
		if err := decodePortablePayload(job.Payload, &input); err != nil {
			return 0, true, err
		}
		candidates, err := ApplicationServiceEndpointRequestMediaCandidates(input)
		if err != nil {
			return 0, true, err
		}
		maximum := 0
		for _, candidate := range candidates {
			maximum = max(maximum, len(candidate))
		}
		return maximum, true, nil
	case WorkApplicationServiceEndpointResponseMedia:
		return maximumStringBytes(applicationServiceResponseMediaValues()...), true, nil
	case WorkApplicationServiceEndpointSuccessStatus:
		var input ApplicationServiceEndpointSuccessStatusInput
		if err := decodePortablePayload(job.Payload, &input); err != nil {
			return 0, true, err
		}
		candidates, err := ApplicationServiceEndpointSuccessStatusCandidates(input)
		if err != nil {
			return 0, true, err
		}
		maximum := 0
		for _, candidate := range candidates {
			maximum = max(maximum, len(strconv.Itoa(candidate)))
		}
		return maximum, true, nil
	case WorkApplicationClassify:
		return maximumStringBytes(
			ApplicationSurfaceBrowser, ApplicationSurfaceCommandLine,
			ApplicationSurfaceService, ApplicationSurfaceUnsupported,
		), true, nil
	case WorkApplicationTargetTree:
		maximum, err := targetTreeResponseMaximum(job)
		return maximum, true, err
	default:
		return 0, false, nil
	}
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
