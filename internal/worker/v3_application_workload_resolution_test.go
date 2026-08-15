package worker

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestApplicationWorkloadResolutionSpecifiesReviewsAndFreezesEveryAcceptedRequirement(t *testing.T) {
	t.Parallel()

	specification := workerApplicationSpecification()
	input := applicationWorkloadInput(specification)
	var kinds []assemblyline.WorkKind
	var specificationPrompts []string
	var reviewPrompts []string
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 3,
		Execute: func(job assemblyline.PortableJob, _ string) (assemblyline.PortableResult, error) {
			prompt, schema, err := assemblyline.RenderPortableJob(job)
			if err != nil {
				return assemblyline.PortableResult{}, err
			}
			kinds = append(kinds, job.Kind)
			switch job.Kind {
			case assemblyline.WorkApplicationJobSpecification:
				if err := assertJobSpecificationSchemaHasNoCodeAuthority(schema); err != nil {
					return assemblyline.PortableResult{}, err
				}
				assertFullJobSpecificationAuthority(t, prompt, specification)
				specificationPrompts = append(specificationPrompts, prompt)
				return workloadPortableCandidate(job, workerJobSpecificationCandidate(prompt)), nil
			case assemblyline.WorkApplicationJobSpecificationReview:
				if err := assertJobSpecificationReviewSchemaHasNoCodeAuthority(schema); err != nil {
					return assemblyline.PortableResult{}, err
				}
				assertFullJobSpecificationAuthority(t, prompt, specification)
				values := workerJobSpecificationValues(prompt)
				if len(values) == 0 {
					return assemblyline.PortableResult{}, fmt.Errorf("independent review omitted proposed specification")
				}
				for _, value := range values {
					if !strings.Contains(prompt, value) {
						return assemblyline.PortableResult{}, fmt.Errorf(
							"independent review omitted proposed semantic field %q", value,
						)
					}
				}
				reviewPrompts = append(reviewPrompts, prompt)
				return workloadPortableCandidate(job, `{"decision":"accept"}`), nil
			default:
				return assemblyline.PortableResult{}, fmt.Errorf("unexpected work kind %s", job.Kind)
			}
		},
	}

	frozen, err := resolveDirectCodingApplicationWorkload(runtime, "semantic", "semantic-review", input)
	if err != nil {
		t.Fatal(err)
	}
	if err := assemblyline.ValidateFrozenApplicationWorkload(input, frozen); err != nil {
		t.Fatalf("returned workload was not frozen from accepted specifications: %v", err)
	}
	wantKinds := make([]assemblyline.WorkKind, 0, len(specification.Requirements)*2)
	for range specification.Requirements {
		wantKinds = append(wantKinds,
			assemblyline.WorkApplicationJobSpecification,
			assemblyline.WorkApplicationJobSpecificationReview,
		)
	}
	if !reflect.DeepEqual(kinds, wantKinds) {
		t.Fatalf("model calls=%v want one specification plus one review per requirement: %v", kinds, wantKinds)
	}
	if len(specificationPrompts) != len(specification.Requirements) ||
		len(reviewPrompts) != len(specification.Requirements) {
		t.Fatalf(
			"specification prompts=%d review prompts=%d requirements=%d",
			len(specificationPrompts), len(reviewPrompts), len(specification.Requirements),
		)
	}
	for index, task := range frozen.Tasks {
		want := workerJobSpecificationForRequirement(specification.Requirements[index].SourceQuote)
		if task.Objective != want.Objective ||
			!reflect.DeepEqual(task.RequiredBehaviors, want.RequiredBehaviors) ||
			!reflect.DeepEqual(task.AcceptanceCriteria, want.AcceptanceCriteria) {
			t.Fatalf("frozen task %d lost its accepted executable specification: %+v", index, task)
		}
		if len(task.DependsOn) != 0 {
			t.Fatalf("semantic model invented scheduler dependencies: %+v", task)
		}
	}
}

func TestApplicationWorkloadResolutionPreservesReviewerNamedLeafRepairs(t *testing.T) {
	t.Parallel()

	specification := workerApplicationSpecification()
	specification.Requirements = specification.Requirements[:1]
	input := applicationWorkloadInput(specification)
	var kinds []assemblyline.WorkKind
	var reviews int
	var repairs int
	findings := []string{
		"The behaviors do not name the concrete grouping actions and visible results.",
		"The checks do not demonstrate the visible results of assignment and removal.",
	}
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 3,
		Execute: func(job assemblyline.PortableJob, _ string) (assemblyline.PortableResult, error) {
			prompt, schema, err := assemblyline.RenderPortableJob(job)
			if err != nil {
				return assemblyline.PortableResult{}, err
			}
			kinds = append(kinds, job.Kind)
			switch job.Kind {
			case assemblyline.WorkApplicationJobSpecification:
				return workloadPortableCandidate(job, `{
					"objective":"Implement interactive record grouping in the records console.",
					"required_behaviors":["Make grouping usable."],
					"acceptance_criteria":["Grouping works."]
				}`), nil
			case assemblyline.WorkApplicationJobSpecificationReview:
				reviews++
				switch reviews {
				case 1:
					return workloadPortableCandidate(job, fmt.Sprintf(
						`{"decision":"repair","field":"required_behaviors","finding":%q,"finding_evidence":"Make grouping usable."}`,
						findings[0],
					)), nil
				case 2:
					if !strings.Contains(prompt, "Users can create a named group and assign a visible record to it.") {
						return assemblyline.PortableResult{}, fmt.Errorf("second review did not receive retained behavior repair")
					}
					return workloadPortableCandidate(job, fmt.Sprintf(
						`{"decision":"repair","field":"acceptance_criteria","finding":%q,"finding_evidence":"Grouping works."}`,
						findings[1],
					)), nil
				default:
					if !strings.Contains(prompt, "After assignment, the record is visibly listed in the selected named group.") {
						return assemblyline.PortableResult{}, fmt.Errorf("final review did not receive retained acceptance repair")
					}
					return workloadPortableCandidate(job, `{"decision":"accept"}`), nil
				}
			case assemblyline.WorkApplicationJobSpecificationRepair:
				repairs++
				if err := assertOneFieldJobSpecificationRepairSchema(schema, repairs); err != nil {
					return assemblyline.PortableResult{}, err
				}
				assertPromptHasNoModelOwnedExecutionAuthority(t, prompt)
				for _, required := range []string{
					`"user_authority"`, `"current_derived_value"`,
					`"target_derived_field"`, `"review_finding"`, findings[repairs-1],
				} {
					if !strings.Contains(prompt, required) {
						return assemblyline.PortableResult{}, fmt.Errorf("repair omitted provenance label %s", required)
					}
				}
				switch repairs {
				case 1:
					return workloadPortableCandidate(job, `{"required_behaviors":["Users can create a named group and assign a visible record to it.","Users can remove a record from its current group."]}`), nil
				case 2:
					return workloadPortableCandidate(job, `{"acceptance_criteria":["After assignment, the record is visibly listed in the selected named group.","After removal, the record is no longer listed in that group."]}`), nil
				default:
					return assemblyline.PortableResult{}, fmt.Errorf("unexpected repair attempt %d", repairs)
				}
			default:
				return assemblyline.PortableResult{}, fmt.Errorf("unexpected work kind %s", job.Kind)
			}
		},
	}

	frozen, err := resolveDirectCodingApplicationWorkload(runtime, "semantic", "semantic-review", input)
	if err != nil {
		t.Fatal(err)
	}
	wantKinds := []assemblyline.WorkKind{
		assemblyline.WorkApplicationJobSpecification,
		assemblyline.WorkApplicationJobSpecificationReview,
		assemblyline.WorkApplicationJobSpecificationRepair,
		assemblyline.WorkApplicationJobSpecificationReview,
		assemblyline.WorkApplicationJobSpecificationRepair,
		assemblyline.WorkApplicationJobSpecificationReview,
	}
	if !reflect.DeepEqual(kinds, wantKinds) {
		t.Fatalf("calls=%v want bounded specification/review/repair sequence %v", kinds, wantKinds)
	}
	if repairs != 2 || reviews != 3 {
		t.Fatalf("repairs=%d reviews=%d", repairs, reviews)
	}
	if len(frozen.Tasks) != 1 {
		t.Fatalf("frozen tasks=%d", len(frozen.Tasks))
	}
	task := frozen.Tasks[0]
	if !reflect.DeepEqual(task.RequiredBehaviors, []string{
		"Users can create a named group and assign a visible record to it.",
		"Users can remove a record from its current group.",
	}) || !reflect.DeepEqual(task.AcceptanceCriteria, []string{
		"After assignment, the record is visibly listed in the selected named group.",
		"After removal, the record is no longer listed in that group.",
	}) {
		t.Fatalf("freeze did not retain the twice-reviewed accepted specification: %+v", task)
	}
}

func TestApplicationWorkloadResolutionRejectsInitialDefectWithoutSingleLeafAuthority(t *testing.T) {
	t.Parallel()

	specification := workerApplicationSpecification()
	specification.Requirements = specification.Requirements[:1]
	input := applicationWorkloadInput(specification)
	var kinds []assemblyline.WorkKind
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 3,
		Execute: func(job assemblyline.PortableJob, _ string) (assemblyline.PortableResult, error) {
			kinds = append(kinds, job.Kind)
			if job.Kind != assemblyline.WorkApplicationJobSpecification {
				return assemblyline.PortableResult{}, fmt.Errorf("uncorrectable initial candidate dispatched %s", job.Kind)
			}
			return workloadPortableCandidate(job, `{
				"objective":"Implement record grouping.",
				"required_behaviors":[],
				"acceptance_criteria":["The grouped record is visible."]
			}`), nil
		},
	}

	frozen, err := resolveDirectCodingApplicationWorkload(runtime, "semantic", "semantic-review", input)
	if err == nil {
		t.Fatal("uncorrectable unreviewed specification succeeded")
	}
	if !reflect.DeepEqual(kinds, []assemblyline.WorkKind{assemblyline.WorkApplicationJobSpecification}) {
		t.Fatalf("uncorrectable unreviewed specification triggered a semantic repair/review: %v", kinds)
	}
	if frozen.Schema != "" || frozen.SHA256 != "" || len(frozen.Tasks) != 0 {
		t.Fatalf("uncorrectable unreviewed specification was frozen: %+v", frozen)
	}
}

func TestApplicationWorkloadResolutionRejectsReviewerReplacementBeforeRepairOrFreeze(t *testing.T) {
	t.Parallel()

	specification := workerApplicationSpecification()
	specification.Requirements = specification.Requirements[:1]
	input := applicationWorkloadInput(specification)
	var kinds []assemblyline.WorkKind
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 3,
		Execute: func(job assemblyline.PortableJob, _ string) (assemblyline.PortableResult, error) {
			kinds = append(kinds, job.Kind)
			switch job.Kind {
			case assemblyline.WorkApplicationJobSpecification:
				return workloadPortableCandidate(job, workerJobSpecificationCandidate("groups records")), nil
			case assemblyline.WorkApplicationJobSpecificationReview:
				return workloadPortableCandidate(job, `{"decision":"repair","field":"acceptance_criteria","finding":"The checks do not cover the required behavior.","finding_evidence":"A newly created named group is visible.","replacement":"Add an unreviewed replacement."}`), nil
			default:
				return assemblyline.PortableResult{}, fmt.Errorf("reviewer replacement dispatched forbidden work %s", job.Kind)
			}
		},
	}

	frozen, err := resolveDirectCodingApplicationWorkload(runtime, "semantic", "semantic-review", input)
	if err == nil {
		t.Fatal("reviewer-authored replacement entered repair authority")
	}
	wantKinds := []assemblyline.WorkKind{
		assemblyline.WorkApplicationJobSpecification,
		assemblyline.WorkApplicationJobSpecificationReview,
	}
	if !reflect.DeepEqual(kinds, wantKinds) {
		t.Fatalf("reviewer replacement dispatched repair/freeze work: %v", kinds)
	}
	if frozen.Schema != "" || frozen.SHA256 != "" || len(frozen.Tasks) != 0 {
		t.Fatalf("reviewer replacement leaked a frozen workload: %+v", frozen)
	}
}

func TestApplicationJobSpecificationTransportFailureDoesNotTriggerReviewOrRepair(t *testing.T) {
	t.Parallel()

	var kinds []assemblyline.WorkKind
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 3,
		Execute: func(job assemblyline.PortableJob, _ string) (assemblyline.PortableResult, error) {
			kinds = append(kinds, job.Kind)
			if job.Kind != assemblyline.WorkApplicationJobSpecification {
				t.Fatalf("transport failure dispatched %s", job.Kind)
			}
			return assemblyline.PortableResult{}, errors.New("model transport unavailable")
		},
	}
	specification := workerApplicationSpecification()
	specification.Requirements = specification.Requirements[:1]
	_, err := resolveDirectCodingApplicationWorkload(
		runtime, "semantic", "semantic-review", applicationWorkloadInput(specification),
	)
	if err == nil || !strings.Contains(err.Error(), "transport unavailable") {
		t.Fatalf("transport failure=%v", err)
	}
	if !reflect.DeepEqual(kinds, []assemblyline.WorkKind{assemblyline.WorkApplicationJobSpecification}) {
		t.Fatalf("transport failure triggered semantic review/repair: %v", kinds)
	}
}

func workerApplicationSpecification() assemblyline.ApplicationSpecification {
	return assemblyline.ApplicationSpecification{
		Surface:      assemblyline.ApplicationSurfaceBrowser,
		ProductQuote: "records console",
		Requirements: []assemblyline.Requirement{
			{ID: "requirement_001", SourceQuote: "groups records"},
			{ID: "requirement_002", SourceQuote: "filters records"},
			{ID: "requirement_003", SourceQuote: "exports summaries"},
		},
	}
}

func workerApplicationWorkloadInput() assemblyline.ApplicationWorkloadDraftInput {
	return applicationWorkloadInput(workerApplicationSpecification())
}

func workerJobSpecificationForRequirement(requirement string) assemblyline.ApplicationJobSpecification {
	switch requirement {
	case "groups records":
		return assemblyline.ApplicationJobSpecification{
			Objective: "Implement interactive record grouping in the records console.",
			RequiredBehaviors: []string{
				"Users can create a named group.",
				"Users can assign and remove visible records from a group.",
			},
			AcceptanceCriteria: []string{
				"A newly created named group is visible.",
				"Assigning a record visibly lists it in the selected group.",
			},
		}
	case "filters records":
		return assemblyline.ApplicationJobSpecification{
			Objective: "Implement interactive record filtering in the records console.",
			RequiredBehaviors: []string{
				"Users can enter and clear a record filter.",
				"The visible record collection responds to the active filter.",
			},
			AcceptanceCriteria: []string{
				"Entering a filter hides records that do not match it.",
				"Clearing the filter restores the full visible record collection.",
			},
		}
	case "exports summaries":
		return assemblyline.ApplicationJobSpecification{
			Objective: "Implement summary export from the records console.",
			RequiredBehaviors: []string{
				"Users can request an export of the visible record summary.",
				"The export represents the current visible record collection.",
			},
			AcceptanceCriteria: []string{
				"The export action produces a summary artifact.",
				"The produced summary reflects the records visible when export was requested.",
			},
		}
	default:
		return assemblyline.ApplicationJobSpecification{}
	}
}

func workerJobSpecificationCandidate(prompt string) string {
	focused := ""
	mostOccurrences := 0
	for _, requirement := range []string{"groups records", "filters records", "exports summaries"} {
		if occurrences := strings.Count(prompt, requirement); occurrences > mostOccurrences {
			focused = requirement
			mostOccurrences = occurrences
		}
	}
	if focused != "" {
		specification := workerJobSpecificationForRequirement(focused)
		return fmt.Sprintf(
			`{"objective":%q,"required_behaviors":[%q,%q],"acceptance_criteria":[%q,%q]}`,
			specification.Objective,
			specification.RequiredBehaviors[0], specification.RequiredBehaviors[1],
			specification.AcceptanceCriteria[0], specification.AcceptanceCriteria[1],
		)
	}
	return `{}`
}

func workerJobSpecificationValues(prompt string) []string {
	for _, requirement := range []string{"groups records", "filters records", "exports summaries"} {
		candidate := workerJobSpecificationForRequirement(requirement)
		if strings.Contains(prompt, candidate.Objective) {
			return append(
				append([]string{candidate.Objective}, candidate.RequiredBehaviors...),
				candidate.AcceptanceCriteria...,
			)
		}
	}
	return nil
}

func workloadPortableCandidate(job assemblyline.PortableJob, candidate string) assemblyline.PortableResult {
	return assemblyline.PortableResult{JobID: job.ID, Candidate: candidate}
}

func assertFullJobSpecificationAuthority(
	t *testing.T,
	prompt string,
	specification assemblyline.ApplicationSpecification,
) {
	t.Helper()
	for _, required := range []string{string(specification.Surface), specification.ProductQuote} {
		if !strings.Contains(prompt, required) {
			t.Fatalf("job-specification prompt omitted authoritative %q:\n%s", required, prompt)
		}
	}
	for _, requirement := range specification.Requirements {
		if !strings.Contains(prompt, requirement.SourceQuote) {
			t.Fatalf("job-specification prompt omitted accepted requirement %q:\n%s", requirement.SourceQuote, prompt)
		}
	}
	assertPromptHasNoModelOwnedExecutionAuthority(t, prompt)
}

func assertPromptHasNoModelOwnedExecutionAuthority(t *testing.T, prompt string) {
	t.Helper()
	for _, forbidden := range []string{
		"requirement_001", "requirement_002", "requirement_003",
		"task_001", "SECRET_WORKSPACE_PATH", "SECRET_TOOL_CATALOG",
		"depends_on", "next_task", "file_path", "workspace_path", "completion_state",
	} {
		if strings.Contains(strings.ToLower(prompt), strings.ToLower(forbidden)) {
			t.Fatalf("semantic prompt exposed model-owned execution authority %q:\n%s", forbidden, prompt)
		}
	}
}

func assertJobSpecificationSchemaHasNoCodeAuthority(schema map[string]any) error {
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		return fmt.Errorf("job specification schema has no closed properties")
	}
	for _, required := range []string{"objective", "required_behaviors", "acceptance_criteria"} {
		if properties[required] == nil {
			return fmt.Errorf("job specification schema omitted %s: %v", required, properties)
		}
	}
	if len(properties) != 3 {
		return fmt.Errorf("job specification schema is not exactly three semantic fields: %v", properties)
	}
	return assertSchemaHasNoCodeExecutionAuthority(properties)
}

func assertJobSpecificationReviewSchemaHasNoCodeAuthority(schema map[string]any) error {
	if schema["type"] != "object" {
		return fmt.Errorf("job specification review schema has no object root: %v", schema)
	}
	branches, ok := schema["oneOf"].([]any)
	if !ok || len(branches) != 2 {
		return fmt.Errorf("job specification review schema is not a two-branch union: %v", schema)
	}
	for index, raw := range branches {
		branch, ok := raw.(map[string]any)
		if !ok || branch["type"] != "object" || branch["additionalProperties"] != false {
			return fmt.Errorf("job specification review branch %d is not closed: %v", index, raw)
		}
		properties, ok := branch["properties"].(map[string]any)
		if !ok || properties["decision"] == nil {
			return fmt.Errorf("job specification review branch %d omits decision: %v", index, branch)
		}
		if err := assertSchemaHasNoCodeExecutionAuthority(properties); err != nil {
			return err
		}
	}
	accept := branches[0].(map[string]any)
	repairBranch := branches[1].(map[string]any)
	if !reflect.DeepEqual(accept["required"], []string{"decision"}) ||
		!reflect.DeepEqual(repairBranch["required"], []string{
			"decision", "field", "finding", "finding_evidence",
		}) {
		return fmt.Errorf("job specification review branches permit incomplete responses: %v", schema)
	}
	return nil
}

func assertOneFieldJobSpecificationRepairSchema(schema map[string]any, attempt int) error {
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		return fmt.Errorf("job specification repair schema has no closed properties")
	}
	want := "required_behaviors"
	if attempt == 2 {
		want = "acceptance_criteria"
	}
	if len(properties) != 1 || properties[want] == nil {
		return fmt.Errorf("repair %d schema is not exactly reviewer field %s: %v", attempt, want, properties)
	}
	return assertSchemaHasNoCodeExecutionAuthority(properties)
}

func assertSchemaHasNoCodeExecutionAuthority(properties map[string]any) error {
	for _, forbidden := range []string{
		"schema", "requirement_id", "task_id", "file", "path", "depends_on",
		"order", "complete", "completion", "next_task", "tool", "command",
	} {
		if _, exists := properties[forbidden]; exists {
			return fmt.Errorf("semantic schema grants code-owned %s authority", forbidden)
		}
	}
	return nil
}
