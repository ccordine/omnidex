package assemblyline

func renderPortableApplicationJob(job PortableJob) (string, bool, error) {
	switch job.Kind {
	case WorkApplicationProductContext:
		return handledPortableRender(renderDecodedPortableInput(job, BuildApplicationProductContextPrompt))
	case WorkApplicationRequirementCoverage:
		return handledPortableRender(renderDecodedPortableInput(job, BuildApplicationRequirementCoveragePrompt))
	case WorkApplicationRequirement:
		return handledPortableRender(renderDecodedPortableInput(job, BuildApplicationRequirementPrompt))
	case WorkApplicationJobObjective:
		return handledPortableRender(renderDecodedPortableInput(job, BuildApplicationJobObjectivePrompt))
	case WorkApplicationBehaviorCoverage:
		return handledPortableRender(renderDecodedPortableInput(job, BuildApplicationBehaviorCoveragePrompt))
	case WorkApplicationBehavior:
		return handledPortableRender(renderDecodedPortableInput(job, BuildApplicationBehaviorPrompt))
	case WorkApplicationCriterionCoverage:
		return handledPortableRender(renderDecodedPortableInput(job, BuildApplicationCriterionCoveragePrompt))
	case WorkApplicationCriterion:
		return handledPortableRender(renderDecodedPortableInput(job, BuildApplicationCriterionPrompt))
	case WorkApplicationStateFieldCoverage:
		return handledPortableRender(renderDecodedPortableInput(job, BuildApplicationStateFieldCoveragePrompt))
	case WorkApplicationStateFieldName:
		return handledPortableRender(renderDecodedPortableInput(job, BuildApplicationStateFieldNamePrompt))
	case WorkApplicationStateFieldKind:
		return handledPortableRender(renderDecodedPortableInput(job, BuildApplicationStateFieldKindPrompt))
	case WorkApplicationRecordFieldCoverage:
		return handledPortableRender(renderDecodedPortableInput(job, BuildApplicationRecordFieldCoveragePrompt))
	case WorkApplicationRecordFieldName:
		return handledPortableRender(renderDecodedPortableInput(job, BuildApplicationRecordFieldNamePrompt))
	case WorkApplicationRecordFieldKind:
		return handledPortableRender(renderDecodedPortableInput(job, BuildApplicationRecordFieldKindPrompt))
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
