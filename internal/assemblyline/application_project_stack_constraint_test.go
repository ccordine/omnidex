package assemblyline

import (
	"strings"
	"testing"
)

func TestApplicationProjectStackConstraintIsOneOpaqueBoundedChoice(t *testing.T) {
	input := ApplicationProjectStackConstraintInput{
		ProductContext:       "interactive inventory console",
		AcceptedRequirements: []string{"Use TypeScript and React", "Show current inventory"},
		Candidates: []ApplicationProjectStackCandidate{
			{CandidateID: "STACK_CANDIDATE_1", TechnicalFormat: "TypeScript with React for a browser application"},
			{CandidateID: "STACK_CANDIDATE_2", TechnicalFormat: "JavaScript with server-rendered HTML for a browser application"},
		},
	}
	job, err := NewApplicationProjectStackConstraintJob(input)
	if err != nil {
		t.Fatal(err)
	}
	prompt, err := RenderPortableJob(job)
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		"STACK_CANDIDATE_1", "STACK_CANDIDATE_2", "UNCONSTRAINED", "UNSUPPORTED",
		"Use TypeScript and React",
	} {
		if !strings.Contains(prompt, required) {
			t.Fatalf("portable stack constraint omitted %q", required)
		}
	}
	if strings.Contains(prompt, "package.json") || strings.Contains(prompt, "src/") {
		t.Fatalf("project stack constraint exposed physical project identity: %s", prompt)
	}
	for _, selected := range []string{
		"STACK_CANDIDATE_1", ApplicationProjectStackUnconstrained, ApplicationProjectStackUnsupported,
	} {
		decision := ApplicationProjectStackConstraintDecision{
			Schema: ApplicationProjectStackConstraintSchemaV1, CandidateID: selected,
		}
		if err := decision.ValidateFor(input); err != nil {
			t.Fatalf("decision %q: %v", selected, err)
		}
		decoded, err := DecodeApplicationProjectStackConstraintDecision(input, selected)
		if err != nil || decoded != decision {
			t.Fatalf("decoded=%+v want=%+v err=%v", decoded, decision, err)
		}
	}
	if _, err := DecodeApplicationProjectStackConstraintDecision(
		input, `{"candidate_id":"STACK_CANDIDATE_1"}`,
	); err == nil {
		t.Fatal("accepted JSON wrapper")
	}
}

func TestApplicationProjectStackConstraintRejectsUnownedOrAmbiguousValues(t *testing.T) {
	base := ApplicationProjectStackConstraintInput{
		ProductContext:       "command line report",
		AcceptedRequirements: []string{"Print a report"},
		Candidates: []ApplicationProjectStackCandidate{{
			CandidateID: "STACK_CANDIDATE_1", TechnicalFormat: "Go for a command-line application",
		}},
	}
	for name, mutate := range map[string]func(*ApplicationProjectStackConstraintInput){
		"non-opaque ID": func(input *ApplicationProjectStackConstraintInput) {
			input.Candidates[0].CandidateID = "go"
		},
		"repeated format": func(input *ApplicationProjectStackConstraintInput) {
			input.Candidates = append(input.Candidates, ApplicationProjectStackCandidate{
				CandidateID: "STACK_CANDIDATE_2", TechnicalFormat: input.Candidates[0].TechnicalFormat,
			})
		},
		"multiline format": func(input *ApplicationProjectStackConstraintInput) {
			input.Candidates[0].TechnicalFormat = "Go\nwith tools"
		},
	} {
		t.Run(name, func(t *testing.T) {
			input := base
			input.Candidates = append([]ApplicationProjectStackCandidate(nil), base.Candidates...)
			mutate(&input)
			if _, err := NewApplicationProjectStackConstraintJob(input); err == nil {
				t.Fatal("accepted invalid project stack constraint")
			}
		})
	}
	decision := ApplicationProjectStackConstraintDecision{
		Schema: ApplicationProjectStackConstraintSchemaV1, CandidateID: "STACK_CANDIDATE_2",
	}
	if err := decision.ValidateFor(base); err == nil {
		t.Fatal("accepted unavailable project stack candidate")
	}
}
