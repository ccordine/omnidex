package assemblyline

import (
	"reflect"
	"strings"
	"testing"
)

func TestDatabaseSemanticStationJSONBoundariesContainNoExecutionAuthority(t *testing.T) {
	t.Parallel()
	assertExactJSONFields(t, reflect.TypeOf(DatabaseSchemaSelectionInput{}), []string{
		"evidence_need_id", "exact_need", "objective_context", "candidates", "max_selections",
	})
	assertExactJSONFields(t, reflect.TypeOf(DatabaseSchemaSelectionDecision{}), []string{
		"schema", "evidence_need_id", "relation_ids",
	})
	assertExactJSONFields(t, reflect.TypeOf(DatabaseSchemaRelationInventoryInput{}), []string{
		"exact_need", "objective_context", "candidates",
	})
	assertExactJSONFields(t, reflect.TypeOf(DatabaseSchemaRelationNecessityInput{}), []string{
		"exact_need", "objective_context", "candidate",
	})
	assertExactJSONFields(t, reflect.TypeOf(DatabaseSchemaRelationResolutionInput{}), []string{
		"candidate", "candidates",
	})
	assertExactJSONFields(t, reflect.TypeOf(DatabaseQueryIntentInput{}), []string{
		"evidence_need_id", "exact_need", "objective_context", "schema_projection", "temporal_as_of", "max_rows",
	})
	assertExactJSONFields(t, reflect.TypeOf(DatabaseQueryIntentDecision{}), []string{
		"schema", "evidence_need_id", "from_relation_id", "shape", "projections", "filters",
		"temporal_windows", "exists", "group_by", "having", "order_by", "limit",
	})
	assertExactJSONFields(t, reflect.TypeOf(DatabaseJoinPathSelectionInput{}), []string{
		"evidence_need_id", "exact_need", "objective_context", "from_relation_id",
		"to_relation_id", "candidates",
	})
	assertExactJSONFields(t, reflect.TypeOf(DatabaseJoinPathSelectionDecision{}), []string{
		"schema", "evidence_need_id", "path_id",
	})
}

func TestDatabaseSemanticPromptsContainNoConnectionOrRawQueryBytes(t *testing.T) {
	t.Parallel()
	queryInput, _, _ := databaseQueryIntentFixture(t)
	queryState := NewDatabaseQueryIntentLeafState(queryInput)
	queryState.FromRelationID = queryInput.SchemaProjection.Relations[0].ID
	jobs := []PortableJob{}
	for _, build := range []func() (PortableJob, error){
		func() (PortableJob, error) {
			input, err := ProjectDatabaseSchemaRelationInventoryInput(
				databaseSchemaSelectionFixture(),
			)
			if err != nil {
				return PortableJob{}, err
			}
			return NewDatabaseSchemaRelationInventoryJob(input)
		},
		func() (PortableJob, error) { return NewDatabaseQueryShapeJob(queryState) },
		func() (PortableJob, error) {
			return NewDatabaseJoinPathSelectionJob(DatabaseJoinPathSelectionInput{
				EvidenceNeedID: "need-join", ExactNeed: "Associate the event with its owner.",
				FromRelationID: "rel_events", ToRelationID: "rel_people",
				Candidates: []DatabaseJoinPathCandidate{
					{PathID: "path_owner", Descriptor: `[{"foreign_key":"events.owner_id"}]`},
					{PathID: "path_actor", Descriptor: `[{"foreign_key":"events.actor_id"}]`},
				},
			})
		},
	} {
		job, err := build()
		if err != nil {
			t.Fatal(err)
		}
		jobs = append(jobs, job)
	}
	for _, job := range jobs {
		prompt, err := RenderPortableJob(job)
		if err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{
			"credential-password", "postgres://reader:credential", `SELECT "credential_probe"`,
			`"password":`, `"dsn":`, `"sql":`, `"parameters":`,
			"choose an operation", "choose an environment operation", "claim execution",
			"do not write sql",
		} {
			if strings.Contains(strings.ToLower(prompt), strings.ToLower(forbidden)) {
				t.Fatalf("%s prompt contains forbidden connection/query bytes %q", job.Kind, forbidden)
			}
		}
	}
}
