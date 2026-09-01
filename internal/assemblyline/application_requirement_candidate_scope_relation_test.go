package assemblyline

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

func TestApplicationRequirementCandidateScopeRelationIsOpaqueBoundAnnotation(t *testing.T) {
	t.Parallel()
	input := applicationRequirementCandidateScopeRelationFixture(t)
	job, err := NewApplicationRequirementCandidateScopeRelationJob(input)
	if err != nil {
		t.Fatal(err)
	}
	if err := job.Validate(); err != nil {
		t.Fatal(err)
	}
	prompt, err := RenderPortableJob(job)
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		input.UserRequest,
		input.Context.Facts[0].Value,
		input.Candidate,
		"Apply a normal classification threshold",
		"necessary or ordinary useful consequence",
		"ordinary objective or repository-justified useful consequence",
		"cohesive and aligned possible work",
		"conflicts with an explicit prohibition or established fact",
		"A.",
		"B.",
		"C.",
		"Answer with A or B or C.",
	} {
		if !strings.Contains(prompt, required) {
			t.Fatalf("scope relation prompt omitted %q:\n%s", required, prompt)
		}
	}
	for _, forbidden := range []string{
		ApplicationRequirementCandidateScopeReasonableDerivation,
		ApplicationRequirementCandidateScopeSpeculativeReview,
		ApplicationRequirementCandidateScopeConcreteConflict,
		ApplicationRequirementCandidateScopeRelationSchemaV1,
		ApplicationRequirementCandidateNotEntailed,
		ApplicationRequirementCandidateAuthorizationSchemaV1,
		input.Authorization.AuthoritySHA256,
		input.Context.Schema,
		input.Context.RequestSHA256,
		input.Context.Facts[0].ID,
		input.Context.Facts[0].NeedID,
		input.Context.Facts[0].SourceID,
		input.Context.Facts[0].SourceSHA256,
		"scope_mode",
	} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("scope relation prompt exposed code-owned value %q:\n%s", forbidden, prompt)
		}
	}

	framing, err := PortableResponseFramingForJob(job)
	if err != nil || framing != PortableResponseFramingSingleLine {
		t.Fatalf("scope relation response framing=%q error=%v", framing, err)
	}
	maximum, err := PortableResponseMaximumBytesForJob(job)
	if err != nil || maximum != 1 {
		t.Fatalf("scope relation response maximum=%d error=%v", maximum, err)
	}
	if _, err := SemanticUncertaintyContractForWorkKind(job.Kind); err != nil {
		t.Fatalf("scope relation semantic uncertainty contract: %v", err)
	}

	for raw, want := range map[string]string{
		"A": ApplicationRequirementCandidateScopeReasonableDerivation,
		"B": ApplicationRequirementCandidateScopeSpeculativeReview,
		"C": ApplicationRequirementCandidateScopeConcreteConflict,
	} {
		result, err := DecodeApplicationRequirementCandidateScopeRelationResult(input, raw)
		if err != nil {
			t.Fatalf("decode %q: %v", raw, err)
		}
		if result.Relation != want || result.AuthoritySHA256 == "" {
			t.Fatalf("decoded %q scope relation=%+v", raw, result)
		}
		if err := result.ValidateFor(input); err != nil {
			t.Fatal(err)
		}
		rawResult, err := json.Marshal(result)
		if err != nil {
			t.Fatal(err)
		}
		for _, forbiddenField := range []string{"accepted", "rejected", "decision", "policy", "scope_mode"} {
			if strings.Contains(string(rawResult), forbiddenField) {
				t.Fatalf("scope annotation acquired authority field %q: %s", forbiddenField, rawResult)
			}
		}
	}
}

func TestApplicationRequirementCandidateScopeRelationModeChangesOnlyClassificationThreshold(t *testing.T) {
	t.Parallel()
	fixture := applicationRequirementCandidateScopeRelationFixture(t)
	tests := []struct {
		mode      model.CodingScopeMode
		required  []string
		forbidden []string
	}{
		{
			mode: model.CodingScopeModeStrict,
			required: []string{
				"Apply a strict classification threshold",
				"only when it is necessary to fulfill the request",
			},
			forbidden: []string{
				"Apply a normal classification threshold",
				"Apply an expansive classification threshold",
			},
		},
		{
			mode: model.CodingScopeModeNormal,
			required: []string{
				"Apply a normal classification threshold",
				"necessary or ordinary useful consequence",
			},
			forbidden: []string{
				"Apply a strict classification threshold",
				"Apply an expansive classification threshold",
			},
		},
		{
			mode: model.CodingScopeModeExpansive,
			required: []string{
				"Apply an expansive classification threshold",
				"cohesive, useful, objective-aligned possibility",
			},
			forbidden: []string{
				"Apply a strict classification threshold",
				"Apply a normal classification threshold",
			},
		},
	}
	for _, test := range tests {
		test := test
		t.Run(string(test.mode), func(t *testing.T) {
			t.Parallel()
			input := fixture
			input.ScopeMode = test.mode
			job, err := NewApplicationRequirementCandidateScopeRelationJob(input)
			if err != nil {
				t.Fatal(err)
			}
			prompt, err := RenderPortableJob(job)
			if err != nil {
				t.Fatal(err)
			}
			for _, required := range test.required {
				if !strings.Contains(prompt, required) {
					t.Fatalf("%s prompt omitted %q:\n%s", test.mode, required, prompt)
				}
			}
			for _, forbidden := range test.forbidden {
				if strings.Contains(prompt, forbidden) {
					t.Fatalf("%s prompt included another threshold %q:\n%s", test.mode, forbidden, prompt)
				}
			}
			if strings.Contains(prompt, "scope_mode") {
				t.Fatalf("%s prompt exposed code-owned field name:\n%s", test.mode, prompt)
			}
		})
	}
}

func TestApplicationRequirementCandidateScopeRelationAuthorityBindsScopeMode(t *testing.T) {
	t.Parallel()
	input := applicationRequirementCandidateScopeRelationFixture(t)
	result, err := DecodeApplicationRequirementCandidateScopeRelationResult(input, "A")
	if err != nil {
		t.Fatal(err)
	}

	drifted := input
	drifted.ScopeMode = model.CodingScopeModeExpansive
	if err := result.ValidateFor(drifted); err == nil {
		t.Fatal("scope relation result validated against a different code-owned scope mode")
	}

	invalid := input
	invalid.ScopeMode = model.CodingScopeMode("wide")
	if _, err := NewApplicationRequirementCandidateScopeRelationJob(invalid); err == nil {
		t.Fatal("scope relation accepted an invalid code-owned scope mode")
	}
}

func TestApplicationRequirementCandidateScopeRelationRequiresExactNotEntailedAuthority(t *testing.T) {
	t.Parallel()
	input := applicationRequirementCandidateScopeRelationFixture(t)

	entailedInput := ApplicationRequirementCandidateAuthorizationInput{
		UserRequest: input.UserRequest,
		Context:     input.Context,
		Candidate:   input.Candidate,
	}
	entailed, err := DecodeApplicationRequirementCandidateAuthorizationResult(entailedInput, "A")
	if err != nil {
		t.Fatal(err)
	}
	withEntailed := input
	withEntailed.Authorization = entailed
	if _, err := NewApplicationRequirementCandidateScopeRelationJob(withEntailed); err == nil {
		t.Fatal("scope relation accepted an entailed candidate")
	}

	driftedCandidate := input
	driftedCandidate.Candidate = "The software exports all notes to a remote service."
	if _, err := NewApplicationRequirementCandidateScopeRelationJob(driftedCandidate); err == nil {
		t.Fatal("scope relation accepted authorization bound to another candidate")
	}

	driftedAuthorization := input
	driftedAuthorization.Authorization.AuthoritySHA256 = strings.Repeat("0", 64)
	if _, err := NewApplicationRequirementCandidateScopeRelationJob(driftedAuthorization); err == nil {
		t.Fatal("scope relation accepted drifted authorization authority")
	}

	result, err := DecodeApplicationRequirementCandidateScopeRelationResult(input, "A")
	if err != nil {
		t.Fatal(err)
	}
	mutations := map[string]func(*ApplicationRequirementCandidateScopeRelationResult){
		"schema": func(value *ApplicationRequirementCandidateScopeRelationResult) {
			value.Schema = "invalid"
		},
		"authority": func(value *ApplicationRequirementCandidateScopeRelationResult) {
			value.AuthoritySHA256 = strings.Repeat("0", 64)
		},
		"relation": func(value *ApplicationRequirementCandidateScopeRelationResult) {
			value.Relation = "UNKNOWN"
		},
	}
	for name, mutate := range mutations {
		name, mutate := name, mutate
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			changed := result
			mutate(&changed)
			if err := changed.ValidateFor(input); err == nil {
				t.Fatal("mutated scope relation validated")
			}
		})
	}
}

func applicationRequirementCandidateScopeRelationFixture(
	t testing.TB,
) ApplicationRequirementCandidateScopeRelationInput {
	t.Helper()
	const request = "Build software that records submitted notes."
	const fact = "The repository already contains a persistent note store."
	context := ApplicationContext{
		Schema:        ApplicationContextSchemaV1,
		RequestSHA256: ExactObjectiveContextSHA(request),
		Facts: []ApplicationContextFact{{
			ID:           "fact_001",
			Kind:         ApplicationContextRepositoryFact,
			Authority:    ApplicationContextEvidenceAuthority,
			NeedID:       "need_001",
			Value:        fact,
			SourceID:     "repository-snapshot",
			SourceSHA256: ExactObjectiveContextSHA(fact),
		}},
	}
	if err := context.Validate(); err != nil {
		t.Fatal(err)
	}
	const candidate = "The software provides a searchable history of stored notes."
	authorizationInput := ApplicationRequirementCandidateAuthorizationInput{
		UserRequest: request,
		Context:     context,
		Candidate:   candidate,
	}
	authorization, err := DecodeApplicationRequirementCandidateAuthorizationResult(
		authorizationInput,
		"B",
	)
	if err != nil {
		t.Fatal(err)
	}
	return ApplicationRequirementCandidateScopeRelationInput{
		UserRequest:   request,
		Context:       context,
		Candidate:     candidate,
		Authorization: authorization,
		ScopeMode:     model.CodingScopeModeNormal,
	}
}
