package assemblyline

import (
	"strings"
	"testing"
)

func TestApplicationContextProjectionOmitsCodeOwnedFactState(t *testing.T) {
	t.Parallel()
	const request = "Build a maintenance tracker."
	const factValue = "The repository already uses PostgreSQL."
	context := ApplicationContext{
		Schema:        ApplicationContextSchemaV1,
		RequestSHA256: ExactObjectiveContextSHA(request),
		Facts: []ApplicationContextFact{{
			ID:           "fact_001",
			Kind:         ApplicationContextRepositoryFact,
			Authority:    ApplicationContextEvidenceAuthority,
			NeedID:       "need_001",
			Value:        factValue,
			SourceID:     "repository-snapshot",
			SourceSHA256: ExactObjectiveContextSHA(factValue),
		}},
	}
	if err := context.Validate(); err != nil {
		t.Fatal(err)
	}
	projection := renderApplicationContextModelProjection(request, context)
	for _, required := range []string{request, factValue} {
		if !strings.Contains(projection, required) {
			t.Fatalf("application context projection omitted %q: %s", required, projection)
		}
	}
	for _, forbidden := range []string{
		context.Schema,
		context.RequestSHA256,
		context.Facts[0].ID,
		string(context.Facts[0].Kind),
		string(context.Facts[0].Authority),
		context.Facts[0].NeedID,
		context.Facts[0].SourceID,
		context.Facts[0].SourceSHA256,
	} {
		if strings.Contains(projection, forbidden) {
			t.Fatalf("application context projection exposed code-owned value %q: %s", forbidden, projection)
		}
	}
}
