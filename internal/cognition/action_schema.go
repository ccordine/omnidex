package cognition

import (
	"encoding/json"
	"fmt"
	"sort"
)

func NewActionSchema(
	id ActionSchemaID,
	version string,
	kind ActionKind,
	parameters []ActionParameterSpec,
	evidencePolicy EvidencePolicy,
) (ActionSchema, error) {
	parameters = cloneSlice(parameters)
	sort.Slice(parameters, func(left, right int) bool {
		return parameters[left].Name < parameters[right].Name
	})
	schema := ActionSchema{
		ID: id, Version: version, Kind: kind,
		Parameters: parameters, EvidencePolicy: evidencePolicy,
	}
	schema.SHA256 = actionSchemaSHA256(schema)
	if err := schema.Validate(); err != nil {
		return ActionSchema{}, err
	}
	return schema, nil
}

func (schema ActionSchema) Validate() error {
	if err := validateIdentity(string(schema.ID), "action schema ID"); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidActionSchema, err)
	}
	if err := validateVersion(schema.Version, "action schema version"); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidActionSchema, err)
	}
	if err := validateIdentity(string(schema.Kind), "action kind"); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidActionSchema, err)
	}
	if len(schema.Parameters) > MaxActionParameters {
		return fmt.Errorf("%w: parameter count exceeds %d", ErrInvalidActionSchema, MaxActionParameters)
	}
	previous := ActionArgumentName("")
	for index, parameter := range schema.Parameters {
		if err := validateIdentity(string(parameter.Name), "action parameter name"); err != nil {
			return fmt.Errorf("%w: parameter %d: %v", ErrInvalidActionSchema, index, err)
		}
		if parameter.MaxBytes < 1 || parameter.MaxBytes > MaxActionValueBytes {
			return fmt.Errorf("%w: parameter %q byte limit is outside registered bounds", ErrInvalidActionSchema, parameter.Name)
		}
		if index > 0 && parameter.Name <= previous {
			return fmt.Errorf("%w: parameters must be uniquely sorted by name", ErrInvalidActionSchema)
		}
		previous = parameter.Name
	}
	switch schema.EvidencePolicy {
	case EvidenceOptional, EvidenceRequired, EvidenceForbidden:
	default:
		return fmt.Errorf("%w: evidence policy %q is not registered", ErrInvalidActionSchema, schema.EvidencePolicy)
	}
	if !validSHA256(schema.SHA256) || actionSchemaSHA256(schema) != schema.SHA256 {
		return fmt.Errorf("%w: schema hash does not bind the exact schema", ErrInvalidActionSchema)
	}
	return nil
}

func (schema ActionSchema) Ref() ActionSchemaRef {
	return ActionSchemaRef{ID: schema.ID, Version: schema.Version, SHA256: schema.SHA256}
}

func (schema ActionSchema) Clone() ActionSchema {
	schema.Parameters = cloneSlice(schema.Parameters)
	return schema
}

func (ref ActionSchemaRef) Validate() error {
	if err := validateIdentity(string(ref.ID), "action schema reference ID"); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidActionSchema, err)
	}
	if err := validateVersion(ref.Version, "action schema reference version"); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidActionSchema, err)
	}
	if !validSHA256(ref.SHA256) {
		return fmt.Errorf("%w: action schema reference hash must be 64 lowercase hex characters", ErrInvalidActionSchema)
	}
	return nil
}

func actionSchemaSHA256(schema ActionSchema) string {
	payload := struct {
		ID             ActionSchemaID        `json:"id"`
		Version        string                `json:"version"`
		Kind           ActionKind            `json:"kind"`
		Parameters     []ActionParameterSpec `json:"parameters"`
		EvidencePolicy EvidencePolicy        `json:"evidence_policy"`
	}{schema.ID, schema.Version, schema.Kind, schema.Parameters, schema.EvidencePolicy}
	raw, err := json.Marshal(payload)
	if err != nil {
		panic(fmt.Sprintf("marshal action schema identity: %v", err))
	}
	return contentSHA256(string(raw))
}
