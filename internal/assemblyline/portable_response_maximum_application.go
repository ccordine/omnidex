package assemblyline

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
	case WorkApplicationClassify:
		return maximumStringBytes(
			ApplicationSurfaceBrowser, ApplicationSurfaceCommandLine,
			ApplicationSurfaceUnsupported,
		), true, nil
	default:
		return 0, false, nil
	}
}
