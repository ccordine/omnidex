package assemblyline

import (
	"fmt"
	"regexp"
	"strings"
)

const ApplicationServiceStateInterfaceSchemaV2 = "omnidex.application-service-state-interface.v2"

const (
	maxApplicationServiceStateInterfaceNeeds  = 10
	maxApplicationServiceStateInterfaceFields = 8
	MaxApplicationServiceStateFieldNameBytes  = 48
	MaxApplicationServiceStatePurposeBytes    = 256
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
	RequirementQuote string `json:"requirement_quote"`
}

type ApplicationServiceStateInterfaceInput struct {
	ProductContext string                                 `json:"product_context"`
	Needs          []ApplicationServiceStateInterfaceNeed `json:"needs"`
}

func ProjectApplicationServiceStateInterfaceNeed(
	authority ApplicationTaskRuntimeAuthority,
) (ApplicationServiceStateInterfaceNeed, error) {
	need := ApplicationServiceStateInterfaceNeed{
		RequirementQuote: authority.RequirementQuote,
	}
	if err := need.validate(); err != nil {
		return ApplicationServiceStateInterfaceNeed{}, err
	}
	return need, nil
}

type ApplicationServiceStateRecordField struct {
	Name    string                           `json:"name"`
	Purpose string                           `json:"purpose"`
	Kind    ApplicationServiceStateFieldKind `json:"kind"`
}

type ApplicationServiceStateField struct {
	Name         string                               `json:"name"`
	Purpose      string                               `json:"purpose"`
	Kind         ApplicationServiceStateFieldKind     `json:"kind"`
	RecordFields []ApplicationServiceStateRecordField `json:"record_fields"`
}

type ApplicationServiceStateInterfaceResult struct {
	Schema string                         `json:"schema"`
	Fields []ApplicationServiceStateField `json:"fields"`
}

var applicationServiceStateFieldName = regexp.MustCompile(fmt.Sprintf(
	`^[a-z][a-z0-9_]{0,%d}$`, MaxApplicationServiceStateFieldNameBytes-1,
))

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
		if err := need.validate(); err != nil {
			return fmt.Errorf("service state interface need %d: %w", index+1, err)
		}
	}
	return nil
}

func (need ApplicationServiceStateInterfaceNeed) validate() error {
	if err := validateApplicationIntentText(
		"service state interface requirement", need.RequirementQuote, maxRequirementQuoteBytes,
	); err != nil {
		return err
	}
	return nil
}

func (result ApplicationServiceStateInterfaceResult) ValidateFor(
	input ApplicationServiceStateInterfaceInput,
) error {
	if err := input.Validate(); err != nil {
		return err
	}
	if result.Schema != ApplicationServiceStateInterfaceSchemaV2 {
		return fmt.Errorf(
			"service state interface schema must be %q",
			ApplicationServiceStateInterfaceSchemaV2,
		)
	}
	if len(result.Fields) < 1 || len(result.Fields) > maxApplicationServiceStateInterfaceFields {
		return fmt.Errorf(
			"service state interface fields must contain between 1 and %d entries",
			maxApplicationServiceStateInterfaceFields,
		)
	}
	seenPurposes := make(map[string]struct{}, len(result.Fields))
	for index, field := range result.Fields {
		expectedName, err := CodeOwnedApplicationServiceStateFieldName(index + 1)
		if err != nil {
			return err
		}
		if field.Name != expectedName {
			return fmt.Errorf(
				"service state interface field %d name %q differs from code-owned name %q",
				index+1, field.Name, expectedName,
			)
		}
		if err := validateApplicationServiceStatePurpose("root field", field.Purpose); err != nil {
			return fmt.Errorf("service state interface field %q: %w", field.Name, err)
		}
		purposeKey := strings.ToLower(field.Purpose)
		if _, duplicate := seenPurposes[purposeKey]; duplicate {
			return fmt.Errorf("service state interface repeats root purpose %q", field.Purpose)
		}
		seenPurposes[purposeKey] = struct{}{}
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
		seenPurposes := make(map[string]struct{}, len(field.RecordFields))
		for index, recordField := range field.RecordFields {
			expectedName, err := CodeOwnedApplicationServiceRecordFieldName(index + 1)
			if err != nil {
				return err
			}
			if recordField.Name != expectedName {
				return fmt.Errorf(
					"record field %d name %q differs from code-owned name %q",
					index+1, recordField.Name, expectedName,
				)
			}
			if err := validateApplicationServiceStatePurpose(
				"record field", recordField.Purpose,
			); err != nil {
				return fmt.Errorf("record field %q: %w", recordField.Name, err)
			}
			purposeKey := strings.ToLower(recordField.Purpose)
			if _, duplicate := seenPurposes[purposeKey]; duplicate {
				return fmt.Errorf("repeats record purpose %q", recordField.Purpose)
			}
			seenPurposes[purposeKey] = struct{}{}
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

func validateApplicationServiceStatePurpose(label, purpose string) error {
	if err := validateContextText(
		"application service state "+label+" purpose",
		purpose,
		MaxApplicationServiceStatePurposeBytes,
	); err != nil {
		return err
	}
	if strings.ContainsAny(purpose, "\r\n") {
		return fmt.Errorf("application service state %s purpose must be one line", label)
	}
	return ValidatePathFreeModelContext(
		"application service state "+label+" purpose",
		purpose,
	)
}

func equalApplicationServicePurpose(left, right string) bool {
	return strings.EqualFold(left, right)
}

func CodeOwnedApplicationServiceStateFieldName(index int) (string, error) {
	if index < 1 || index > maxApplicationServiceStateInterfaceFields {
		return "", fmt.Errorf(
			"application service state field index must be between 1 and %d",
			maxApplicationServiceStateInterfaceFields,
		)
	}
	return fmt.Sprintf("state_%03d", index), nil
}

func CodeOwnedApplicationServiceRecordFieldName(index int) (string, error) {
	if index < 1 || index > maxApplicationServiceStateInterfaceFields {
		return "", fmt.Errorf(
			"application service record field index must be between 1 and %d",
			maxApplicationServiceStateInterfaceFields,
		)
	}
	return fmt.Sprintf("member_%03d", index), nil
}
