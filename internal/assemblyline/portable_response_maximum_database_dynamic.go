package assemblyline

import (
	"fmt"
	"math"
	"strconv"

	"github.com/gryph/omnidex/internal/datasource"
)

func databaseFilterFieldMaximum(job PortableJob) (int, error) {
	var input DatabaseQueryFilterLeafInput
	if err := decodePortablePayload(job.Payload, &input); err != nil {
		return 0, err
	}
	return maximumAcceptedCandidateBytes(
		"database query filter field", databaseColumnCandidates(input.State),
		func(candidate string) error {
			_, err := DecodeDatabaseQueryFilterFieldLeaf(input, candidate)
			return err
		},
	)
}

func databaseFilterOperatorMaximum(job PortableJob) (int, error) {
	var input DatabaseQueryFilterLeafInput
	if err := decodePortablePayload(job.Payload, &input); err != nil {
		return 0, err
	}
	operators := databaseQueryFilterOperators(input.State, input.FieldID)
	if len(operators) == 0 {
		return 0, fmt.Errorf("database query filter has no accepted operator")
	}
	return maximumStringBytes(operators...), nil
}

func databaseFilterValueCoverageMaximum(job PortableJob) (int, error) {
	var input DatabaseQueryFilterLeafInput
	if err := decodePortablePayload(job.Payload, &input); err != nil {
		return 0, err
	}
	return maximumAcceptedCandidateBytes(
		"database query filter value coverage",
		[]string{DatabaseQueryValueRemains, DatabaseQueryNoUncoveredValue},
		func(candidate string) error {
			_, err := DecodeDatabaseQueryFilterValueCoverageLeaf(input, candidate)
			return err
		},
	)
}

func databaseFilterValueMaximum(job PortableJob) (int, error) {
	var input DatabaseQueryFilterLeafInput
	if err := decodePortablePayload(job.Payload, &input); err != nil {
		return 0, err
	}
	column, _, ok := databaseQueryColumn(input.State, input.FieldID)
	if !ok {
		return 0, fmt.Errorf("database query filter field %q was not projected", input.FieldID)
	}
	if len(column.AllowedValues) > 0 {
		return maximumAcceptedCandidateBytes(
			"database query filter value", column.AllowedValues,
			func(candidate string) error {
				_, err := DecodeDatabaseQueryFilterValueLeaf(input, candidate)
				return err
			},
		)
	}
	if column.TypeCategory == datasource.TypeBoolean {
		return maximumAcceptedCandidateBytes(
			"database query boolean filter value", []string{"true", "false"},
			func(candidate string) error {
				_, err := DecodeDatabaseQueryFilterValueLeaf(input, candidate)
				return err
			},
		)
	}
	maximum := databaseLiteralTypeMaximum(column.TypeCategory)
	if maximum == 0 {
		return 0, fmt.Errorf(
			"database query field type %q has no accepted literal", column.TypeCategory,
		)
	}
	return cappedResponseMaximum(maximum, maxDatabaseQueryLeafBytes), nil
}

func databaseLiteralTypeMaximum(category datasource.ColumnTypeCategory) int {
	switch category {
	case datasource.TypeText, datasource.TypeTemporal:
		return maxDatabaseQueryLeafBytes
	case datasource.TypeInteger:
		return len(strconv.FormatInt(math.MinInt64, 10))
	case datasource.TypeDecimal:
		return datasource.MaxIntentDecimalLiteralBytes
	case datasource.TypeBoolean:
		return maximumStringBytes("true", "false")
	case datasource.TypeDate:
		return len("2006-01-02")
	case datasource.TypeUUID:
		return len("00000000-0000-0000-0000-000000000000")
	default:
		return 0
	}
}

func databaseWindowFieldMaximum(job PortableJob) (int, error) {
	var input DatabaseQueryWindowLeafInput
	if err := decodePortablePayload(job.Payload, &input); err != nil {
		return 0, err
	}
	return maximumAcceptedCandidateBytes(
		"database query window field", databaseColumnCandidates(input.State),
		func(candidate string) error {
			_, err := DecodeDatabaseQueryWindowFieldLeaf(input, candidate)
			return err
		},
	)
}

func databaseExistenceRelationMaximum(job PortableJob) (int, error) {
	var input DatabaseQueryExistenceLeafInput
	if err := decodePortablePayload(job.Payload, &input); err != nil {
		return 0, err
	}
	return maximumAcceptedCandidateBytes(
		"database query existence relation", databaseRelationCandidates(input.State),
		func(candidate string) error {
			_, err := DecodeDatabaseQueryExistenceRelationLeaf(input, candidate)
			return err
		},
	)
}

func databaseHavingFieldMaximum(job PortableJob) (int, error) {
	var input DatabaseQueryHavingLeafInput
	if err := decodePortablePayload(job.Payload, &input); err != nil {
		return 0, err
	}
	return maximumAcceptedCandidateBytes(
		"database query having field", databaseColumnCandidates(input.State),
		func(candidate string) error {
			_, err := DecodeDatabaseQueryHavingFieldLeaf(input, candidate)
			return err
		},
	)
}

func databaseOrderProjectionMaximum(job PortableJob) (int, error) {
	var input DatabaseQueryOrderLeafInput
	if err := decodePortablePayload(job.Payload, &input); err != nil {
		return 0, err
	}
	candidates := make([]string, len(input.State.Projections))
	for index := range input.State.Projections {
		candidates[index] = strconv.Itoa(index)
	}
	return maximumAcceptedCandidateBytes(
		"database query order projection", candidates,
		func(candidate string) error {
			_, err := DecodeDatabaseQueryOrderProjectionLeaf(input, candidate)
			return err
		},
	)
}

func databaseJoinPathMaximum(job PortableJob) (int, error) {
	var input DatabaseJoinPathSelectionInput
	if err := decodePortablePayload(job.Payload, &input); err != nil {
		return 0, err
	}
	candidates := make([]string, len(input.Candidates))
	for index, candidate := range input.Candidates {
		candidates[index] = candidate.PathID
	}
	return maximumAcceptedCandidateBytes(
		"database join path selection", candidates,
		func(candidate string) error {
			_, err := DecodeDatabaseJoinPathSelectionDecision(input, candidate)
			return err
		},
	)
}

func databaseRelationCandidates(state DatabaseQueryIntentLeafState) []string {
	candidates := make([]string, len(state.Authority.SchemaProjection.Relations))
	for index, relation := range state.Authority.SchemaProjection.Relations {
		candidates[index] = relation.ID
	}
	return candidates
}

func databaseColumnCandidates(state DatabaseQueryIntentLeafState) []string {
	var candidates []string
	for _, relation := range state.Authority.SchemaProjection.Relations {
		for _, column := range relation.Columns {
			candidates = append(candidates, column.ID)
		}
	}
	return candidates
}
