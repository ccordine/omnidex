package assemblyline

func renderPortableApplicationJob(job PortableJob) (string, bool, error) {
	switch job.Kind {
	case WorkApplicationProductContext:
		return handledPortableRender(renderDecodedPortableInput(job, BuildApplicationProductContextPrompt))
	case WorkApplicationRequirementInventory:
		return handledPortableRender(renderDecodedPortableInput(job, BuildApplicationRequirementInventoryPrompt))
	case WorkApplicationRequirementCandidateCardinality:
		return handledPortableRender(renderDecodedPortableInput(
			job, BuildApplicationRequirementCandidateCardinalityPrompt,
		))
	case WorkApplicationRequirementCandidateKind:
		return handledPortableRender(renderDecodedPortableInput(
			job, BuildApplicationRequirementCandidateContentPresencePrompt,
		))
	case WorkApplicationRequirementCandidateAuthorization:
		return handledPortableRender(renderDecodedPortableInput(
			job, BuildApplicationRequirementCandidateAuthorizationPrompt,
		))
	case WorkApplicationRequirementCandidateOutcomeRelation:
		return handledPortableRender(renderDecodedPortableInput(
			job, BuildApplicationRequirementCandidateOutcomeRelationPrompt,
		))
	case WorkApplicationRequirementCandidatePartition:
		return handledPortableRender(renderDecodedPortableInput(
			job, BuildApplicationRequirementCandidatePartitionPrompt,
		))
	case WorkApplicationContextQuestionInventory:
		return handledPortableRender(renderDecodedPortableInput(
			job, BuildApplicationContextQuestionInventoryPrompt,
		))
	case WorkApplicationContextQuestionNecessity:
		return handledPortableRender(renderDecodedPortableInput(
			job, BuildApplicationContextQuestionNecessityPrompt,
		))
	case WorkApplicationContextQuestionRelation:
		return handledPortableRender(renderDecodedPortableInput(
			job, BuildApplicationContextQuestionRelationPrompt,
		))
	case WorkApplicationProjectStackConstraint:
		return handledPortableRender(renderDecodedPortableInput(job, BuildApplicationProjectStackConstraintPrompt))
	case WorkApplicationClassify:
		return handledPortableRender(renderDecodedPortableInput(job, BuildApplicationClassificationPrompt))
	default:
		return "", false, nil
	}
}
