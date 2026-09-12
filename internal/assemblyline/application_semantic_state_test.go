package assemblyline

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestApplicationSemanticResultsContainValuesWithoutHashReceipts(t *testing.T) {
	t.Parallel()
	for _, statement := range []string{
		"The software lists the supplied labels.",
		"The software calculates the sum of the supplied numbers.",
	} {
		t.Run(statement, func(t *testing.T) {
			context, err := BootstrapApplicationContext(statement)
			if err != nil {
				t.Fatal(err)
			}
			authorization, err := DecodeApplicationRequirementCandidateAuthorizationResult(
				ApplicationRequirementCandidateAuthorizationInput{
					UserRequest: statement, Context: context, Candidate: statement,
				}, "A",
			)
			if err != nil {
				t.Fatal(err)
			}
			authority := applicationRequirementCandidateResultRelationAuthorityFixture(t, statement)
			presence, err := DecodeApplicationRequirementCandidateResultPresenceResult(
				ApplicationRequirementCandidateResultPresenceInput{
					Candidate: statement, Kind: authority.Kind, Cardinality: authority.Cardinality,
					Dimension: ApplicationRequirementDerivedValueDimension,
				}, "B",
			)
			if err != nil {
				t.Fatal(err)
			}
			result, err := ResolveApplicationRequirementCandidateResultRelation(authority, presence, nil)
			if err != nil {
				t.Fatal(err)
			}
			for _, value := range []any{context, authorization, authority.Kind, authority.Cardinality, presence, result} {
				encoded, err := json.Marshal(value)
				if err != nil {
					t.Fatal(err)
				}
				if strings.Contains(string(encoded), "sha256") {
					t.Fatalf("semantic state still requires hash receipts: %s", encoded)
				}
			}
			if authorization.Relation != ApplicationRequirementCandidateEntailed ||
				result.Relation != ApplicationRequirementNoDerivedResult {
				t.Fatal("plain-text responses were not mapped to their semantic values")
			}
		})
	}
}

func TestSemanticLeafResultsDoNotCarryAuthorityOrResponseHashes(t *testing.T) {
	t.Parallel()
	for _, value := range []any{
		ApplicationRequirementInventory{}, ApplicationRequirementCandidatePartition{},
		ContextRelevanceRelationResult{}, DatabaseSchemaRelationChoiceResult{},
		DatabaseQueryPurposeInventory{},
		DatabaseQueryPurposeNecessityResult{}, DatabaseQueryPurposeRelationResult{},
		GroundedAnswerParagraphInventory{}, RoleplayGroundedParagraphInventory{},
		RoleplayCanonFactInventory{},
		RoleplayCanonFactCandidateAuthorization{}, RoleplayCanonFactCandidateRelation{},
	} {
		typeOf := reflect.TypeOf(value)
		for index := 0; index < typeOf.NumField(); index++ {
			field := typeOf.Field(index)
			if strings.Contains(field.Tag.Get("json"), "sha256") {
				t.Fatalf("%s still carries a hash receipt field %s", typeOf.Name(), field.Name)
			}
		}
	}
}
