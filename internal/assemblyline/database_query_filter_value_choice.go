package assemblyline

import (
	"fmt"
	"strconv"

	"github.com/gryph/omnidex/internal/datasource"
)

const WorkDatabaseQueryFilterValueChoice WorkKind = "database_query_filter_value_choice"

const databaseFilterNoAdditionalValue = "code-owned:no-additional-filter-value"

// A nil value is the semantic absence of another applicable member. Code owns
// the retained set, removes each selection, and decides when selection ends.
type DatabaseQueryFilterValueChoice struct {
	Value *datasource.IntentLiteral
}

func (choice DatabaseQueryFilterValueChoice) ValidateFor(input DatabaseQueryFilterLeafInput) error {
	choices, err := databaseQueryFilterValueSubsetChoices(input)
	if err != nil {
		return err
	}
	if choice.Value == nil {
		for _, option := range choices {
			if option.value == databaseFilterNoAdditionalValue {
				return nil
			}
		}
		return fmt.Errorf("an initially sole filter value must be consumed")
	}
	literal, err := databaseQueryFilterValue(input, choice.Value.Value)
	if err != nil {
		return err
	}
	if literal != *choice.Value {
		return fmt.Errorf("database filter value choice differs from its field type")
	}
	return nil
}

func DatabaseQueryFilterHasClosedValues(input DatabaseQueryFilterLeafInput) (bool, error) {
	_, closed, err := databaseQueryAvailableFilterValues(input)
	return closed, err
}

func NewDatabaseQueryFilterValueChoiceJob(input DatabaseQueryFilterLeafInput) (PortableJob, error) {
	return newPortableJob(WorkDatabaseQueryFilterValueChoice, input)
}

func databaseQueryFilterValueSubsetChoices(input DatabaseQueryFilterLeafInput) ([]OpaqueModelChoice, error) {
	values, closed, err := databaseQueryAvailableFilterValues(input)
	if err != nil {
		return nil, err
	}
	if !closed || input.Operator != datasource.FilterIn && input.Operator != datasource.FilterNotIn {
		return nil, fmt.Errorf("remaining-value choice requires a closed set-membership field")
	}
	specs := make([]databaseOpaqueChoiceSpec, 0, len(values)+1)
	for index, value := range values {
		specs = append(specs, databaseOpaqueChoiceSpec{
			description: "The exact value " + strconv.Quote(value),
			value:       strconv.Itoa(index),
		})
	}
	// An initially sole candidate is determined by code. A sole remaining
	// candidate after earlier selections still competes with semantic absence.
	if len(values) != 1 || len(input.AcceptedValues) > 0 {
		specs = append(specs, databaseOpaqueChoiceSpec{
			description: "No additional remaining value belongs in the requested set",
			value:       databaseFilterNoAdditionalValue,
		})
	}
	return databaseOpaqueChoices(specs)
}

func BuildDatabaseQueryFilterValueChoicePrompt(input DatabaseQueryFilterLeafInput) (string, error) {
	choices, err := databaseQueryFilterValueSubsetChoices(input)
	if err != nil {
		return "", err
	}
	authority, err := renderDatabaseQueryFilterParameterAuthority(input)
	if err != nil {
		return "", err
	}
	return databaseQueryOpaqueChoicePrompt(
		"Which one of these remaining values, if any, belongs in the set required by the focused filter purpose?",
		authority, choices,
	)
}

func DecodeDatabaseQueryFilterValueChoice(input DatabaseQueryFilterLeafInput, raw string) (DatabaseQueryFilterValueChoice, error) {
	choices, err := databaseQueryFilterValueSubsetChoices(input)
	if err != nil {
		return DatabaseQueryFilterValueChoice{}, err
	}
	selected, err := DecodeOpaqueModelChoice(raw, choices)
	if err != nil {
		return DatabaseQueryFilterValueChoice{}, err
	}
	if selected == databaseFilterNoAdditionalValue {
		return DatabaseQueryFilterValueChoice{}, nil
	}
	values, _, err := databaseQueryAvailableFilterValues(input)
	if err != nil {
		return DatabaseQueryFilterValueChoice{}, err
	}
	index, err := strconv.Atoi(selected)
	if err != nil || index < 0 || index >= len(values) {
		return DatabaseQueryFilterValueChoice{}, fmt.Errorf("database filter choice has no remaining value")
	}
	literal, err := databaseQueryFilterValue(input, values[index])
	if err != nil {
		return DatabaseQueryFilterValueChoice{}, err
	}
	return DatabaseQueryFilterValueChoice{Value: &literal}, nil
}

func ResolveDatabaseQueryFilterValueChoice(input DatabaseQueryFilterLeafInput) (DatabaseQueryFilterValueChoice, bool, error) {
	choices, err := databaseQueryFilterValueSubsetChoices(input)
	if err != nil {
		return DatabaseQueryFilterValueChoice{}, false, err
	}
	_, resolved, err := ResolveSoleOpaqueModelChoice(choices)
	if err != nil || !resolved {
		return DatabaseQueryFilterValueChoice{}, false, err
	}
	value, err := DecodeDatabaseQueryFilterValueChoice(input, opaqueModelChoiceID(0))
	return value, err == nil, err
}
