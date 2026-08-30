package assemblyline

import (
	"strings"
	"testing"
)

func TestApplicationRequirementCandidateAuthorizationIsOneEntailmentRelation(t *testing.T) {
	t.Parallel()
	input := applicationRequirementCandidateAuthorizationFixture(t)
	job, err := NewApplicationRequirementCandidateAuthorizationJob(input)
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
		input.Candidate,
		"one semantic entailment question",
		"Entailment is semantic, not textual identity",
		"A direct imperative that states an operation over runtime input",
		"The finished software processes each supplied item",
		"Expressing the purpose noun as its corresponding action",
		"neutral runtime subject adds no meaning",
		"customary controls, variants, history, persistence, presentation, process steps, triggers, or enhancements",
		"construction authority only",
		"does not entail that the finished software renders an interface",
		"unstated mechanism, interface, device or input source, algorithm, mode",
		"An entailed core does not excuse one added detail",
		ApplicationRequirementCandidateEntailed,
		ApplicationRequirementCandidateNotEntailed,
	} {
		if !strings.Contains(prompt, required) {
			t.Fatalf("authorization prompt omitted %q:\n%s", required, prompt)
		}
	}
	for _, forbidden := range []string{
		"quality review", "completeness review", "workflow decision",
		"authorization runs first", "later user turn", "downstream", "task queue",
		"kind or cardinality", "unclassified",
	} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("authorization prompt exposed orchestration language %q:\n%s", forbidden, prompt)
		}
	}
	for _, relation := range []string{
		ApplicationRequirementCandidateEntailed,
		ApplicationRequirementCandidateNotEntailed,
	} {
		result, err := DecodeApplicationRequirementCandidateAuthorizationResult(input, relation)
		if err != nil {
			t.Fatal(err)
		}
		if result.Relation != relation || result.AuthoritySHA256 == "" {
			t.Fatalf("authorization=%+v", result)
		}
		if err := result.ValidateFor(input); err != nil {
			t.Fatal(err)
		}
	}
	if maximum, err := PortableResponseMaximumBytesForJob(job); err != nil {
		t.Fatal(err)
	} else if maximum != len(ApplicationRequirementCandidateNotEntailed) {
		t.Fatalf("authorization response maximum=%d", maximum)
	}
}

func TestApplicationRequirementCandidateAuthorizationReceiptRejectsAuthorityDrift(t *testing.T) {
	t.Parallel()
	input := applicationRequirementCandidateAuthorizationFixture(t)
	result, err := DecodeApplicationRequirementCandidateAuthorizationResult(
		input,
		ApplicationRequirementCandidateEntailed,
	)
	if err != nil {
		t.Fatal(err)
	}
	mutations := map[string]func(*ApplicationRequirementCandidateAuthorizationResult){
		"schema": func(value *ApplicationRequirementCandidateAuthorizationResult) {
			value.Schema = "invalid"
		},
		"authority": func(value *ApplicationRequirementCandidateAuthorizationResult) {
			value.AuthoritySHA256 = strings.Repeat("0", 64)
		},
		"relation": func(value *ApplicationRequirementCandidateAuthorizationResult) {
			value.Relation = "UNKNOWN"
		},
	}
	for name, mutate := range mutations {
		name, mutate := name, mutate
		t.Run(name, func(t *testing.T) {
			changed := result
			mutate(&changed)
			if err := changed.ValidateFor(input); err == nil {
				t.Fatal("mutated authorization receipt validated")
			}
		})
	}
	changedInput := input
	changedInput.Candidate = "The application stores prior values."
	if err := result.ValidateFor(changedInput); err == nil {
		t.Fatal("authorization receipt validated for another candidate")
	}
}

func TestExactSourceApplicationRequirementCandidateAuthorizationIsMechanical(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name    string
		request string
	}{
		{name: "display", request: "Display the submitted note."},
		{name: "persistence", request: "Persist the submitted preference."},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			context, err := BootstrapApplicationContext(
				test.request,
				ApplicationWorkspaceEmpty,
			)
			if err != nil {
				t.Fatal(err)
			}
			input := ApplicationRequirementCandidateAuthorizationInput{
				UserRequest: test.request,
				Context:     context,
				Candidate:   test.request,
			}
			result, resolved, err := ResolveExactSourceApplicationRequirementCandidateAuthorization(input)
			if err != nil {
				t.Fatal(err)
			}
			if !resolved || result.Relation != ApplicationRequirementCandidateEntailed {
				t.Fatalf("resolved=%t authorization=%+v", resolved, result)
			}
			if err := result.ValidateFor(input); err != nil {
				t.Fatal(err)
			}

			changed := input
			changed.Candidate = strings.TrimSuffix(test.request, ".")
			unresolved, resolved, err := ResolveExactSourceApplicationRequirementCandidateAuthorization(changed)
			if err != nil {
				t.Fatal(err)
			}
			if resolved || unresolved != (ApplicationRequirementCandidateAuthorizationResult{}) {
				t.Fatalf("byte-different candidate resolved=%t authorization=%+v", resolved, unresolved)
			}
		})
	}
}

func applicationRequirementCandidateAuthorizationFixture(
	t *testing.T,
) ApplicationRequirementCandidateAuthorizationInput {
	t.Helper()
	request := "Build a browser transformer that displays the reversed input text."
	context, err := BootstrapApplicationContext(request, ApplicationWorkspaceEmpty)
	if err != nil {
		t.Fatal(err)
	}
	return ApplicationRequirementCandidateAuthorizationInput{
		UserRequest: request,
		Context:     context,
		Candidate:   "The application displays the reversed input text.",
	}
}
