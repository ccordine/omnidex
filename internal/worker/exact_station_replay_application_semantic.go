package worker

import "github.com/gryph/omnidex/internal/assemblyline"

func replayApplicationSemanticLeaf(
	job assemblyline.PortableJob,
	raw string,
) (bool, error) {
	switch job.Kind {
	case assemblyline.WorkApplicationContextQuestionInventory:
		return true, decodeReplaySemanticLeaf(
			job, raw, assemblyline.DecodeApplicationContextQuestionInventory,
		)
	case assemblyline.WorkApplicationContextQuestionNecessity:
		return true, decodeReplaySemanticLeaf(
			job, raw, assemblyline.DecodeApplicationContextQuestionNecessityResult,
		)
	case assemblyline.WorkApplicationContextQuestionRelation:
		return true, decodeReplaySemanticLeaf(
			job, raw, assemblyline.DecodeApplicationContextQuestionRelationResult,
		)
	case assemblyline.WorkApplicationProductContext:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeApplicationProductContextLeaf)
	case assemblyline.WorkApplicationRequirementInventory:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeApplicationRequirementInventory)
	case assemblyline.WorkApplicationRequirementCandidateKind:
		return true, decodeReplaySemanticLeaf(
			job, raw, assemblyline.DecodeApplicationRequirementCandidateContentPresenceResult,
		)
	case assemblyline.WorkApplicationRequirementCandidateAuthorization:
		return true, decodeReplaySemanticLeaf(
			job, raw, assemblyline.DecodeApplicationRequirementCandidateAuthorizationResult,
		)
	case assemblyline.WorkApplicationRequirementCandidateCardinality:
		return true, decodeReplaySemanticLeaf(
			job, raw, assemblyline.DecodeApplicationRequirementCandidateCardinalityResult,
		)
	case assemblyline.WorkApplicationRequirementCandidateOutcomeRelation:
		return true, decodeReplaySemanticLeaf(
			job, raw, assemblyline.DecodeApplicationRequirementCandidateOutcomeRelationResult,
		)
	case assemblyline.WorkApplicationRequirementCandidateResultRelation:
		return true, decodeReplaySemanticLeaf(
			job, raw, assemblyline.DecodeApplicationRequirementCandidateResultPresenceResult,
		)
	case assemblyline.WorkApplicationRequirementCandidateResultRelationGrounding:
		return true, decodeReplaySemanticLeaf(
			job, raw, assemblyline.DecodeApplicationRequirementCandidateResultRelationGroundingResult,
		)
	case assemblyline.WorkApplicationRequirementCandidateResultRelationCorrection:
		return true, decodeReplaySemanticLeaf(
			job, raw, assemblyline.DecodeApplicationRequirementCandidateResultRelationCorrectionLeaf,
		)
	case assemblyline.WorkApplicationRequirementCandidatePartition:
		return true, decodeReplaySemanticLeaf(
			job, raw, assemblyline.DecodeApplicationRequirementCandidatePartition,
		)
	case assemblyline.WorkApplicationProjectStackConstraint:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeApplicationProjectStackConstraintDecision)
	case assemblyline.WorkApplicationServiceContinuedAvailability:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeApplicationServiceContinuedAvailabilityResult)
	case assemblyline.WorkApplicationServicePersistenceDestination:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeApplicationServicePersistenceDestinationResult)
	case assemblyline.WorkApplicationServiceStateLifetime:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeApplicationServiceStateLifetimeResult)
	case assemblyline.WorkApplicationStateFieldPurposeInventory:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeApplicationStateFieldPurposeInventory)
	case assemblyline.WorkApplicationStateFieldKind:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeApplicationStateFieldKindLeaf)
	case assemblyline.WorkApplicationRecordFieldPurposeInventory:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeApplicationRecordFieldPurposeInventory)
	case assemblyline.WorkApplicationRecordFieldKind:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeApplicationRecordFieldKindLeaf)
	case assemblyline.WorkApplicationServiceStatePurposeNecessity:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeApplicationServiceStatePurposeNecessityResult)
	case assemblyline.WorkApplicationServiceStatePurposeRelation:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeApplicationServiceStatePurposeRelationResult)
	case assemblyline.WorkApplicationServiceEndpointRequirement:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeApplicationServiceEndpointRequirementResult)
	case assemblyline.WorkApplicationServiceEndpointExposure:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeApplicationServiceEndpointExposureResult)
	case assemblyline.WorkApplicationServiceEndpointMethod:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeApplicationServiceEndpointMethodResult)
	case assemblyline.WorkApplicationServiceEndpointRouteTemplate:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeApplicationServiceEndpointRouteTemplateResult)
	case assemblyline.WorkApplicationServiceEndpointRequestMedia:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeApplicationServiceEndpointRequestMediaResult)
	case assemblyline.WorkApplicationServiceEndpointResponseMedia:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeApplicationServiceEndpointResponseMediaResult)
	case assemblyline.WorkApplicationServiceEndpointSuccessStatus:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeApplicationServiceEndpointSuccessStatusResult)
	case assemblyline.WorkApplicationClassify:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeApplicationClassification)
	default:
		return false, nil
	}
}
