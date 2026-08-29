package assemblyline

import "testing"

func applicationWorkloadTestSpecification() ApplicationSpecification {
	return ApplicationSpecification{
		Surface:      ApplicationSurfaceBrowser,
		ProductQuote: "browser operations console",
		Requirements: []Requirement{
			{ID: "requirement_001", SourceQuote: "group records by status"},
			{ID: "requirement_002", SourceQuote: "filter records quickly"},
			{ID: "requirement_003", SourceQuote: "export printable summaries"},
		},
	}
}

func applicationTaskAuthorityProjectionFixture(
	t *testing.T,
) (ApplicationSpecification, FrozenApplicationWorkload) {
	t.Helper()
	specification := ApplicationSpecification{
		Surface: ApplicationSurfaceService, ProductQuote: "sentinel inventory service",
		Requirements: []Requirement{{
			ID:          "requirement_001",
			SourceQuote: "A later request can observe an earlier accepted inventory value.",
		}},
	}
	frozen, err := FreezeApplicationWorkload(specification)
	if err != nil {
		t.Fatal(err)
	}
	return specification, frozen
}
