package assemblyline

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/datasource"
)

const (
	WorkDatabaseQueryPurposeInventory WorkKind = "database_query_purpose_inventory"

	DatabaseQueryProjectionPurpose  DatabaseQueryPurposeCollection = "projection"
	DatabaseQueryFilterPurpose      DatabaseQueryPurposeCollection = "filter"
	DatabaseQueryFilterValuePurpose DatabaseQueryPurposeCollection = "filter_value"
	DatabaseQueryWindowPurpose      DatabaseQueryPurposeCollection = "temporal_window"
	DatabaseQueryExistencePurpose   DatabaseQueryPurposeCollection = "existence"
	DatabaseQueryHavingPurpose      DatabaseQueryPurposeCollection = "having"
	DatabaseQueryOrderPurpose       DatabaseQueryPurposeCollection = "order"

	DatabaseQueryPurposeInventorySchemaV1 = "omnidex.database-query-purpose-inventory.v1"
	DatabaseNoQueryPurposeCandidates      = "NO_QUERY_PURPOSE_CANDIDATES"

	MaxDatabaseQueryPurposeCandidates     = 64
	maxDatabaseQueryPurposeBytes          = 1024
	maxDatabaseQueryPurposeInventoryBytes = MaxDatabaseQueryPurposeCandidates*maxDatabaseQueryPurposeBytes +
		MaxDatabaseQueryPurposeCandidates - 1
)

type DatabaseQueryPurposeCollection string

// DatabaseQueryPurposeAuthority is code-owned focus for exactly one query
// collection. Focused field/operator values exist only for set-membership
// literal purposes; ParentPurpose binds those values and scoped filters to the
// already accepted purpose that opened them.
type DatabaseQueryPurposeAuthority struct {
	State           DatabaseQueryIntentLeafState   `json:"state"`
	Collection      DatabaseQueryPurposeCollection `json:"collection"`
	ScopeRelationID string                         `json:"scope_relation_id"`
	ParentPurpose   string                         `json:"parent_purpose"`
	FocusedFieldID  string                         `json:"focused_field_id"`
	FocusedOperator datasource.FilterOperator      `json:"focused_operator"`
}

type DatabaseQueryPurposeInventory struct {
	Schema     string   `json:"schema"`
	Candidates []string `json:"candidates"`
}

func NewDatabaseQueryPurposeInventoryJob(
	input DatabaseQueryPurposeAuthority,
) (PortableJob, error) {
	return newPortableJob(WorkDatabaseQueryPurposeInventory, input)
}

func (input DatabaseQueryPurposeAuthority) validate() error {
	if err := input.State.validateReady(); err != nil {
		return err
	}
	if input.ParentPurpose != "" {
		if err := validateDatabaseQueryPurpose("database query parent purpose", input.ParentPurpose); err != nil {
			return err
		}
	}
	switch input.Collection {
	case DatabaseQueryProjectionPurpose:
		if input.State.Shape == datasource.ResultExistence {
			return fmt.Errorf("existence-shaped database query cannot inventory projection purposes")
		}
		return input.validateNoFocus()
	case DatabaseQueryFilterPurpose:
		if input.FocusedFieldID != "" || input.FocusedOperator != "" {
			return fmt.Errorf("database query filter purpose authority cannot contain a focused value")
		}
		if input.ScopeRelationID == "" {
			if input.ParentPurpose != "" {
				return fmt.Errorf("top-level database query filter purpose cannot contain a parent purpose")
			}
			return nil
		}
		if !databaseQueryRelationExists(input.State, input.ScopeRelationID) {
			return fmt.Errorf("database query filter purpose scope %q was not projected", input.ScopeRelationID)
		}
		if input.ParentPurpose == "" {
			return fmt.Errorf("scoped database query filter purpose requires its accepted parent purpose")
		}
		return nil
	case DatabaseQueryFilterValuePurpose:
		if input.ParentPurpose == "" {
			return fmt.Errorf("database query filter-value purpose requires its accepted filter purpose")
		}
		_, relationID, ok := databaseQueryColumn(input.State, input.FocusedFieldID)
		if !ok || input.ScopeRelationID != "" && relationID != input.ScopeRelationID {
			return fmt.Errorf("database query filter-value field %q is outside its authority", input.FocusedFieldID)
		}
		if input.FocusedOperator != datasource.FilterIn && input.FocusedOperator != datasource.FilterNotIn {
			return fmt.Errorf("database query filter-value purpose requires one set-membership operator")
		}
		return nil
	case DatabaseQueryWindowPurpose, DatabaseQueryExistencePurpose,
		DatabaseQueryHavingPurpose, DatabaseQueryOrderPurpose:
		return input.validateNoFocus()
	default:
		return fmt.Errorf("database query purpose collection %q is not registered", input.Collection)
	}
}

func (input DatabaseQueryPurposeAuthority) validateNoFocus() error {
	if input.ScopeRelationID != "" || input.ParentPurpose != "" ||
		input.FocusedFieldID != "" || input.FocusedOperator != "" {
		return fmt.Errorf("database query %s purpose authority contains unrelated focus", input.Collection)
	}
	return nil
}

func BuildDatabaseQueryPurposeInventoryPrompt(
	input DatabaseQueryPurposeAuthority,
) (string, error) {
	authority, err := renderDatabaseQueryPurposeAuthority(input)
	if err != nil {
		return "", err
	}
	label := databaseQueryPurposeCollectionLabel(input.Collection)
	prompt := strings.Join([]string{
		fmt.Sprintf("What candidate %s purposes are expressed by the exact evidence need?", label),
		"Include each semantically separable purpose that could belong to this exact collection, including repeated or potentially unnecessary purposes. Make every candidate a concise standalone statement of why that query-clause collection is needed. For filter values, each line states only one requested literal meaning.",
		"Do not add customary constraints, implied reporting conventions, implementation details, or purposes from another collection.",
		fmt.Sprintf("Write between 1 and %d concise candidate purposes, one per line. If there are none, answer %s.", MaxDatabaseQueryPurposeCandidates, DatabaseNoQueryPurposeCandidates),
		"Database query context:\n" + authority,
	}, "\n\n")
	if len(prompt) > maxPortablePayloadBytes {
		return "", fmt.Errorf("database query purpose inventory prompt exceeds %d bytes", maxPortablePayloadBytes)
	}
	return prompt, nil
}

func DecodeDatabaseQueryPurposeInventory(
	input DatabaseQueryPurposeAuthority,
	raw string,
) (DatabaseQueryPurposeInventory, error) {
	var zero DatabaseQueryPurposeInventory
	if err := input.validate(); err != nil {
		return zero, err
	}
	leaf, err := decodeRawSemanticLeaf(
		"database query purpose inventory", raw, maxDatabaseQueryPurposeInventoryBytes, true,
	)
	if err != nil {
		return zero, err
	}
	if strings.ContainsRune(leaf, '\r') {
		return zero, fmt.Errorf("database query purpose inventory must use LF line boundaries")
	}
	candidates := []string{}
	if leaf != DatabaseNoQueryPurposeCandidates {
		candidates = strings.Split(leaf, "\n")
	}
	result := DatabaseQueryPurposeInventory{
		Schema: DatabaseQueryPurposeInventorySchemaV1, Candidates: append([]string{}, candidates...),
	}
	if err := result.ValidateFor(input); err != nil {
		return zero, err
	}
	return result, nil
}

func (inventory DatabaseQueryPurposeInventory) ValidateFor(
	input DatabaseQueryPurposeAuthority,
) error {
	if err := input.validate(); err != nil {
		return err
	}
	if inventory.Schema != DatabaseQueryPurposeInventorySchemaV1 {
		return fmt.Errorf("database query purpose inventory schema must be %q", DatabaseQueryPurposeInventorySchemaV1)
	}
	if inventory.Candidates == nil {
		return fmt.Errorf("database query purpose inventory candidates must be an explicit array")
	}
	if len(inventory.Candidates) > MaxDatabaseQueryPurposeCandidates {
		return fmt.Errorf("database query purpose inventory must contain 0..%d candidates", MaxDatabaseQueryPurposeCandidates)
	}
	for index, candidate := range inventory.Candidates {
		if candidate == DatabaseNoQueryPurposeCandidates {
			return fmt.Errorf("database query purpose inventory candidate %d cannot mix the registered absence result with positive candidates", index)
		}
		if err := validateDatabaseQueryPurpose(
			fmt.Sprintf("database query purpose candidate %d", index), candidate,
		); err != nil {
			return err
		}
	}
	return nil
}

func validateDatabaseQueryPurpose(label, purpose string) error {
	_, err := decodeRawSemanticLeaf(label, purpose, maxDatabaseQueryPurposeBytes, false)
	return err
}
