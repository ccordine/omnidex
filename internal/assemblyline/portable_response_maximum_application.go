package assemblyline

func portableApplicationResponseMaximum(job PortableJob) (portableResponseContract, bool, error) {
	switch job.Kind {
	case WorkApplicationConstructionContext:
		return portableResponseContract{maximum: maxConstructionContextBytes}, true, nil
	case WorkApplicationInputSource:
		maximum, err := opaqueModelChoiceBuilderResponseContract(applicationInputSourceChoices)
		return maximum, true, err
	case WorkApplicationResultValueKind:
		maximum, err := opaqueModelChoiceBuilderResponseContract(applicationResultValueKindChoices)
		return maximum, true, err
	case WorkApplicationProductContext:
		return portableResponseContract{maximum: maxApplicationProductBytes}, true, nil
	case WorkApplicationRequirementInventory:
		return portableResponseContract{maximum: max(maxApplicationRequirementInventoryBytes, len(ApplicationNoRuntimeRequirementCandidates))}, true, nil
	case WorkApplicationRequirementCandidateCardinality:
		maximum, err := opaqueModelChoiceBuilderResponseContract(
			applicationRequirementCandidateCardinalityOpaqueChoices,
		)
		return maximum, true, err
	case WorkApplicationRequirementCandidateKind:
		var input ApplicationRequirementCandidateContentPresenceInput
		if err := decodePortablePayload(job.Payload, &input); err != nil {
			return portableResponseContract{}, true, err
		}
		maximum, err := opaqueModelChoiceBuilderResponseContract(func() ([]OpaqueModelChoice, error) {
			return applicationRequirementCandidateContentPresenceOpaqueChoices(input.Dimension)
		})
		return maximum, true, err
	case WorkApplicationRequirementCandidateAuthorization:
		maximum, err := opaqueModelChoiceBuilderResponseContract(
			applicationRequirementCandidateAuthorizationOpaqueChoices,
		)
		return maximum, true, err
	case WorkApplicationRequirementCandidateOutcomeRelation:
		maximum, err := opaqueModelChoiceBuilderResponseContract(
			applicationRequirementCandidateOutcomeRelationOpaqueChoices,
		)
		return maximum, true, err
	case WorkApplicationRequirementCandidateResultRelation:
		var input ApplicationRequirementCandidateResultPresenceInput
		if err := decodePortablePayload(job.Payload, &input); err != nil {
			return portableResponseContract{}, true, err
		}
		present, absent, err := applicationRequirementCandidateResultPresenceDescriptions(input.Dimension)
		if err != nil {
			return portableResponseContract{}, true, err
		}
		maximum, err := opaqueModelChoiceBuilderResponseContract(func() ([]OpaqueModelChoice, error) {
			return applicationRequirementCandidateResultPresenceOpaqueChoices(present, absent)
		})
		return maximum, true, err
	case WorkApplicationRequirementCandidatePartition:
		var input ApplicationRequirementCandidatePartitionInput
		if err := decodePortablePayload(job.Payload, &input); err != nil {
			return portableResponseContract{}, true, err
		}
		if err := input.validate(); err != nil {
			return portableResponseContract{}, true, err
		}
		if input.Kind != nil {
			return portableResponseContract{maximum: 2*maxRequirementQuoteBytes + 1}, true, nil
		}
		return portableResponseContract{maximum: maxApplicationRequirementCandidatePartitionBytes}, true, nil
	case WorkApplicationProjectStackConstraint:
		var input ApplicationProjectStackConstraintInput
		if err := decodePortablePayload(job.Payload, &input); err != nil {
			return portableResponseContract{}, true, err
		}
		maximum, err := opaqueModelChoiceBuilderResponseContract(func() ([]OpaqueModelChoice, error) {
			return applicationProjectStackConstraintOpaqueChoices(input.Candidates)
		})
		return maximum, true, err
	case WorkApplicationClassify:
		maximum, err := opaqueModelChoiceBuilderResponseContract(
			applicationClassificationOpaqueChoices,
		)
		return maximum, true, err
	default:
		return portableResponseContract{}, false, nil
	}
}
