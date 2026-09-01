package assemblyline

func portableApplicationResponseMaximum(job PortableJob) (int, bool, error) {
	switch job.Kind {
	case WorkApplicationProductContext:
		return maxApplicationProductBytes, true, nil
	case WorkApplicationRequirementInventory:
		return max(maxApplicationRequirementInventoryBytes, len(ApplicationNoRuntimeRequirementCandidates)), true, nil
	case WorkApplicationRequirementCandidateCardinality:
		maximum, err := opaqueModelChoiceBuilderResponseMaximum(
			applicationRequirementCandidateCardinalityOpaqueChoices,
		)
		return maximum, true, err
	case WorkApplicationRequirementCandidateKind:
		var input ApplicationRequirementCandidateContentPresenceInput
		if err := decodePortablePayload(job.Payload, &input); err != nil {
			return 0, true, err
		}
		maximum, err := opaqueModelChoiceBuilderResponseMaximum(func() ([]OpaqueModelChoice, error) {
			return applicationRequirementCandidateContentPresenceOpaqueChoices(input.Dimension)
		})
		return maximum, true, err
	case WorkApplicationRequirementCandidateAuthorization:
		maximum, err := opaqueModelChoiceBuilderResponseMaximum(
			applicationRequirementCandidateAuthorizationOpaqueChoices,
		)
		return maximum, true, err
	case WorkApplicationRequirementCandidateScopeRelation:
		maximum, err := opaqueModelChoiceBuilderResponseMaximum(
			applicationRequirementCandidateScopeRelationOpaqueChoices,
		)
		return maximum, true, err
	case WorkApplicationRequirementCandidateOutcomeRelation:
		maximum, err := opaqueModelChoiceBuilderResponseMaximum(
			applicationRequirementCandidateOutcomeRelationOpaqueChoices,
		)
		return maximum, true, err
	case WorkApplicationRequirementCandidateResultRelation:
		var input ApplicationRequirementCandidateResultPresenceInput
		if err := decodePortablePayload(job.Payload, &input); err != nil {
			return 0, true, err
		}
		present, absent, err := applicationRequirementCandidateResultPresenceDescriptions(input.Dimension)
		if err != nil {
			return 0, true, err
		}
		maximum, err := opaqueModelChoiceBuilderResponseMaximum(func() ([]OpaqueModelChoice, error) {
			return applicationRequirementCandidateResultPresenceOpaqueChoices(present, absent)
		})
		return maximum, true, err
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
		maximum, err := opaqueModelChoiceBuilderResponseMaximum(func() ([]OpaqueModelChoice, error) {
			return applicationProjectStackConstraintOpaqueChoices(input.Candidates)
		})
		return maximum, true, err
	case WorkApplicationClassify:
		maximum, err := opaqueModelChoiceBuilderResponseMaximum(
			applicationClassificationOpaqueChoices,
		)
		return maximum, true, err
	default:
		return 0, false, nil
	}
}
