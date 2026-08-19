package datasource

import (
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"time"
)

const (
	DelegatedSchemaPath          = "/v1/omnidex/database/schema"
	DelegatedEvidencePath        = "/v1/omnidex/database/evidence"
	DelegatedSchemaRequestV1     = "omnidex.delegated-schema-request.v1"
	DelegatedSchemaResponseV1    = "omnidex.delegated-schema-response.v1"
	DelegatedEvidenceRequestV1   = "omnidex.delegated-evidence-request.v1"
	DelegatedEvidenceResponseV1  = "omnidex.delegated-evidence-response.v1"
	DelegatedErrorResponseV1     = "omnidex.delegated-error.v1"
	DelegatedCredentialEnvPrefix = "OMNIDEX_DELEGATED_AUTHORITY_"
	MaxDelegatedResponseBytes    = 16 * 1024 * 1024
	MaxDelegatedAuthorityIDBytes = 68
)

var (
	delegatedAuthorityIDPattern   = regexp.MustCompile(`^dba_[0-9a-f]{64}$`)
	delegatedSourceIDPattern      = regexp.MustCompile(`^[a-z0-9][a-z0-9_.:-]{0,127}$`)
	delegatedErrorCodePattern     = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)
	delegatedCredentialEnvPattern = regexp.MustCompile(`^OMNIDEX_DELEGATED_AUTHORITY_[A-Z][A-Z0-9_]{0,93}_TOKEN$`)
)

type DelegatedSchemaRequest struct {
	Schema      string `json:"schema"`
	SourceID    string `json:"source_id"`
	AuthorityID string `json:"authority_id"`
}

type DelegatedSchemaResponse struct {
	Schema     string                        `json:"schema"`
	SourceID   string                        `json:"source_id"`
	CapturedAt time.Time                     `json:"captured_at"`
	Relations  []DelegatedRelationDefinition `json:"relations"`
}

type DelegatedColumnDefinition struct {
	Name          string             `json:"name"`
	Ordinal       int                `json:"ordinal"`
	DataType      string             `json:"data_type"`
	TypeCategory  ColumnTypeCategory `json:"type_category"`
	Nullable      bool               `json:"nullable"`
	Generated     bool               `json:"generated"`
	Identity      bool               `json:"identity"`
	AllowedValues []string           `json:"allowed_values"`
}

type DelegatedForeignKeyDefinition struct {
	Name               string   `json:"name"`
	Columns            []string `json:"columns"`
	ReferencedSchema   string   `json:"referenced_schema"`
	ReferencedRelation string   `json:"referenced_relation"`
	ReferencedColumns  []string `json:"referenced_columns"`
}

type DelegatedRelationDefinition struct {
	Schema         string                          `json:"schema"`
	Name           string                          `json:"name"`
	Kind           RelationKind                    `json:"kind"`
	RowEstimate    int64                           `json:"row_estimate"`
	Columns        []DelegatedColumnDefinition     `json:"columns"`
	PrimaryKeyName string                          `json:"primary_key_name"`
	PrimaryKey     []string                        `json:"primary_key"`
	ForeignKeys    []DelegatedForeignKeyDefinition `json:"foreign_keys"`
}

type DelegatedExecutionLimits struct {
	MaxTotalCost       float64 `json:"max_total_cost"`
	MaxPlanRows        int64   `json:"max_plan_rows"`
	MaxRows            int     `json:"max_rows"`
	MaxBytes           int     `json:"max_bytes"`
	StatementTimeoutMS int64   `json:"statement_timeout_ms"`
	LockTimeoutMS      int64   `json:"lock_timeout_ms"`
}

type DelegatedEvidenceRequest struct {
	Schema      string                   `json:"schema"`
	AuthorityID string                   `json:"authority_id"`
	Snapshot    SchemaSnapshot           `json:"snapshot"`
	Plan        RelationalQueryPlan      `json:"plan"`
	Limits      DelegatedExecutionLimits `json:"limits"`
}

type DelegatedEvidenceResponse struct {
	Schema   string         `json:"schema"`
	Evidence EvidenceResult `json:"evidence"`
}

type DelegatedErrorResponse struct {
	Schema    string `json:"schema"`
	ErrorCode string `json:"error_code"`
}

func ValidateDelegatedAuthorityID(value string) error {
	if len(value) != MaxDelegatedAuthorityIDBytes || !delegatedAuthorityIDPattern.MatchString(value) {
		return fmt.Errorf("delegated database authority must be one opaque dba_ identity")
	}
	return nil
}

func ValidateDelegatedCredentialEnvironmentName(value string) error {
	if !delegatedCredentialEnvPattern.MatchString(value) {
		return fmt.Errorf(
			"delegated credential environment must use dedicated namespace %s*",
			DelegatedCredentialEnvPrefix,
		)
	}
	return nil
}

func DelegatedAuthorityURLEnvironmentName(credentialEnvironment string) (string, error) {
	if err := ValidateDelegatedCredentialEnvironmentName(credentialEnvironment); err != nil {
		return "", err
	}
	return strings.TrimSuffix(credentialEnvironment, "_TOKEN") + "_URL", nil
}

func NewDelegatedExecutionLimits(limits ExecutionLimits) (DelegatedExecutionLimits, error) {
	if err := validateExecutionBounds(1, limits); err != nil {
		return DelegatedExecutionLimits{}, err
	}
	return DelegatedExecutionLimits{
		MaxTotalCost: limits.MaxTotalCost, MaxPlanRows: limits.MaxPlanRows,
		MaxRows: limits.MaxRows, MaxBytes: limits.MaxBytes,
		StatementTimeoutMS: limits.StatementTimeout.Milliseconds(),
		LockTimeoutMS:      limits.LockTimeout.Milliseconds(),
	}, nil
}

func (limits DelegatedExecutionLimits) ExecutionLimits() (ExecutionLimits, error) {
	resolved := ExecutionLimits{
		MaxTotalCost: limits.MaxTotalCost, MaxPlanRows: limits.MaxPlanRows,
		MaxRows: limits.MaxRows, MaxBytes: limits.MaxBytes,
		StatementTimeout: time.Duration(limits.StatementTimeoutMS) * time.Millisecond,
		LockTimeout:      time.Duration(limits.LockTimeoutMS) * time.Millisecond,
	}
	if err := validateExecutionBounds(1, resolved); err != nil {
		return ExecutionLimits{}, err
	}
	return resolved, nil
}

func (request DelegatedSchemaRequest) Validate() error {
	if request.Schema != DelegatedSchemaRequestV1 || !delegatedSourceIDPattern.MatchString(request.SourceID) {
		return fmt.Errorf("delegated schema request has invalid protocol or source authority")
	}
	return ValidateDelegatedAuthorityID(request.AuthorityID)
}

func (response DelegatedSchemaResponse) Snapshot(sourceID, sourceName string) (SchemaSnapshot, error) {
	if response.Schema != DelegatedSchemaResponseV1 || response.SourceID != sourceID ||
		response.CapturedAt.IsZero() || response.CapturedAt.Location() != time.UTC {
		return SchemaSnapshot{}, fmt.Errorf("delegated schema response does not match its requested source authority")
	}
	definitions := make([]RelationDefinition, len(response.Relations))
	for index, relation := range response.Relations {
		if relation.RowEstimate < 0 {
			return SchemaSnapshot{}, fmt.Errorf("delegated schema relation has a negative row estimate")
		}
		definition := RelationDefinition{
			Schema: relation.Schema, Name: relation.Name, Kind: relation.Kind,
			RowEstimate: relation.RowEstimate, PrimaryKeyName: relation.PrimaryKeyName,
			PrimaryKey: append([]string(nil), relation.PrimaryKey...),
		}
		for _, column := range relation.Columns {
			definition.Columns = append(definition.Columns, ColumnDefinition{
				Name: column.Name, Ordinal: column.Ordinal, DataType: column.DataType,
				TypeCategory: column.TypeCategory, Nullable: column.Nullable,
				Generated: column.Generated, Identity: column.Identity,
				AllowedValues: append([]string(nil), column.AllowedValues...),
			})
		}
		for _, foreignKey := range relation.ForeignKeys {
			definition.ForeignKeys = append(definition.ForeignKeys, ForeignKeyDefinition{
				Name: foreignKey.Name, Columns: append([]string(nil), foreignKey.Columns...),
				ReferencedSchema:   foreignKey.ReferencedSchema,
				ReferencedRelation: foreignKey.ReferencedRelation,
				ReferencedColumns:  append([]string(nil), foreignKey.ReferencedColumns...),
			})
		}
		definitions[index] = definition
	}
	snapshot, err := NewSchemaSnapshot(sourceID, sourceName, definitions, response.CapturedAt)
	if err != nil {
		return SchemaSnapshot{}, fmt.Errorf("build delegated schema snapshot: %w", err)
	}
	return snapshot, nil
}

func (request DelegatedEvidenceRequest) Validate(snapshot SchemaSnapshot) (ExecutionLimits, error) {
	if request.Schema != DelegatedEvidenceRequestV1 {
		return ExecutionLimits{}, fmt.Errorf("delegated evidence request has an unsupported protocol")
	}
	if err := ValidateDelegatedAuthorityID(request.AuthorityID); err != nil {
		return ExecutionLimits{}, err
	}
	if err := request.Snapshot.ValidateIntegrity(); err != nil || !reflect.DeepEqual(request.Snapshot, snapshot) {
		return ExecutionLimits{}, fmt.Errorf("delegated evidence request has stale schema authority")
	}
	if err := request.Plan.Validate(snapshot); err != nil {
		return ExecutionLimits{}, err
	}
	limits, err := request.Limits.ExecutionLimits()
	if err != nil {
		return ExecutionLimits{}, err
	}
	if err := validateExecutionBounds(request.Plan.Intent.Limit, limits); err != nil {
		return ExecutionLimits{}, err
	}
	return limits, nil
}

func (response DelegatedEvidenceResponse) Validate(
	snapshot SchemaSnapshot,
	plan RelationalQueryPlan,
	limits ExecutionLimits,
) error {
	if response.Schema != DelegatedEvidenceResponseV1 {
		return fmt.Errorf("delegated evidence response has an unsupported protocol")
	}
	return response.Evidence.ValidateForPlan(snapshot, plan, limits)
}

func (response DelegatedErrorResponse) Validate() error {
	if response.Schema != DelegatedErrorResponseV1 || !delegatedErrorCodePattern.MatchString(response.ErrorCode) ||
		response.ErrorCode != strings.TrimSpace(response.ErrorCode) {
		return fmt.Errorf("delegated host returned an invalid error envelope")
	}
	return nil
}
