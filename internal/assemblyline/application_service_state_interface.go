package assemblyline

import (
	"fmt"
	"regexp"
)

const ApplicationServiceStateInterfaceSchemaV1 = "omnidex.application-service-state-interface.v1"

const (
	maxApplicationServiceStateInterfaceNeeds  = 10
	maxApplicationServiceStateInterfaceFields = 8
)

type ApplicationServiceStateFieldKind string

const (
	ApplicationServiceStateString      ApplicationServiceStateFieldKind = "string"
	ApplicationServiceStateInteger     ApplicationServiceStateFieldKind = "integer"
	ApplicationServiceStateNumber      ApplicationServiceStateFieldKind = "number"
	ApplicationServiceStateBoolean     ApplicationServiceStateFieldKind = "boolean"
	ApplicationServiceStateStringList  ApplicationServiceStateFieldKind = "string_list"
	ApplicationServiceStateIntegerList ApplicationServiceStateFieldKind = "integer_list"
	ApplicationServiceStateNumberList  ApplicationServiceStateFieldKind = "number_list"
	ApplicationServiceStateBooleanList ApplicationServiceStateFieldKind = "boolean_list"
	ApplicationServiceStateRecordList  ApplicationServiceStateFieldKind = "record_list"
)

type ApplicationServiceStateInterfaceNeed struct {
	RequirementQuote   string   `json:"requirement_quote"`
	Objective          string   `json:"objective"`
	RequiredBehaviors  []string `json:"required_behaviors"`
	AcceptanceCriteria []string `json:"acceptance_criteria"`
}

type ApplicationServiceStateInterfaceInput struct {
	ProductContext string                                 `json:"product_context"`
	Needs          []ApplicationServiceStateInterfaceNeed `json:"needs"`
}

type ApplicationServiceStateRecordField struct {
	Name string                           `json:"name"`
	Kind ApplicationServiceStateFieldKind `json:"kind"`
}

type ApplicationServiceStateField struct {
	Name         string                               `json:"name"`
	Kind         ApplicationServiceStateFieldKind     `json:"kind"`
	RecordFields []ApplicationServiceStateRecordField `json:"record_fields"`
}

type ApplicationServiceStateInterfaceResult struct {
	Schema string                         `json:"schema"`
	Fields []ApplicationServiceStateField `json:"fields"`
}

var applicationServiceStateFieldName = regexp.MustCompile(`^[a-z][a-z0-9_]{0,47}$`)

func NewApplicationServiceStateInterfaceJob(
	input ApplicationServiceStateInterfaceInput,
) (PortableJob, error) {
	return newValidatedPortableJob(WorkApplicationServiceStateInterface, input, input.Validate)
}

func (input ApplicationServiceStateInterfaceInput) Validate() error {
	if err := validateApplicationProductQuote(
		"service state interface product", input.ProductContext,
	); err != nil {
		return err
	}
	if len(input.Needs) < 1 || len(input.Needs) > maxApplicationServiceStateInterfaceNeeds {
		return fmt.Errorf(
			"service state interface needs must contain between 1 and %d entries",
			maxApplicationServiceStateInterfaceNeeds,
		)
	}
	for index, need := range input.Needs {
		if err := validateApplicationIntentText(
			"service state interface requirement", need.RequirementQuote, maxRequirementQuoteBytes,
		); err != nil {
			return fmt.Errorf("service state interface need %d: %w", index+1, err)
		}
		if err := validateApplicationWorkloadLine(
			"service state interface objective", need.Objective, maxApplicationObjectiveRunes,
		); err != nil {
			return fmt.Errorf("service state interface need %d: %w", index+1, err)
		}
		if err := validateApplicationJobSpecificationList(
			"service state interface behavior", need.RequiredBehaviors,
			maxApplicationRequiredBehaviors, maxApplicationBehaviorRunes,
		); err != nil {
			return fmt.Errorf("service state interface need %d: %w", index+1, err)
		}
		if err := validateApplicationJobSpecificationList(
			"service state interface criterion", need.AcceptanceCriteria,
			maxApplicationAcceptanceCriteria, maxApplicationCriterionRunes,
		); err != nil {
			return fmt.Errorf("service state interface need %d: %w", index+1, err)
		}
	}
	return nil
}

func (result ApplicationServiceStateInterfaceResult) ValidateFor(
	input ApplicationServiceStateInterfaceInput,
) error {
	if err := input.Validate(); err != nil {
		return err
	}
	if result.Schema != ApplicationServiceStateInterfaceSchemaV1 {
		return fmt.Errorf(
			"service state interface schema must be %q",
			ApplicationServiceStateInterfaceSchemaV1,
		)
	}
	if len(result.Fields) < 1 || len(result.Fields) > maxApplicationServiceStateInterfaceFields {
		return fmt.Errorf(
			"service state interface fields must contain between 1 and %d entries",
			maxApplicationServiceStateInterfaceFields,
		)
	}
	seen := make(map[string]struct{}, len(result.Fields))
	for index, field := range result.Fields {
		if err := validateApplicationServiceStateFieldName(field.Name); err != nil {
			return fmt.Errorf("service state interface field %d: %w", index+1, err)
		}
		if _, duplicate := seen[field.Name]; duplicate {
			return fmt.Errorf("service state interface repeats field %q", field.Name)
		}
		seen[field.Name] = struct{}{}
		if err := validateApplicationServiceStateField(field); err != nil {
			return fmt.Errorf("service state interface field %q: %w", field.Name, err)
		}
	}
	return nil
}

func validateApplicationServiceStateField(field ApplicationServiceStateField) error {
	switch field.Kind {
	case ApplicationServiceStateString, ApplicationServiceStateInteger,
		ApplicationServiceStateNumber, ApplicationServiceStateBoolean,
		ApplicationServiceStateStringList, ApplicationServiceStateIntegerList,
		ApplicationServiceStateNumberList, ApplicationServiceStateBooleanList:
		if len(field.RecordFields) != 0 {
			return fmt.Errorf("non-record field has record fields")
		}
		return nil
	case ApplicationServiceStateRecordList:
		if len(field.RecordFields) < 1 || len(field.RecordFields) > maxApplicationServiceStateInterfaceFields {
			return fmt.Errorf(
				"record fields must contain between 1 and %d entries",
				maxApplicationServiceStateInterfaceFields,
			)
		}
		seen := make(map[string]struct{}, len(field.RecordFields))
		for _, recordField := range field.RecordFields {
			if err := validateApplicationServiceStateFieldName(recordField.Name); err != nil {
				return fmt.Errorf("record field: %w", err)
			}
			if _, duplicate := seen[recordField.Name]; duplicate {
				return fmt.Errorf("repeats record field %q", recordField.Name)
			}
			seen[recordField.Name] = struct{}{}
			switch recordField.Kind {
			case ApplicationServiceStateString, ApplicationServiceStateInteger,
				ApplicationServiceStateNumber, ApplicationServiceStateBoolean:
			default:
				return fmt.Errorf(
					"record field %q has unsupported scalar kind %q",
					recordField.Name, recordField.Kind,
				)
			}
		}
		return nil
	default:
		return fmt.Errorf("has unsupported kind %q", field.Kind)
	}
}

func validateApplicationServiceStateFieldName(name string) error {
	if !applicationServiceStateFieldName.MatchString(name) || len(name) > 48 {
		return fmt.Errorf("name %q must be one bounded lowercase snake-case identifier", name)
	}
	return nil
}
