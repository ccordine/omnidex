package assemblyline

import (
	"fmt"
	"strings"
)

const (
	WorkDatabaseQueryPurposeNecessity WorkKind = "database_query_purpose_necessity"
	WorkDatabaseQueryPurposeRelation  WorkKind = "database_query_purpose_relation"

	DatabaseQueryPurposeNecessary    = "NECESSARY_QUERY_PURPOSE"
	DatabaseQueryPurposeNotNecessary = "NOT_NECESSARY_QUERY_PURPOSE"
	DatabaseQueryPurposesSame        = "SAME_QUERY_PURPOSE"
	DatabaseQueryPurposesDistinct    = "DISTINCT_QUERY_PURPOSES"

	DatabaseQueryPurposeNecessitySchemaV1 = "omnidex.database-query-purpose-necessity.v1"
	DatabaseQueryPurposeRelationSchemaV1  = "omnidex.database-query-purpose-relation.v1"
)

type DatabaseQueryPurposeNecessityInput struct {
	Authority      DatabaseQueryPurposeAuthority `json:"authority"`
	Inventory      DatabaseQueryPurposeInventory `json:"inventory"`
	CandidateIndex int                           `json:"candidate_index"`
}

type DatabaseQueryPurposeNecessityResult struct {
	Schema          string `json:"schema"`
	AuthoritySHA256 string `json:"authority_sha256"`
	Relation        string `json:"relation"`
}

type DatabaseQueryPurposeRelationInput struct {
	Collection      DatabaseQueryPurposeCollection `json:"collection"`
	Candidate       string                         `json:"candidate"`
	AcceptedPurpose string                         `json:"accepted_purpose"`
}

type DatabaseQueryPurposeRelationResult struct {
	Schema          string `json:"schema"`
	AuthoritySHA256 string `json:"authority_sha256"`
	Relation        string `json:"relation"`
}

func NewDatabaseQueryPurposeNecessityJob(
	input DatabaseQueryPurposeNecessityInput,
) (PortableJob, error) {
	return newValidatedPortableJob(WorkDatabaseQueryPurposeNecessity, input, input.validate)
}

func (input DatabaseQueryPurposeNecessityInput) validate() error {
	if err := input.Inventory.ValidateFor(input.Authority); err != nil {
		return err
	}
	if input.CandidateIndex < 0 || input.CandidateIndex >= len(input.Inventory.Candidates) {
		return fmt.Errorf("database query purpose candidate index is outside inventory")
	}
	return nil
}

func (input DatabaseQueryPurposeNecessityInput) candidate() string {
	if input.CandidateIndex < 0 || input.CandidateIndex >= len(input.Inventory.Candidates) {
		return ""
	}
	return input.Inventory.Candidates[input.CandidateIndex]
}

func BuildDatabaseQueryPurposeNecessityPrompt(
	input DatabaseQueryPurposeNecessityInput,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	authority, err := renderDatabaseQueryPurposeAuthority(input.Authority)
	if err != nil {
		return "", err
	}
	return strings.Join([]string{
		"Answer one candidate-bound semantic relation: is the exact candidate purpose explicitly required to answer the exact evidence need within the focused query collection?",
		"Evaluate only the necessity of this exact candidate within the focused collection.",
		"Return NECESSARY_QUERY_PURPOSE only when the evidence need directly requires this complete candidate in this collection. Return NOT_NECESSARY_QUERY_PURPOSE for an unrequested, merely plausible, customary, inferred, redundant-across-collections, implementation-oriented, or non-query candidate.",
		"Return only the registered raw relation, with no JSON, label, Markdown, or explanation.",
		"DATABASE QUERY PURPOSE AUTHORITY:\n" + authority,
		"EXACT CANDIDATE PURPOSE:\n" + input.candidate(),
	}, "\n\n"), nil
}

func DecodeDatabaseQueryPurposeNecessityResult(
	input DatabaseQueryPurposeNecessityInput,
	raw string,
) (DatabaseQueryPurposeNecessityResult, error) {
	var zero DatabaseQueryPurposeNecessityResult
	if err := input.validate(); err != nil {
		return zero, err
	}
	leaf, err := decodeRawSemanticLeaf(
		"database query purpose necessity", raw,
		maximumStringBytes(DatabaseQueryPurposeNecessary, DatabaseQueryPurposeNotNecessary), false,
	)
	if err != nil {
		return zero, err
	}
	authoritySHA256, err := databaseQueryPurposeAuthoritySHA256(input)
	if err != nil {
		return zero, err
	}
	result := DatabaseQueryPurposeNecessityResult{
		Schema: DatabaseQueryPurposeNecessitySchemaV1, AuthoritySHA256: authoritySHA256, Relation: leaf,
	}
	if err := result.ValidateFor(input); err != nil {
		return zero, err
	}
	return result, nil
}

func (result DatabaseQueryPurposeNecessityResult) ValidateFor(
	input DatabaseQueryPurposeNecessityInput,
) error {
	if err := input.validate(); err != nil {
		return err
	}
	if result.Schema != DatabaseQueryPurposeNecessitySchemaV1 {
		return fmt.Errorf("database query purpose necessity schema must be %q", DatabaseQueryPurposeNecessitySchemaV1)
	}
	authoritySHA256, err := databaseQueryPurposeAuthoritySHA256(input)
	if err != nil {
		return err
	}
	if result.AuthoritySHA256 != authoritySHA256 {
		return fmt.Errorf("database query purpose necessity authority hash does not match")
	}
	switch result.Relation {
	case DatabaseQueryPurposeNecessary, DatabaseQueryPurposeNotNecessary:
		return nil
	default:
		return fmt.Errorf("database query purpose necessity value %q is not registered", result.Relation)
	}
}

func NewDatabaseQueryPurposeRelationJob(
	input DatabaseQueryPurposeRelationInput,
) (PortableJob, error) {
	return newValidatedPortableJob(WorkDatabaseQueryPurposeRelation, input, input.validate)
}

func (input DatabaseQueryPurposeRelationInput) validate() error {
	if databaseQueryPurposeCollectionLabel(input.Collection) == "" {
		return fmt.Errorf("database query purpose relation collection %q is not registered", input.Collection)
	}
	if err := validateDatabaseQueryPurpose("database query candidate purpose", input.Candidate); err != nil {
		return err
	}
	if err := validateDatabaseQueryPurpose("database query accepted purpose", input.AcceptedPurpose); err != nil {
		return err
	}
	if input.Candidate == input.AcceptedPurpose {
		return fmt.Errorf("identical database query purposes must be deduplicated by code")
	}
	return nil
}

func BuildDatabaseQueryPurposeRelationPrompt(
	input DatabaseQueryPurposeRelationInput,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	return strings.Join([]string{
		fmt.Sprintf("Answer one pairwise semantic relation: do the candidate and already accepted %s purpose express the same query responsibility?", databaseQueryPurposeCollectionLabel(input.Collection)),
		"Compare only whether these two purposes express the same query responsibility.",
		"Return SAME_QUERY_PURPOSE when retaining both would duplicate one query responsibility despite wording differences. Return DISTINCT_QUERY_PURPOSES when each purpose adds a different requested responsibility within this collection.",
		"Return only the registered raw relation, with no JSON, label, Markdown, or explanation.",
		"CANDIDATE PURPOSE:\n" + input.Candidate,
		"ALREADY ACCEPTED PURPOSE:\n" + input.AcceptedPurpose,
	}, "\n\n"), nil
}

func DecodeDatabaseQueryPurposeRelationResult(
	input DatabaseQueryPurposeRelationInput,
	raw string,
) (DatabaseQueryPurposeRelationResult, error) {
	var zero DatabaseQueryPurposeRelationResult
	if err := input.validate(); err != nil {
		return zero, err
	}
	leaf, err := decodeRawSemanticLeaf(
		"database query purpose relation", raw,
		maximumStringBytes(DatabaseQueryPurposesSame, DatabaseQueryPurposesDistinct), false,
	)
	if err != nil {
		return zero, err
	}
	authoritySHA256, err := databaseQueryPurposeAuthoritySHA256(input)
	if err != nil {
		return zero, err
	}
	result := DatabaseQueryPurposeRelationResult{
		Schema: DatabaseQueryPurposeRelationSchemaV1, AuthoritySHA256: authoritySHA256, Relation: leaf,
	}
	if err := result.ValidateFor(input); err != nil {
		return zero, err
	}
	return result, nil
}

func (result DatabaseQueryPurposeRelationResult) ValidateFor(
	input DatabaseQueryPurposeRelationInput,
) error {
	if err := input.validate(); err != nil {
		return err
	}
	if result.Schema != DatabaseQueryPurposeRelationSchemaV1 {
		return fmt.Errorf("database query purpose relation schema must be %q", DatabaseQueryPurposeRelationSchemaV1)
	}
	authoritySHA256, err := databaseQueryPurposeAuthoritySHA256(input)
	if err != nil {
		return err
	}
	if result.AuthoritySHA256 != authoritySHA256 {
		return fmt.Errorf("database query purpose relation authority hash does not match")
	}
	switch result.Relation {
	case DatabaseQueryPurposesSame, DatabaseQueryPurposesDistinct:
		return nil
	default:
		return fmt.Errorf("database query purpose relation value %q is not registered", result.Relation)
	}
}

func databaseQueryPurposeCollectionLabel(collection DatabaseQueryPurposeCollection) string {
	switch collection {
	case DatabaseQueryProjectionPurpose:
		return "projection"
	case DatabaseQueryFilterPurpose:
		return "filter"
	case DatabaseQueryFilterValuePurpose:
		return "set-membership literal"
	case DatabaseQueryWindowPurpose:
		return "temporal-window"
	case DatabaseQueryExistencePurpose:
		return "existence-predicate"
	case DatabaseQueryHavingPurpose:
		return "having-predicate"
	case DatabaseQueryOrderPurpose:
		return "ordering-term"
	default:
		return ""
	}
}

func renderDatabaseQueryPurposeAuthority(input DatabaseQueryPurposeAuthority) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	accepted, err := renderDatabaseQueryAcceptedQuery(input.State)
	if err != nil {
		return "", err
	}
	sections := []string{
		accepted,
		"FOCUSED QUERY COLLECTION:\n" + databaseQueryPurposeCollectionLabel(input.Collection),
	}
	if input.ScopeRelationID != "" {
		focused, err := renderDatabaseQueryFocusedRelation(input.State, input.ScopeRelationID)
		if err != nil {
			return "", err
		}
		sections = append(sections, focused)
	}
	if input.ParentPurpose != "" {
		sections = append(sections, "ACCEPTED PARENT PURPOSE:\n"+input.ParentPurpose)
	}
	if input.FocusedFieldID != "" {
		focused, err := renderDatabaseQueryFocusedField(input.State, input.FocusedFieldID)
		if err != nil {
			return "", err
		}
		sections = append(sections, focused)
	}
	if input.FocusedOperator != "" {
		sections = append(sections, "ACCEPTED FILTER OPERATOR:\n"+string(input.FocusedOperator))
	}
	return renderDatabaseQueryAuthority(input.State, sections...), nil
}
