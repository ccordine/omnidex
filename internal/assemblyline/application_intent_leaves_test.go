package assemblyline

import (
	"strings"
	"testing"
)

func TestApplicationIntentLeavesReturnOneRawValueAtATime(t *testing.T) {
	authority := applicationIntentLeafFixture(t)
	productInput := ApplicationProductContextInput{
		UserRequest: authority.UserRequest, Context: authority.Context,
	}
	prompt, err := BuildApplicationProductContextPrompt(productInput)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prompt, "one semantic question") ||
		!strings.Contains(prompt, "Return only one concise product or domain identity phrase") ||
		!strings.Contains(prompt, "JSON") {
		t.Fatalf("product-context prompt is not one raw leaf: %s", prompt)
	}
	product, err := DecodeApplicationProductContextLeaf(
		productInput, "A browser counter for tracking a current value.",
	)
	if err != nil {
		t.Fatal(err)
	}
	if product != "A browser counter for tracking a current value." {
		t.Fatalf("product=%q", product)
	}
	for _, wrapped := range []string{
		`"A browser counter."`,
		`{"product_context":"A browser counter."}`,
	} {
		if _, err := DecodeApplicationProductContextLeaf(productInput, wrapped); err == nil {
			t.Fatalf("accepted wrapped product context %q", wrapped)
		}
	}
}

func TestApplicationRequirementLeavesSeparateCoverageFromGeneration(t *testing.T) {
	authority := applicationIntentLeafFixture(t)
	coverageInput := ApplicationRequirementCoverageInput{
		UserRequest: authority.UserRequest, Context: authority.Context,
		AcceptedRequirements: []string{},
	}
	coveragePrompt, err := BuildApplicationRequirementCoveragePrompt(coverageInput)
	if err != nil {
		t.Fatal(err)
	}
	coverage, err := DecodeApplicationRequirementCoverageLeaf(
		coverageInput, ApplicationRequirementRemains,
	)
	if err != nil {
		t.Fatal(err)
	}
	candidateInput := ApplicationRequirementCandidateInput{
		Authority: coverageInput, Coverage: coverage,
	}
	requirementPrompt, err := BuildApplicationRequirementPrompt(candidateInput)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(coveragePrompt, "ACCEPTED REQUIREMENTS:\n(none)") ||
		!strings.Contains(requirementPrompt, "ACCEPTED REQUIREMENTS:\n(none)") {
		t.Fatalf("empty accepted requirement projection was not explicit")
	}
	for _, required := range []string{
		"task-local runtime implementation requirement",
		"independently testable runtime outcome",
		"Items joined by commas, conjunctions, or a list remain separate outcomes",
		"generic test obligations",
		"build or verification obligations",
		"deployment and continued-availability obligations",
		"Does even one independently testable included runtime outcome remain uncovered?",
	} {
		if !strings.Contains(coveragePrompt, required) {
			t.Fatalf("coverage prompt omitted boundary %q:\n%s", required, coveragePrompt)
		}
	}
	for _, required := range []string{
		"exactly one independently testable runtime outcome",
		"return only the first uncovered outcome",
		"Never return an umbrella construction statement, a list, or multiple actions",
		"generic test obligations",
		"build or verification obligations",
		"deployment and continued-availability obligations",
		"What is the one earliest uncovered independently testable runtime outcome?",
	} {
		if !strings.Contains(requirementPrompt, required) {
			t.Fatalf("generation prompt omitted boundary %q:\n%s", required, requirementPrompt)
		}
	}
	if strings.Contains(coveragePrompt, "Return only the requirement as raw prose") ||
		strings.Contains(requirementPrompt, "NO_UNCOVERED_REQUIREMENT") {
		t.Fatalf("coverage and generation responsibilities were combined")
	}
	if coverage.Relation != ApplicationRequirementRemains ||
		!strings.Contains(requirementPrompt, "CODE-ESTABLISHED UNCOVERED RELATION:\n"+ApplicationRequirementRemains) {
		t.Fatalf("coverage=%+v prompt=%s", coverage, requirementPrompt)
	}
	requirement, err := DecodeApplicationRequirementLeaf(
		candidateInput, "The user can increment the current count.",
	)
	if err != nil {
		t.Fatal(err)
	}
	coverageInput.AcceptedRequirements = []string{requirement}
	candidateInput = applicationRequirementCandidateFixture(t, coverageInput)
	if _, err := DecodeApplicationRequirementLeaf(candidateInput, requirement); err == nil ||
		!strings.Contains(err.Error(), "duplicates") {
		t.Fatalf("duplicate requirement error=%v", err)
	}
}

func TestApplicationRequirementCandidateRequiresExactBoundCoverageAuthority(t *testing.T) {
	t.Parallel()
	authority := applicationIntentLeafFixture(t)
	coverageInput := ApplicationRequirementCoverageInput{
		UserRequest: authority.UserRequest, Context: authority.Context,
		AcceptedRequirements: []string{},
	}
	remains, err := DecodeApplicationRequirementCoverageLeaf(
		coverageInput, ApplicationRequirementRemains,
	)
	if err != nil {
		t.Fatal(err)
	}
	valid := ApplicationRequirementCandidateInput{
		Authority: coverageInput, Coverage: remains,
	}
	if _, err := NewApplicationRequirementJob(valid); err != nil {
		t.Fatal(err)
	}

	mutations := map[string]func(*ApplicationRequirementCandidateInput){
		"schema": func(input *ApplicationRequirementCandidateInput) {
			input.Coverage.Schema = "invalid"
		},
		"authority hash": func(input *ApplicationRequirementCandidateInput) {
			input.Coverage.AuthoritySHA256 = strings.Repeat("0", 64)
		},
		"accepted set": func(input *ApplicationRequirementCandidateInput) {
			input.Authority.AcceptedRequirements = []string{"Display the current count."}
		},
		"unregistered relation": func(input *ApplicationRequirementCandidateInput) {
			input.Coverage.Relation = "UNKNOWN"
		},
	}
	for name, mutate := range mutations {
		name, mutate := name, mutate
		t.Run(name, func(t *testing.T) {
			candidate := valid
			candidate.Authority.AcceptedRequirements = append(
				[]string(nil), valid.Authority.AcceptedRequirements...,
			)
			mutate(&candidate)
			if _, err := NewApplicationRequirementJob(candidate); err == nil {
				t.Fatal("invalid coverage authority opened a generation job")
			}
		})
	}

	none, err := DecodeApplicationRequirementCoverageLeaf(
		coverageInput, ApplicationNoUncoveredRequirement,
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewApplicationRequirementJob(ApplicationRequirementCandidateInput{
		Authority: coverageInput, Coverage: none,
	}); err == nil || !strings.Contains(err.Error(), ApplicationRequirementRemains) {
		t.Fatalf("NO_UNCOVERED_REQUIREMENT generation error=%v", err)
	}
}

func TestApplicationRequirementPayloadSchemasAreRendererExact(t *testing.T) {
	t.Parallel()
	authority := applicationIntentLeafFixture(t)
	coverageInput := ApplicationRequirementCoverageInput{
		UserRequest: authority.UserRequest, Context: authority.Context,
		AcceptedRequirements: []string{},
	}
	currentCoverage, err := NewApplicationRequirementCoverageJob(coverageInput)
	if err != nil {
		t.Fatal(err)
	}
	currentGeneration, err := NewApplicationRequirementJob(
		applicationRequirementCandidateFixture(t, coverageInput),
	)
	if err != nil {
		t.Fatal(err)
	}
	legacy := applicationRequirementLeafInputV1{
		UserRequest: authority.UserRequest, Context: authority.Context,
		ProductContext: "A browser counter.", AcceptedRequirements: []string{},
	}
	historicalCoverage, err := newPortableJob(WorkApplicationRequirementCoverage, legacy)
	if err != nil {
		t.Fatal(err)
	}
	historicalGeneration, err := newPortableJob(WorkApplicationRequirement, legacy)
	if err != nil {
		t.Fatal(err)
	}

	for _, job := range []PortableJob{currentCoverage, currentGeneration} {
		if err := ValidatePortableJobForRenderer(job, PortableRendererV8); err != nil {
			t.Fatal(err)
		}
		for _, renderer := range []string{
			HistoricalPortableRendererV7,
			HistoricalPortableRendererV6,
			HistoricalPortableRendererV5,
		} {
			if err := ValidatePortableJobForRenderer(job, renderer); err == nil {
				t.Fatalf("current %s payload validated as %s", job.Kind, renderer)
			}
		}
	}
	for _, job := range []PortableJob{historicalCoverage, historicalGeneration} {
		if err := ValidatePortableJobForRenderer(job, PortableRendererV8); err == nil {
			t.Fatalf("historical %s payload validated as V8", job.Kind)
		}
		for _, renderer := range []string{
			HistoricalPortableRendererV7,
			HistoricalPortableRendererV6,
			HistoricalPortableRendererV5,
		} {
			if err := ValidatePortableJobForRenderer(job, renderer); err != nil {
				t.Fatalf("historical %s payload rejected for %s: %v", job.Kind, renderer, err)
			}
		}
	}
}

func TestApplicationRequirementFixedPointExcludesGlobalConstraintsButKeepsRuntimeFormats(t *testing.T) {
	t.Parallel()
	request := "Build a responsive command-line report that exports CSV using Rust in one source file, with focused tests, a production build, and continued availability."
	context, err := BootstrapApplicationContext(request, ApplicationWorkspaceEmpty)
	if err != nil {
		t.Fatal(err)
	}
	coverageInput := ApplicationRequirementCoverageInput{
		UserRequest: request, Context: context,
		AcceptedRequirements: []string{"Export reports as CSV."},
	}
	candidateInput := applicationRequirementCandidateFixture(t, coverageInput)
	prompts := map[string]struct {
		build func() (string, error)
	}{
		"coverage": {build: func() (string, error) {
			return BuildApplicationRequirementCoveragePrompt(coverageInput)
		}},
		"generation": {build: func() (string, error) {
			return BuildApplicationRequirementPrompt(candidateInput)
		}},
	}
	for label, fixture := range prompts {
		prompt, promptErr := fixture.build()
		if promptErr != nil {
			t.Fatal(promptErr)
		}
		for _, required := range []string{
			"exporting CSV is a runtime behavior",
			"using Rust, React, Jest, or a single-file project",
			"generic test obligations",
			"build or verification obligations",
			"deployment and continued-availability obligations",
		} {
			if !strings.Contains(prompt, required) {
				t.Fatalf("%s prompt omitted %q:\n%s", label, required, prompt)
			}
		}
	}
}

func TestApplicationRequirementPromptsDoNotLetUmbrellaContextHideDistinctRequirements(t *testing.T) {
	t.Parallel()
	fixtures := []struct {
		name     string
		request  string
		product  string
		accepted string
	}{
		{
			name:     "browser inventory",
			request:  "Build a browser inventory board with keyboard navigation, low-stock filters, and a production build.",
			product:  "A browser inventory board with operational controls.",
			accepted: "Build a browser inventory board.",
		},
		{
			name:     "command line report",
			request:  "Build a command-line report exporter with date filtering, CSV output, useful invalid-filter errors, and focused tests.",
			product:  "A command-line reporting utility.",
			accepted: "Build a command-line report exporter.",
		},
	}
	for _, fixture := range fixtures {
		fixture := fixture
		t.Run(fixture.name, func(t *testing.T) {
			t.Parallel()
			context, err := BootstrapApplicationContext(
				fixture.request, ApplicationWorkspaceEmpty,
			)
			if err != nil {
				t.Fatal(err)
			}
			coverageInput := ApplicationRequirementCoverageInput{
				UserRequest: fixture.request, Context: context,
				AcceptedRequirements: []string{fixture.accepted},
			}
			coverage, err := BuildApplicationRequirementCoveragePrompt(coverageInput)
			if err != nil {
				t.Fatal(err)
			}
			generation, err := BuildApplicationRequirementPrompt(
				applicationRequirementCandidateFixture(t, coverageInput),
			)
			if err != nil {
				t.Fatal(err)
			}
			for label, prompt := range map[string]string{
				"coverage":   coverage,
				"generation": generation,
			} {
				for _, required := range []string{"independently testable", fixture.accepted} {
					if !strings.Contains(prompt, required) {
						t.Fatalf("%s prompt omitted %q:\n%s", label, required, prompt)
					}
				}
				if strings.Contains(prompt, fixture.product) ||
					strings.Contains(prompt, "PRODUCT CONTEXT:") {
					t.Fatalf("%s prompt received redundant product context:\n%s", label, prompt)
				}
			}
		})
	}
}

func applicationRequirementCandidateFixture(
	t *testing.T,
	authority ApplicationRequirementCoverageInput,
) ApplicationRequirementCandidateInput {
	t.Helper()
	coverage, err := DecodeApplicationRequirementCoverageLeaf(
		authority, ApplicationRequirementRemains,
	)
	if err != nil {
		t.Fatal(err)
	}
	return ApplicationRequirementCandidateInput{
		Authority: authority,
		Coverage:  coverage,
	}
}

func TestApplicationProductContextPromptExcludesRequirementsFromItsResponsibility(t *testing.T) {
	t.Parallel()
	authority := applicationIntentLeafFixture(t)
	prompt, err := BuildApplicationProductContextPrompt(ApplicationProductContextInput{
		UserRequest: authority.UserRequest,
		Context:     authority.Context,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		"product or domain identity",
		"Exclude requested qualities, capabilities, behaviors",
		"tests, build or deployment constraints",
	} {
		if !strings.Contains(prompt, required) {
			t.Fatalf("product-context prompt omitted %q:\n%s", required, prompt)
		}
	}
}

func applicationIntentLeafFixture(t *testing.T) ApplicationIntentInput {
	t.Helper()
	request := "Build a browser counter that displays and increments a count."
	context, err := BootstrapApplicationContext(
		request, ApplicationWorkspaceEmpty,
	)
	if err != nil {
		t.Fatal(err)
	}
	return ApplicationIntentInput{
		UserRequest: request,
		Context:     context,
	}
}
