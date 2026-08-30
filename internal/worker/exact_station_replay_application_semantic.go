package worker

import "github.com/gryph/omnidex/internal/assemblyline"

func replayApplicationSemanticLeaf(
	job assemblyline.PortableJob,
	raw string,
) (bool, error) {
	switch job.Kind {
	case assemblyline.WorkApplicationContextNeedCoverage:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeApplicationContextNeedCoverageLeaf)
	case assemblyline.WorkApplicationContextNeedQuestion:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeApplicationContextNeedQuestionLeaf)
	case assemblyline.WorkApplicationProductContext:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeApplicationProductContextLeaf)
	case assemblyline.WorkApplicationRequirementCoverage:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeApplicationRequirementCoverageLeaf)
	case assemblyline.WorkApplicationRequirement:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeApplicationRequirementLeaf)
	case assemblyline.WorkApplicationRequirementCandidateKind:
		return true, decodeReplaySemanticLeaf(
			job, raw, assemblyline.DecodeApplicationRequirementCandidateKindResult,
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
			job, raw, assemblyline.DecodeApplicationRequirementCandidateResultRelationResult,
		)
	case assemblyline.WorkApplicationRequirementCandidateResultRelationGrounding:
		return true, decodeReplaySemanticLeaf(
			job, raw, assemblyline.DecodeApplicationRequirementCandidateResultRelationGroundingResult,
		)
	case assemblyline.WorkApplicationRequirementCandidateResultRelationCorrection:
		return true, decodeReplaySemanticLeaf(
			job, raw, assemblyline.DecodeApplicationRequirementCandidateResultRelationCorrectionLeaf,
		)
	case assemblyline.WorkApplicationRequirementCandidateSplit:
		return true, decodeReplaySemanticLeaf(
			job, raw, assemblyline.DecodeApplicationRequirementCandidateSplitLeaf,
		)
	case assemblyline.WorkApplicationRequirementCandidateSplitCorrection:
		return true, decodeReplaySemanticLeaf(
			job, raw, assemblyline.DecodeApplicationRequirementCandidateSplitCorrectionLeaf,
		)
	case assemblyline.WorkApplicationProjectStackConstraint:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeApplicationProjectStackConstraintDecision)
	case assemblyline.WorkApplicationServiceContinuedAvailability:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeApplicationServiceContinuedAvailabilityResult)
	case assemblyline.WorkApplicationServicePersistenceDestination:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeApplicationServicePersistenceDestinationResult)
	case assemblyline.WorkApplicationServiceStateLifetime:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeApplicationServiceStateLifetimeResult)
	case assemblyline.WorkApplicationStateFieldCoverage:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeApplicationStateFieldCoverageLeaf)
	case assemblyline.WorkApplicationStateFieldPurpose:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeApplicationStateFieldPurposeLeaf)
	case assemblyline.WorkApplicationStateFieldKind:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeApplicationStateFieldKindLeaf)
	case assemblyline.WorkApplicationRecordFieldCoverage:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeApplicationRecordFieldCoverageLeaf)
	case assemblyline.WorkApplicationRecordFieldPurpose:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeApplicationRecordFieldPurposeLeaf)
	case assemblyline.WorkApplicationRecordFieldKind:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeApplicationRecordFieldKindLeaf)
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
