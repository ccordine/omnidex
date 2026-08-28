package worker

import "github.com/gryph/omnidex/internal/assemblyline"

func replayDatabaseSemanticLeaf(
	job assemblyline.PortableJob,
	raw string,
) (bool, error) {
	switch job.Kind {
	case assemblyline.WorkDatabaseSchemaSelectionCoverage:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeDatabaseSchemaSelectionCoverageLeaf)
	case assemblyline.WorkDatabaseSchemaRelationSelection:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeDatabaseSchemaRelationSelectionLeaf)
	case assemblyline.WorkDatabaseQueryFromRelation:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeDatabaseQueryFromRelationLeaf)
	case assemblyline.WorkDatabaseQueryShape:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeDatabaseQueryShapeLeaf)
	case assemblyline.WorkDatabaseQueryProjectionCoverage:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeDatabaseQueryProjectionCoverageLeaf)
	case assemblyline.WorkDatabaseQueryProjectionAggregate:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeDatabaseQueryProjectionAggregateLeaf)
	case assemblyline.WorkDatabaseQueryProjectionField:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeDatabaseQueryProjectionFieldLeaf)
	case assemblyline.WorkDatabaseQueryProjectionTimeBucket:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeDatabaseQueryProjectionTimeBucketLeaf)
	case assemblyline.WorkDatabaseQueryFilterCoverage:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeDatabaseQueryFilterCoverageLeaf)
	case assemblyline.WorkDatabaseQueryFilterField:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeDatabaseQueryFilterFieldLeaf)
	case assemblyline.WorkDatabaseQueryFilterOperator:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeDatabaseQueryFilterOperatorLeaf)
	case assemblyline.WorkDatabaseQueryFilterValueCoverage:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeDatabaseQueryFilterValueCoverageLeaf)
	case assemblyline.WorkDatabaseQueryFilterValue:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeDatabaseQueryFilterValueLeaf)
	case assemblyline.WorkDatabaseQueryWindowCoverage:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeDatabaseQueryWindowCoverageLeaf)
	case assemblyline.WorkDatabaseQueryWindowField:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeDatabaseQueryWindowFieldLeaf)
	case assemblyline.WorkDatabaseQueryWindowUnit:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeDatabaseQueryWindowUnitLeaf)
	case assemblyline.WorkDatabaseQueryWindowAmount:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeDatabaseQueryWindowAmountLeaf)
	case assemblyline.WorkDatabaseQueryExistenceCoverage:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeDatabaseQueryExistenceCoverageLeaf)
	case assemblyline.WorkDatabaseQueryExistenceRelation:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeDatabaseQueryExistenceRelationLeaf)
	case assemblyline.WorkDatabaseQueryExistenceNegated:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeDatabaseQueryExistenceNegatedLeaf)
	case assemblyline.WorkDatabaseQueryHavingCoverage:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeDatabaseQueryHavingCoverageLeaf)
	case assemblyline.WorkDatabaseQueryHavingAggregate:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeDatabaseQueryHavingAggregateLeaf)
	case assemblyline.WorkDatabaseQueryHavingField:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeDatabaseQueryHavingFieldLeaf)
	case assemblyline.WorkDatabaseQueryHavingOperator:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeDatabaseQueryHavingOperatorLeaf)
	case assemblyline.WorkDatabaseQueryHavingValue:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeDatabaseQueryHavingValueLeaf)
	case assemblyline.WorkDatabaseQueryOrderCoverage:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeDatabaseQueryOrderCoverageLeaf)
	case assemblyline.WorkDatabaseQueryOrderProjection:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeDatabaseQueryOrderProjectionLeaf)
	case assemblyline.WorkDatabaseQueryOrderDirection:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeDatabaseQueryOrderDirectionLeaf)
	case assemblyline.WorkDatabaseEvidenceGap:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeDatabaseEvidenceGapDecision)
	case assemblyline.WorkDatabaseJoinPathSelection:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeDatabaseJoinPathSelectionDecision)
	default:
		return false, nil
	}
}
