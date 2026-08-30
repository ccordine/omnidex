package assemblyline

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/datasource"
	"github.com/gryph/omnidex/internal/exactjson"
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

	DatabaseQueryNoPurposeCandidates = "NO_QUERY_PURPOSE_CANDIDATES"

	DatabaseQueryPurposeInventorySchemaV1 = "omnidex.database-query-purpose-inventory.v1"

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
	Schema          string   `json:"schema"`
	AuthoritySHA256 string   `json:"authority_sha256"`
	RawSHA256       string   `json:"raw_sha256"`
	Candidates      []string `json:"candidates"`
}

func NewDatabaseQueryPurposeInventoryJob(
	input DatabaseQueryPurposeAuthority,
) (PortableJob, error) {
	return newValidatedPortableJob(WorkDatabaseQueryPurposeInventory, input, input.validate)
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
		fmt.Sprintf("Return one bounded source-ordered raw inventory of candidate %s purposes expressed by the exact evidence need.", label),
		"Include each semantically separable purpose that could belong to this exact collection, including repeated or potentially unnecessary purposes. Make every candidate a concise standalone statement of why that query-clause collection is needed. For filter values, each line states only one requested literal meaning.",
		"Do not add customary constraints, implied reporting conventions, implementation details, or purposes from another collection. Preserve the source order in which the evidence need expresses the candidate purposes.",
		fmt.Sprintf("Return at most %d candidates, one concise candidate purpose per non-empty raw line. If no candidate exists for this collection, return only %s.", MaxDatabaseQueryPurposeCandidates, DatabaseQueryNoPurposeCandidates),
		"Return raw candidate text only, with no JSON, labels, Markdown, explanation, or surrounding envelope.",
		"DATABASE QUERY PURPOSE INVENTORY AUTHORITY:\n" + authority,
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
	if leaf != DatabaseQueryNoPurposeCandidates {
		candidates = strings.Split(leaf, "\n")
		if len(candidates) > MaxDatabaseQueryPurposeCandidates {
			return zero, fmt.Errorf("database query purpose inventory exceeds its %d-candidate bound", MaxDatabaseQueryPurposeCandidates)
		}
		for index, candidate := range candidates {
			if err := validateDatabaseQueryPurpose(
				fmt.Sprintf("database query purpose candidate %d", index), candidate,
			); err != nil {
				return zero, err
			}
		}
	}
	authoritySHA256, err := databaseQueryPurposeAuthoritySHA256(input)
	if err != nil {
		return zero, err
	}
	result := DatabaseQueryPurposeInventory{
		Schema: DatabaseQueryPurposeInventorySchemaV1, AuthoritySHA256: authoritySHA256,
		RawSHA256: ExactObjectiveContextSHA(leaf), Candidates: append([]string{}, candidates...),
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
	authoritySHA256, err := databaseQueryPurposeAuthoritySHA256(input)
	if err != nil {
		return err
	}
	if inventory.AuthoritySHA256 != authoritySHA256 {
		return fmt.Errorf("database query purpose inventory authority hash does not match")
	}
	if inventory.Candidates == nil || len(inventory.Candidates) > MaxDatabaseQueryPurposeCandidates {
		return fmt.Errorf("database query purpose inventory must contain 0..%d candidates", MaxDatabaseQueryPurposeCandidates)
	}
	for index, candidate := range inventory.Candidates {
		if err := validateDatabaseQueryPurpose(
			fmt.Sprintf("database query purpose candidate %d", index), candidate,
		); err != nil {
			return err
		}
	}
	raw := DatabaseQueryNoPurposeCandidates
	if len(inventory.Candidates) > 0 {
		raw = strings.Join(inventory.Candidates, "\n")
	}
	if inventory.RawSHA256 != ExactObjectiveContextSHA(raw) {
		return fmt.Errorf("database query purpose inventory raw hash does not match")
	}
	return nil
}

func validateDatabaseQueryPurpose(label, purpose string) error {
	leaf, err := decodeRawSemanticLeaf(label, purpose, maxDatabaseQueryPurposeBytes, false)
	if err != nil {
		return err
	}
	if leaf == DatabaseQueryNoPurposeCandidates {
		return fmt.Errorf("%s cannot equal the inventory absence value", label)
	}
	return nil
}

func databaseQueryPurposeAuthoritySHA256(value any) (string, error) {
	authority, err := exactjson.Canonical(value)
	if err != nil {
		return "", fmt.Errorf("encode database query purpose authority: %w", err)
	}
	return ExactObjectiveContextSHA(string(authority)), nil
}
