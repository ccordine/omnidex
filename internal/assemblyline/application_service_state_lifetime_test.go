package assemblyline

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestApplicationServiceStateLifetimeIsOneTaskLocalMechanismBlindLeaf(t *testing.T) {
	workloadInput, frozen := applicationTaskAuthorityProjectionFixture(t)
	authority, err := ProjectApplicationTaskRuntimeAuthority(workloadInput, frozen, "task_001")
	if err != nil {
		t.Fatal(err)
	}
	input, err := ProjectApplicationServiceStateLifetimeInput(authority)
	if err != nil {
		t.Fatal(err)
	}
	job, err := NewApplicationServiceStateLifetimeJob(input)
	if err != nil {
		t.Fatal(err)
	}
	if job.Kind != WorkApplicationServiceStateLifetime {
		t.Fatalf("kind=%q", job.Kind)
	}
	prompt, schema, err := RenderPortableJob(job)
	if err != nil {
		t.Fatal(err)
	}
	schemaJSON, err := json.Marshal(schema)
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		ApplicationServiceStateLifetimeSchemaV1,
		string(ApplicationServiceStateRequestLocalOnly),
		string(ApplicationServiceStateCrossRequestAuthorityRequired),
		input.RequirementQuote,
	} {
		if !strings.Contains(prompt, required) && !strings.Contains(string(schemaJSON), required) {
			t.Fatalf("service state lifetime envelope omitted %q", required)
		}
	}
	for _, forbidden := range []string{
		"task_id", "workspace", "filename", "path", "tool", "command", "endpoint",
		"database", "postgresql", "redis", "cache", "filesystem",
		strings.ToLower(frozen.Tasks[0].AcceptanceCriteria[0]), "acceptance_criteria",
	} {
		if strings.Contains(strings.ToLower(prompt), forbidden) {
			t.Fatalf("service state lifetime prompt exposed %q authority: %s", forbidden, prompt)
		}
	}
}

func TestApplicationServiceStateLifetimeStrictlyDecodesRegisteredLeaf(t *testing.T) {
	input := serviceStateLifetimeFixture()
	for _, lifetime := range []ApplicationServiceStateLifetime{
		ApplicationServiceStateRequestLocalOnly,
		ApplicationServiceStateCrossRequestAuthorityRequired,
	} {
		raw := `{"schema":"` + ApplicationServiceStateLifetimeSchemaV1 +
			`","state_lifetime":"` + string(lifetime) + `"}`
		result, err := DecodeApplicationServiceStateLifetimeResult(input, raw)
		if err != nil {
			t.Fatalf("lifetime %q: %v", lifetime, err)
		}
		if result.StateLifetime != lifetime {
			t.Fatalf("lifetime=%q want=%q", result.StateLifetime, lifetime)
		}
	}
	for name, raw := range map[string]string{
		"unknown": `{"schema":"` + ApplicationServiceStateLifetimeSchemaV1 +
			`","state_lifetime":"durable"}`,
		"extra": `{"schema":"` + ApplicationServiceStateLifetimeSchemaV1 +
			`","state_lifetime":"request_local_only","mechanism":"database"}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := DecodeApplicationServiceStateLifetimeResult(input, raw); err == nil {
				t.Fatal("accepted invalid service state lifetime result")
			}
		})
	}
}

func TestApplicationServiceStateLifetimeRejectsPathBearingOrOversizedAuthority(t *testing.T) {
	pathBearing := serviceStateLifetimeFixture()
	pathBearing.RequirementQuote = "Retain values described by src/state.php."
	if _, err := NewApplicationServiceStateLifetimeJob(pathBearing); err == nil {
		t.Fatal("accepted path-bearing service state lifetime authority")
	}
	overLimit := serviceStateLifetimeFixture()
	overLimit.Objective = strings.Repeat("x", maxApplicationObjectiveRunes+1)
	if _, err := NewApplicationServiceStateLifetimeJob(overLimit); err == nil {
		t.Fatal("accepted oversized service state lifetime authority")
	}
}

func serviceStateLifetimeFixture() ApplicationServiceStateLifetimeInput {
	return ApplicationServiceStateLifetimeInput{
		ProductContext:    "inventory service",
		RequirementQuote:  "A later request can observe an earlier accepted value.",
		Objective:         "Preserve accepted values across separate requests.",
		RequiredBehaviors: []string{"Retain each accepted value for later retrieval."},
	}
}
