package assemblyline

import (
	"fmt"
	"strconv"

	"github.com/gryph/omnidex/internal/datasource"
)

func portableDatabaseResponseMaximum(job PortableJob) (int, bool, error) {
	switch job.Kind {
	case WorkDatabaseSchemaSelectionCoverage:
		return maximumStringBytes(
			DatabaseSchemaRelationRemains, DatabaseSchemaNoUncoveredRelation,
		), true, nil
	case WorkDatabaseSchemaRelationSelection:
		maximum, err := databaseSchemaRelationMaximum(job)
		return maximum, true, err
	case WorkDatabaseQueryFromRelation:
		maximum, err := databaseFromRelationMaximum(job)
		return maximum, true, err
	case WorkDatabaseQueryShape:
		return maximumStringBytes(
			datasource.ResultRecords, datasource.ResultScalar,
			datasource.ResultRanking, datasource.ResultDistribution,
			datasource.ResultComparison, datasource.ResultTrend,
			datasource.ResultExistence,
		), true, nil
	case WorkDatabaseQueryProjectionCoverage:
		maximum, err := databaseCollectionCoverageMaximum(job, "projection")
		return maximum, true, err
	case WorkDatabaseQueryProjectionAggregate:
		return maximumStringBytes(
			datasource.AggregateCountRows, datasource.AggregateCount,
			datasource.AggregateCountDistinct, datasource.AggregateSum,
			datasource.AggregateAverage, datasource.AggregateMinimum,
			datasource.AggregateMaximum,
		), true, nil
	case WorkDatabaseQueryProjectionField:
		maximum, err := databaseProjectionFieldMaximum(job)
		return maximum, true, err
	case WorkDatabaseQueryProjectionTimeBucket:
		return maximumStringBytes(
			datasource.BucketDay, datasource.BucketWeek, datasource.BucketMonth,
			datasource.BucketQuarter, datasource.BucketYear,
		), true, nil
	case WorkDatabaseQueryFilterCoverage:
		return maximumStringBytes(
			DatabaseQueryItemRemains, DatabaseQueryNoUncoveredItem,
		), true, nil
	case WorkDatabaseQueryFilterField:
		maximum, err := databaseFilterFieldMaximum(job)
		return maximum, true, err
	case WorkDatabaseQueryFilterOperator:
		maximum, err := databaseFilterOperatorMaximum(job)
		return maximum, true, err
	case WorkDatabaseQueryFilterValueCoverage:
		maximum, err := databaseFilterValueCoverageMaximum(job)
		return maximum, true, err
	case WorkDatabaseQueryFilterValue:
		maximum, err := databaseFilterValueMaximum(job)
		return maximum, true, err
	case WorkDatabaseQueryWindowCoverage,
		WorkDatabaseQueryExistenceCoverage, WorkDatabaseQueryHavingCoverage:
		return maximumStringBytes(
			DatabaseQueryItemRemains, DatabaseQueryNoUncoveredItem,
		), true, nil
	case WorkDatabaseQueryWindowField:
		maximum, err := databaseWindowFieldMaximum(job)
		return maximum, true, err
	case WorkDatabaseQueryWindowUnit:
		return maximumStringBytes(
			datasource.WindowHour, datasource.WindowDay, datasource.WindowWeek,
			datasource.WindowMonth, datasource.WindowYear,
		), true, nil
	case WorkDatabaseQueryWindowAmount:
		return len(strconv.Itoa(10000)), true, nil
	case WorkDatabaseQueryExistenceRelation:
		maximum, err := databaseExistenceRelationMaximum(job)
		return maximum, true, err
	case WorkDatabaseQueryExistenceNegated:
		return maximumStringBytes("EXISTS", "NOT_EXISTS"), true, nil
	case WorkDatabaseQueryHavingAggregate:
		return maximumStringBytes(
			datasource.AggregateCountRows, datasource.AggregateCount,
			datasource.AggregateCountDistinct, datasource.AggregateSum,
			datasource.AggregateAverage,
		), true, nil
	case WorkDatabaseQueryHavingField:
		maximum, err := databaseHavingFieldMaximum(job)
		return maximum, true, err
	case WorkDatabaseQueryHavingOperator:
		return maximumStringBytes(
			datasource.FilterEqual, datasource.FilterNotEqual,
			datasource.FilterGT, datasource.FilterGTE,
			datasource.FilterLT, datasource.FilterLTE,
		), true, nil
	case WorkDatabaseQueryHavingValue:
		return datasource.MaxIntentDecimalLiteralBytes, true, nil
	case WorkDatabaseQueryOrderCoverage:
		maximum, err := databaseCollectionCoverageMaximum(job, "order")
		return maximum, true, err
	case WorkDatabaseQueryOrderProjection:
		maximum, err := databaseOrderProjectionMaximum(job)
		return maximum, true, err
	case WorkDatabaseQueryOrderDirection:
		return maximumStringBytes(
			datasource.OrderAscending, datasource.OrderDescending,
		), true, nil
	case WorkDatabaseEvidenceGap:
		return maxDatabaseEvidenceGapBytes, true, nil
	case WorkDatabaseJoinPathSelection:
		maximum, err := databaseJoinPathMaximum(job)
		return maximum, true, err
	default:
		return 0, false, nil
	}
}

func databaseSchemaRelationMaximum(job PortableJob) (int, error) {
	var input DatabaseSchemaSelectionLeafInput
	if err := decodePortablePayload(job.Payload, &input); err != nil {
		return 0, err
	}
	candidates := make([]string, 0, len(input.Authority.Candidates))
	for _, candidate := range input.Authority.Candidates {
		candidates = append(candidates, candidate.RelationID)
	}
	return maximumAcceptedCandidateBytes(
		"database schema relation selection", candidates,
		func(candidate string) error {
			_, err := DecodeDatabaseSchemaRelationSelectionLeaf(input, candidate)
			return err
		},
	)
}

func databaseFromRelationMaximum(job PortableJob) (int, error) {
	var state DatabaseQueryIntentLeafState
	if err := decodePortablePayload(job.Payload, &state); err != nil {
		return 0, err
	}
	candidates := databaseRelationCandidates(state)
	return maximumAcceptedCandidateBytes(
		"database query from relation", candidates,
		func(candidate string) error {
			_, err := DecodeDatabaseQueryFromRelationLeaf(state, candidate)
			return err
		},
	)
}

func databaseCollectionCoverageMaximum(job PortableJob, collection string) (int, error) {
	var state DatabaseQueryIntentLeafState
	if err := decodePortablePayload(job.Payload, &state); err != nil {
		return 0, err
	}
	return maximumAcceptedCandidateBytes(
		"database query "+collection+" coverage",
		[]string{DatabaseQueryItemRemains, DatabaseQueryNoUncoveredItem},
		func(candidate string) error {
			var err error
			switch collection {
			case "projection":
				_, err = DecodeDatabaseQueryProjectionCoverageLeaf(state, candidate)
			case "order":
				_, err = DecodeDatabaseQueryOrderCoverageLeaf(state, candidate)
			default:
				err = fmt.Errorf("database collection %q has no coverage decoder", collection)
			}
			return err
		},
	)
}

func databaseProjectionFieldMaximum(job PortableJob) (int, error) {
	var input DatabaseQueryProjectionLeafInput
	if err := decodePortablePayload(job.Payload, &input); err != nil {
		return 0, err
	}
	return maximumAcceptedCandidateBytes(
		"database query projection field", databaseColumnCandidates(input.State),
		func(candidate string) error {
			_, err := DecodeDatabaseQueryProjectionFieldLeaf(input, candidate)
			return err
		},
	)
}
