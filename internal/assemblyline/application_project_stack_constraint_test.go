package assemblyline

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestApplicationProjectStackConstraintIsOneOpaqueBoundedChoice(t *testing.T) {
	input := ApplicationProjectStackConstraintInput{
		UserRequest: "Build an interactive inventory console using TypeScript and React.",
		Candidates: []ApplicationProjectStackCandidate{
			{CandidateID: "STACK_CANDIDATE_1", TechnicalFormat: "TypeScript with React for a browser application; packaging shape: one workload source and one browser verification"},
			{CandidateID: "STACK_CANDIDATE_2", TechnicalFormat: "JavaScript with server-rendered HTML for a browser application; packaging shape: one server source and one verification"},
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
		input.UserRequest, "packaging shape",
	} {
		if !strings.Contains(prompt, required) {
			t.Fatalf("portable stack constraint omitted %q", required)
		}
	}
	if strings.Contains(prompt, "package.json") || strings.Contains(prompt, "src/") ||
		strings.Contains(prompt, "product_context") || strings.Contains(prompt, "accepted_requirements") {
		t.Fatalf("project stack constraint exposed physical project identity: %s", prompt)
	}
	for _, selected := range []string{
		"STACK_CANDIDATE_1", ApplicationProjectStackUnconstrained, ApplicationProjectStackUnsupported,
	} {
		decision := ApplicationProjectStackConstraintDecision{
			Schema: ApplicationProjectStackConstraintSchemaV2, CandidateID: selected,
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
		UserRequest: "Build a command-line report that prints one report.",
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
		Schema: ApplicationProjectStackConstraintSchemaV2, CandidateID: "STACK_CANDIDATE_2",
	}
	if err := decision.ValidateFor(base); err == nil {
		t.Fatal("accepted unavailable project stack candidate")
	}
}

func TestApplicationProjectStackConstraintPayloadIsRendererVersioned(t *testing.T) {
	legacy := applicationProjectStackConstraintInputV1{
		ProductContext:       "interactive inventory console",
		AcceptedRequirements: []string{"Use TypeScript and React", "Show current inventory"},
		Candidates: []ApplicationProjectStackCandidate{{
			CandidateID:     "STACK_CANDIDATE_1",
			TechnicalFormat: "TypeScript with React for a browser application",
		}},
	}
	historicalJob, err := newPortableJob(WorkApplicationProjectStackConstraint, legacy)
	if err != nil {
		t.Fatal(err)
	}
	for _, renderer := range []string{
		HistoricalPortableRendererV5, HistoricalPortableRendererV6,
	} {
		if err := ValidatePortableJobForRenderer(historicalJob, renderer); err != nil {
			t.Fatalf("historical %s input: %v", renderer, err)
		}
	}
	for _, renderer := range []string{PortableRendererV8, HistoricalPortableRendererV7} {
		if err := ValidatePortableJobForRenderer(historicalJob, renderer); err == nil {
			t.Fatalf("V2 renderer %s accepted historical stack authority", renderer)
		}
	}
	if _, err := RenderPortableJob(historicalJob); err == nil {
		t.Fatal("current renderer reconstructed a historical stack prompt")
	}
	legacyDecision, err := DecodeApplicationProjectStackConstraintDecisionForPortableRenderer(
		historicalJob.Payload, HistoricalPortableRendererV6, "STACK_CANDIDATE_1",
	)
	if err != nil || legacyDecision.Schema != ApplicationProjectStackConstraintSchemaV1 {
		t.Fatalf("legacy decision=%+v error=%v", legacyDecision, err)
	}

	current := ApplicationProjectStackConstraintInput{
		UserRequest: "Build the inventory console using TypeScript and React.",
		Candidates:  legacy.Candidates,
	}
	currentJob, err := NewApplicationProjectStackConstraintJob(current)
	if err != nil {
		t.Fatal(err)
	}
	for _, renderer := range []string{PortableRendererV8, HistoricalPortableRendererV7} {
		if err := ValidatePortableJobForRenderer(currentJob, renderer); err != nil {
			t.Fatalf("V2 renderer %s: %v", renderer, err)
		}
	}
	for _, renderer := range []string{
		HistoricalPortableRendererV5, HistoricalPortableRendererV6,
	} {
		if err := ValidatePortableJobForRenderer(currentJob, renderer); err == nil {
			t.Fatalf("historical renderer %s accepted current stack authority", renderer)
		}
	}
}

func TestApplicationProjectStackConstraintPreservesHistoricalCandidateBound(t *testing.T) {
	longFormat := strings.Repeat("x", maxApplicationProjectStackSummaryBytesV1+1)
	legacy := applicationProjectStackConstraintInputV1{
		ProductContext:       "bounded historical application",
		AcceptedRequirements: []string{"Use the registered technical format"},
		Candidates: []ApplicationProjectStackCandidate{{
			CandidateID: "STACK_CANDIDATE_1", TechnicalFormat: longFormat,
		}},
	}
	payload, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	historicalJob := PortableJob{
		Schema: PortableJobSchemaV2, Kind: WorkApplicationProjectStackConstraint,
		Payload: payload,
	}
	historicalJob.ID = portableJobDigest(
		historicalJob.Schema, historicalJob.Kind, historicalJob.Payload,
	)
	for _, renderer := range []string{
		HistoricalPortableRendererV5, HistoricalPortableRendererV6,
	} {
		err := ValidatePortableJobForRenderer(historicalJob, renderer)
		if err == nil || !strings.Contains(err.Error(), "exceeds 1024 bytes") {
			t.Fatalf("historical renderer %s candidate bound error=%v", renderer, err)
		}
	}

	currentJob, err := NewApplicationProjectStackConstraintJob(
		ApplicationProjectStackConstraintInput{
			UserRequest: "Build an application using the registered technical format.",
			Candidates: []ApplicationProjectStackCandidate{{
				CandidateID: "STACK_CANDIDATE_1", TechnicalFormat: longFormat,
			}},
		},
	)
	if err != nil {
		t.Fatalf("current renderer rejected bounded V2 candidate: %v", err)
	}
	if err := ValidatePortableJobForRenderer(currentJob, PortableRendererV8); err != nil {
		t.Fatal(err)
	}
}
