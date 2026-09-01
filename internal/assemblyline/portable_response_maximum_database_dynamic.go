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
	return opaqueModelChoiceBuilderResponseMaximum(func() ([]OpaqueModelChoice, error) {
		return databaseQueryFieldChoices(input.State, input.ScopeRelationID, nil)
	})
}

func databaseFilterOperatorMaximum(job PortableJob) (int, error) {
	var input DatabaseQueryFilterLeafInput
	if err := decodePortablePayload(job.Payload, &input); err != nil {
		return 0, err
	}
	return opaqueModelChoiceBuilderResponseMaximum(func() ([]OpaqueModelChoice, error) {
		return databaseQueryFilterOperatorChoices(input)
	})
}

func databaseFilterValueMaximum(job PortableJob) (int, error) {
	var input DatabaseQueryFilterLeafInput
	if err := decodePortablePayload(job.Payload, &input); err != nil {
		return 0, err
	}
	choices, closed, err := databaseQueryFilterValueChoices(input)
	if err != nil {
		return 0, err
	}
	if closed {
		return opaqueModelChoiceResponseMaximum(choices)
	}
	column, _, ok := databaseQueryColumn(input.State, input.FieldID)
	if !ok {
		return 0, fmt.Errorf("database query filter field %q was not projected", input.FieldID)
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
	return opaqueModelChoiceBuilderResponseMaximum(func() ([]OpaqueModelChoice, error) {
		return databaseQueryFieldChoices(input.State, "", databaseQueryTemporalFieldEligible)
	})
}

func databaseExistenceRelationMaximum(job PortableJob) (int, error) {
	var input DatabaseQueryExistenceLeafInput
	if err := decodePortablePayload(job.Payload, &input); err != nil {
		return 0, err
	}
	excluded := make(map[string]struct{}, len(input.State.Exists))
	for _, accepted := range input.State.Exists {
		excluded[accepted.RelationID] = struct{}{}
	}
	return opaqueModelChoiceBuilderResponseMaximum(func() ([]OpaqueModelChoice, error) {
		return databaseQueryRelationChoicesExcluding(input.State, excluded)
	})
}

func databaseHavingFieldMaximum(job PortableJob) (int, error) {
	var input DatabaseQueryHavingLeafInput
	if err := decodePortablePayload(job.Payload, &input); err != nil {
		return 0, err
	}
	return opaqueModelChoiceBuilderResponseMaximum(func() ([]OpaqueModelChoice, error) {
		return databaseQueryFieldChoices(
			input.State, "", databaseQueryAggregateFieldEligible(input.Aggregate),
		)
	})
}

func databaseOrderProjectionMaximum(job PortableJob) (int, error) {
	var input DatabaseQueryOrderLeafInput
	if err := decodePortablePayload(job.Payload, &input); err != nil {
		return 0, err
	}
	return opaqueModelChoiceBuilderResponseMaximum(func() ([]OpaqueModelChoice, error) {
		return databaseQueryOrderProjectionChoices(input)
	})
}

func databaseJoinPathMaximum(job PortableJob) (int, error) {
	var input DatabaseJoinPathSelectionInput
	if err := decodePortablePayload(job.Payload, &input); err != nil {
		return 0, err
	}
	return opaqueModelChoiceBuilderResponseMaximum(func() ([]OpaqueModelChoice, error) {
		return databaseJoinPathSelectionChoices(input)
	})
}
