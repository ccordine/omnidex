package assemblyline

import "testing"

const structuredSchemaUnsafeRepetition = 2000

func TestActiveLongTextSchemasAvoidUnsafeFiniteGrammarRepetitions(t *testing.T) {
	databaseInput, _, _ := databaseQueryIntentFixture(t)
	databaseGapInput := DatabaseEvidenceGapInput{
		RequirementID: "requirement-1", ExactRequirement: "How many records exist?",
		Evidence: []GroundedEvidenceCapsule{{ID: "E1", Text: "Record count: 4."}},
	}
	tests := []struct {
		name   string
		schema func() (map[string]any, error)
	}{
		{name: "context minification", schema: func() (map[string]any, error) {
			return ContextMinificationResponseSchema(), nil
		}},
		{name: "database evidence gap", schema: func() (map[string]any, error) {
			return DatabaseEvidenceGapResponseSchema(databaseGapInput)
		}},
		{name: "grounded answer", schema: func() (map[string]any, error) {
			return GroundedAnswerResponseSchema(groundedAnswerFixture())
		}},
		{name: "repository grounded correction", schema: func() (map[string]any, error) {
			return RepositoryGroundedCorrectionResponseSchema(repositoryGroundedCorrectionFixture())
		}},
		{name: "database query intent", schema: func() (map[string]any, error) {
			return DatabaseQueryIntentResponseSchema(databaseInput)
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			schema, err := test.schema()
			if err != nil {
				t.Fatal(err)
			}
			assertNoUnsafeStructuredSchemaRepetition(t, "response", schema)
		})
	}
}

func assertNoUnsafeStructuredSchemaRepetition(t *testing.T, path string, value any) {
	t.Helper()
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			childPath := path + "." + key
			if key == "maxLength" {
				maximum, ok := child.(int)
				if !ok {
					t.Fatalf("%s uses non-integer maxLength %T", childPath, child)
				}
				if maximum >= structuredSchemaUnsafeRepetition {
					t.Fatalf(
						"%s encodes maxLength=%d at or above the structured grammar repetition limit",
						childPath, maximum,
					)
				}
			}
			assertNoUnsafeStructuredSchemaRepetition(t, childPath, child)
		}
	case []any:
		for _, child := range typed {
			assertNoUnsafeStructuredSchemaRepetition(t, path, child)
		}
	}
}
