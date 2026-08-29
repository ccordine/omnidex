package assemblyline

import "strconv"

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
	case WorkApplicationRequirementCandidateCardinality:
		return maximumStringBytes(
			ApplicationRequirementOneRuntimeOutcome,
			ApplicationRequirementMultipleRuntimeOutcomes,
		), true, nil
	case WorkApplicationRequirementCandidateSplit:
		return maxRequirementQuoteBytes, true, nil
	case WorkApplicationRequirementCandidateSplitCorrection:
		return maxRequirementQuoteBytes, true, nil
	case WorkApplicationProjectStackConstraint:
		var input applicationProjectStackConstraintVersionedInput
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
	case WorkApplicationStateFieldPurpose, WorkApplicationRecordFieldPurpose:
		return MaxApplicationServiceStatePurposeBytes, true, nil
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
	case WorkApplicationRecordFieldCoverage:
		return maximumStringBytes(
			ApplicationRecordFieldRemains, ApplicationNoUncoveredRecordField,
		), true, nil
	case WorkApplicationRecordFieldKind:
		return maximumStringBytes(
			string(ApplicationServiceStateString),
			string(ApplicationServiceStateInteger),
			string(ApplicationServiceStateNumber),
			string(ApplicationServiceStateBoolean),
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
