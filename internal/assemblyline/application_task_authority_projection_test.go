package assemblyline

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestApplicationTaskAuthorityProjectionsUseExactAcceptedRequirement(t *testing.T) {
	t.Parallel()
	specification := ApplicationSpecification{
		Surface: ApplicationSurfaceService, ProductQuote: "sentinel inventory service",
		Requirements: []Requirement{{
			ID: "requirement_001", SourceQuote: "accept one inventory record",
		}},
	}
	frozen, err := FreezeApplicationWorkload(specification)
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := ProjectApplicationTaskRuntimeAuthority(frozen, "task_001")
	if err != nil {
		t.Fatal(err)
	}
	verification, err := ProjectApplicationTaskVerificationAuthority(frozen, "task_001")
	if err != nil {
		t.Fatal(err)
	}
	if runtime.RequirementQuote != specification.Requirements[0].SourceQuote ||
		verification.RequirementQuote != specification.Requirements[0].SourceQuote {
		t.Fatalf("projections drifted from accepted requirement: runtime=%+v verification=%+v", runtime, verification)
	}
	for _, projection := range []any{runtime, verification} {
		raw, err := json.Marshal(projection)
		if err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{"objective", "required_behaviors", "acceptance_criteria"} {
			if strings.Contains(string(raw), forbidden) {
				t.Fatalf("projection contains obsolete planner field %q: %s", forbidden, raw)
			}
		}
	}
}

func TestApplicationTaskAuthorityProjectionsRejectUnboundTask(t *testing.T) {
	t.Parallel()
	frozen, err := FreezeApplicationWorkload(applicationWorkloadTestSpecification())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ProjectApplicationTaskRuntimeAuthority(frozen, "task_999"); err == nil {
		t.Fatal("runtime authority accepted an unbound task")
	}
	if _, err := ProjectApplicationTaskVerificationAuthority(frozen, "task_999"); err == nil {
		t.Fatal("verification authority accepted an unbound task")
	}
}
