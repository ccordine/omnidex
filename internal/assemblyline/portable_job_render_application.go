package assemblyline

func renderPortableApplicationJob(job PortableJob) (string, bool, error) {
	switch job.Kind {
	case WorkApplicationProductContext:
		return handledPortableRender(renderDecodedPortableInput(job, BuildApplicationProductContextPrompt))
	case WorkApplicationRequirementCoverage:
		return handledPortableRender(renderDecodedPortableInput(job, BuildApplicationRequirementCoveragePrompt))
	case WorkApplicationRequirement:
		return handledPortableRender(renderDecodedPortableInput(job, BuildApplicationRequirementPrompt))
	case WorkApplicationRequirementCandidateCardinality:
		return handledPortableRender(renderDecodedPortableInput(
			job, BuildApplicationRequirementCandidateCardinalityPrompt,
		))
	case WorkApplicationRequirementCandidateKind:
		return handledPortableRender(renderDecodedPortableInput(
			job, BuildApplicationRequirementCandidateKindPrompt,
		))
	case WorkApplicationRequirementCandidateOutcomeRelation:
		return handledPortableRender(renderDecodedPortableInput(
			job, BuildApplicationRequirementCandidateOutcomeRelationPrompt,
		))
	case WorkApplicationRequirementCandidateResultRelation:
		return handledPortableRender(renderDecodedPortableInput(
			job, BuildApplicationRequirementCandidateResultRelationPrompt,
		))
	case WorkApplicationRequirementCandidateResultRelationGrounding:
		return handledPortableRender(renderDecodedPortableInput(
			job, BuildApplicationRequirementCandidateResultRelationGroundingPrompt,
		))
	case WorkApplicationRequirementCandidateResultRelationCorrection:
		return handledPortableRender(renderDecodedPortableInput(
			job, BuildApplicationRequirementCandidateResultRelationCorrectionPrompt,
		))
	case WorkApplicationRequirementCandidateSplit:
		return handledPortableRender(renderDecodedPortableInput(
			job, BuildApplicationRequirementCandidateSplitPrompt,
		))
	case WorkApplicationRequirementCandidateSplitCorrection:
		return handledPortableRender(renderDecodedPortableInput(
			job, BuildApplicationRequirementCandidateSplitCorrectionPrompt,
		))
	case WorkApplicationContextNeedCoverage:
		return handledPortableRender(renderDecodedPortableInput(job, BuildApplicationContextNeedCoveragePrompt))
	case WorkApplicationContextNeedQuestion:
		return handledPortableRender(renderDecodedPortableInput(job, BuildApplicationContextNeedQuestionPrompt))
	case WorkApplicationProjectStackConstraint:
		return handledPortableRender(renderDecodedPortableInput(job, BuildApplicationProjectStackConstraintPrompt))
	case WorkApplicationServiceContinuedAvailability:
		return handledPortableRender(renderDecodedPortableInput(job, BuildApplicationServiceContinuedAvailabilityPrompt))
	case WorkApplicationServicePersistenceDestination:
		return handledPortableRender(renderDecodedPortableInput(job, BuildApplicationServicePersistenceDestinationPrompt))
	case WorkApplicationServiceStateLifetime:
		return handledPortableRender(renderDecodedPortableInput(job, BuildApplicationServiceStateLifetimePrompt))
	case WorkApplicationStateFieldCoverage:
		return handledPortableRender(renderDecodedPortableInput(job, BuildApplicationStateFieldCoveragePrompt))
	case WorkApplicationStateFieldPurpose:
		return handledPortableRender(renderDecodedPortableInput(job, BuildApplicationStateFieldPurposePrompt))
	case WorkApplicationStateFieldKind:
		return handledPortableRender(renderDecodedPortableInput(job, BuildApplicationStateFieldKindPrompt))
	case WorkApplicationRecordFieldCoverage:
		return handledPortableRender(renderDecodedPortableInput(job, BuildApplicationRecordFieldCoveragePrompt))
	case WorkApplicationRecordFieldPurpose:
		return handledPortableRender(renderDecodedPortableInput(job, BuildApplicationRecordFieldPurposePrompt))
	case WorkApplicationRecordFieldKind:
		return handledPortableRender(renderDecodedPortableInput(job, BuildApplicationRecordFieldKindPrompt))
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
