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
	case WorkApplicationRequirementCandidateResultRelation:
		return handledPortableRender(renderDecodedPortableInput(
			job, BuildApplicationRequirementCandidateResultPresencePrompt,
		))
	case WorkApplicationRequirementCandidateResultRelationGrounding:
		return handledPortableRender(renderDecodedPortableInput(
			job, BuildApplicationRequirementCandidateResultRelationGroundingPrompt,
		))
	case WorkApplicationRequirementCandidateResultRelationCorrection:
		return handledPortableRender(renderDecodedPortableInput(
			job, BuildApplicationRequirementCandidateResultRelationCorrectionPrompt,
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
	case WorkApplicationServiceContinuedAvailability:
		return handledPortableRender(renderDecodedPortableInput(job, BuildApplicationServiceContinuedAvailabilityPrompt))
	case WorkApplicationServicePersistenceDestination:
		return handledPortableRender(renderDecodedPortableInput(job, BuildApplicationServicePersistenceDestinationPrompt))
	case WorkApplicationServiceStateLifetime:
		return handledPortableRender(renderDecodedPortableInput(job, BuildApplicationServiceStateLifetimePrompt))
	case WorkApplicationStateFieldPurposeInventory:
		return handledPortableRender(renderDecodedPortableInput(job, BuildApplicationStateFieldPurposeInventoryPrompt))
	case WorkApplicationStateFieldKind:
		return handledPortableRender(renderDecodedPortableInput(job, BuildApplicationStateFieldKindPrompt))
	case WorkApplicationRecordFieldPurposeInventory:
		return handledPortableRender(renderDecodedPortableInput(job, BuildApplicationRecordFieldPurposeInventoryPrompt))
	case WorkApplicationRecordFieldKind:
		return handledPortableRender(renderDecodedPortableInput(job, BuildApplicationRecordFieldKindPrompt))
	case WorkApplicationServiceStatePurposeNecessity:
		return handledPortableRender(renderDecodedPortableInput(job, BuildApplicationServiceStatePurposeNecessityPrompt))
	case WorkApplicationServiceStatePurposeRelation:
		return handledPortableRender(renderDecodedPortableInput(job, BuildApplicationServiceStatePurposeRelationPrompt))
	case WorkApplicationServiceEndpointRequirement:
		return handledPortableRender(renderDecodedPortableInput(job, BuildApplicationServiceEndpointRequirementPrompt))
	case WorkApplicationServiceEndpointExposure:
		return handledPortableRender(renderDecodedPortableInput(job, BuildApplicationServiceEndpointExposurePrompt))
	case WorkApplicationServiceEndpointMethod:
		return handledPortableRender(renderDecodedPortableInput(job, BuildApplicationServiceEndpointMethodPrompt))
	case WorkApplicationServiceEndpointRouteTemplate:
		return handledPortableRender(renderDecodedPortableInput(job, BuildApplicationServiceEndpointRouteTemplatePrompt))
	case WorkApplicationServiceEndpointRequestMedia:
		return handledPortableRender(renderDecodedPortableInput(job, BuildApplicationServiceEndpointRequestMediaPrompt))
	case WorkApplicationServiceEndpointResponseMedia:
		return handledPortableRender(renderDecodedPortableInput(job, BuildApplicationServiceEndpointResponseMediaPrompt))
	case WorkApplicationServiceEndpointSuccessStatus:
		return handledPortableRender(renderDecodedPortableInput(job, BuildApplicationServiceEndpointSuccessStatusPrompt))
	case WorkApplicationClassify:
		return handledPortableRender(renderDecodedPortableInput(job, BuildApplicationClassificationPrompt))
	case WorkApplicationTargetTree:
		return handledPortableRender(renderDecodedPortableInput(job, BuildTargetTreePrompt))
	default:
		return "", false, nil
	}
}
