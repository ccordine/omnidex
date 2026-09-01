package assemblyline

import "fmt"

const WorkDatabaseQueryPurposePresence WorkKind = "database_query_purpose_presence"

const DatabaseQueryPurposePresenceSchemaV1 = "omnidex.database-query-purpose-presence.v1"

const (
	databaseQueryPurposePresent = "code-owned:purpose-present"
	databaseQueryPurposeAbsent  = "code-owned:purpose-absent"
)

// DatabaseQueryPurposePresenceResult is code-owned state assembled from one
// opaque binary semantic choice. Neither internal value is model-visible.
type DatabaseQueryPurposePresenceResult struct {
	Schema          string `json:"schema"`
	AuthoritySHA256 string `json:"authority_sha256"`
	Present         bool   `json:"present"`
}

func NewDatabaseQueryPurposePresenceJob(
	input DatabaseQueryPurposeAuthority,
) (PortableJob, error) {
	return newPortableJob(WorkDatabaseQueryPurposePresence, input)
}

func BuildDatabaseQueryPurposePresencePrompt(
	input DatabaseQueryPurposeAuthority,
) (string, error) {
	authority, err := renderDatabaseQueryPurposeAuthority(input)
	if err != nil {
		return "", err
	}
	choices, err := databaseQueryPurposePresenceChoices(input)
	if err != nil {
		return "", err
	}
	return RenderOpaqueModelChoiceQuestion(
		fmt.Sprintf(
			"Does the exact evidence need express any additional %s purpose?",
			databaseQueryPurposeCollectionLabel(input.Collection),
		),
		[]string{"Database query context:\n" + authority},
		choices,
	)
}

func DecodeDatabaseQueryPurposePresenceResult(
	input DatabaseQueryPurposeAuthority,
	raw string,
) (DatabaseQueryPurposePresenceResult, error) {
	var zero DatabaseQueryPurposePresenceResult
	choices, err := databaseQueryPurposePresenceChoices(input)
	if err != nil {
		return zero, err
	}
	value, err := DecodeOpaqueModelChoice(raw, choices)
	if err != nil {
		return zero, err
	}
	authoritySHA256, err := databaseQueryPurposeAuthoritySHA256(input)
	if err != nil {
		return zero, err
	}
	result := DatabaseQueryPurposePresenceResult{
		Schema:          DatabaseQueryPurposePresenceSchemaV1,
		AuthoritySHA256: authoritySHA256,
		Present:         value == databaseQueryPurposePresent,
	}
	if err := result.ValidateFor(input); err != nil {
		return zero, err
	}
	return result, nil
}

func (result DatabaseQueryPurposePresenceResult) ValidateFor(
	input DatabaseQueryPurposeAuthority,
) error {
	if err := input.validate(); err != nil {
		return err
	}
	if result.Schema != DatabaseQueryPurposePresenceSchemaV1 {
		return fmt.Errorf(
			"database query purpose presence schema must be %q",
			DatabaseQueryPurposePresenceSchemaV1,
		)
	}
	authoritySHA256, err := databaseQueryPurposeAuthoritySHA256(input)
	if err != nil {
		return err
	}
	if result.AuthoritySHA256 != authoritySHA256 {
		return fmt.Errorf("database query purpose presence authority hash does not match")
	}
	return nil
}

func databaseQueryPurposePresenceChoices(
	input DatabaseQueryPurposeAuthority,
) ([]OpaqueModelChoice, error) {
	if err := input.validate(); err != nil {
		return nil, err
	}
	label := databaseQueryPurposeCollectionLabel(input.Collection)
	return databaseOpaqueChoices([]databaseOpaqueChoiceSpec{
		{
			description: "The exact evidence need expresses at least one additional " + label + " purpose",
			value:       databaseQueryPurposePresent,
		},
		{
			description: "The exact evidence need expresses no additional " + label + " purpose",
			value:       databaseQueryPurposeAbsent,
		},
	})
}
