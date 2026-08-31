package worker

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

const directCodingResultRelationTestRequest = "Build a neutral operations console with one public control and one observable value derived from one input."

func directCodingResultRelationTestAuthority(
	t testing.TB,
) directCodingApplicationRequestAuthority {
	t.Helper()
	authority, err := newDirectCodingApplicationRequestAuthority(
		directCodingResultRelationTestRequest,
		directCodingResultRelationTestRequest,
	)
	if err != nil {
		t.Fatal(err)
	}
	return authority
}

func directCodingTestRequirementRelations(
	t *testing.T,
	workload assemblyline.FrozenApplicationWorkload,
	relations ...string,
) directCodingApplicationTaskResultRelationPlan {
	t.Helper()
	if len(relations) == 0 {
		relations = make([]string, len(workload.Tasks))
		for index := range relations {
			relations[index] = assemblyline.ApplicationRequirementNoDerivedResult
		}
	}
	if len(relations) != len(workload.Tasks) {
		t.Fatalf("result-relation fixture has %d values for %d tasks", len(relations), len(workload.Tasks))
	}
	accepted := make([]assemblyline.ApplicationRequirement, len(workload.Tasks))
	requestSHA256 := assemblyline.ExactObjectiveContextSHA(
		directCodingResultRelationTestRequest,
	)
	for index, task := range workload.Tasks {
		kind := applicationRequirementCandidateKindReceiptForTest(
			t,
			task.RequirementQuote,
			assemblyline.ApplicationRequirementCandidateTaskLocal,
		)
		cardinality, err := assemblyline.DecodeApplicationRequirementCandidateCardinalityResult(
			assemblyline.ApplicationRequirementCandidateCardinalityInput{Candidate: task.RequirementQuote},
			assemblyline.ApplicationRequirementOneRuntimeOutcome,
		)
		if err != nil {
			t.Fatal(err)
		}
		relation := applicationRequirementCandidateResultRelationReceiptForTest(
			t,
			assemblyline.ApplicationRequirementCandidateResultRelationInput{
				Candidate: task.RequirementQuote, Kind: kind, Cardinality: cardinality,
			},
			relations[index],
		)
		accepted[index] = assemblyline.ApplicationRequirement{
			ID: task.RequirementID, Statement: task.RequirementQuote,
			RequestSHA256:  requestSHA256,
			ResultRelation: relation,
		}
	}
	plan, err := newDirectCodingApplicationTaskResultRelationPlan(
		workload, accepted, directCodingResultRelationTestAuthority(t),
	)
	if err != nil {
		t.Fatal(err)
	}
	return plan
}

func bindDirectCodingTestRequirementRelations(
	t *testing.T,
	program *directCodingProgram,
	relations ...string,
) {
	t.Helper()
	if program == nil {
		t.Fatal("result-relation fixture requires one program")
	}
	program.RequirementRelations = directCodingTestRequirementRelations(
		t, program.Workload, relations...,
	)
}

func TestApplicationTaskResultRelationPlanBindsAndProjectsAcceptedReceipts(t *testing.T) {
	workload := directCodingResultRelationTestWorkload(t)
	plan := directCodingTestRequirementRelations(
		t,
		workload,
		assemblyline.ApplicationRequirementNoDerivedResult,
		assemblyline.ApplicationRequirementExplicitResultRelation,
	)
	if err := plan.validateCompleteFor(workload); err != nil {
		t.Fatal(err)
	}
	projected, err := plan.projectTask(workload, "task_002")
	if err != nil {
		t.Fatal(err)
	}
	if len(projected.Bindings) != 1 || projected.Bindings[0].TaskID != "task_002" ||
		projected.Bindings[0].Receipt.Relation != assemblyline.ApplicationRequirementExplicitResultRelation {
		t.Fatalf("projected result relation=%+v", projected)
	}
	if _, err := projected.bindingForTask(workload, "task_002"); err != nil {
		t.Fatal(err)
	}
}

func TestApplicationTaskResultRelationPlanRejectsAuthorityDrift(t *testing.T) {
	workload := directCodingResultRelationTestWorkload(t)
	valid := directCodingTestRequirementRelations(t, workload)
	tests := map[string]func(*directCodingApplicationTaskResultRelationPlan){
		"missing binding": func(plan *directCodingApplicationTaskResultRelationPlan) {
			plan.Bindings = plan.Bindings[:1]
		},
		"extra binding": func(plan *directCodingApplicationTaskResultRelationPlan) {
			plan.Bindings = append(plan.Bindings, plan.Bindings[0])
		},
		"swapped binding": func(plan *directCodingApplicationTaskResultRelationPlan) {
			plan.Bindings[0], plan.Bindings[1] = plan.Bindings[1], plan.Bindings[0]
		},
		"workload hash": func(plan *directCodingApplicationTaskResultRelationPlan) {
			plan.WorkloadSHA256 = strings.Repeat("b", 64)
		},
		"request hash": func(plan *directCodingApplicationTaskResultRelationPlan) {
			plan.RequestSHA256 = strings.Repeat("b", 64)
		},
		"agreed fabricated request hash": func(plan *directCodingApplicationTaskResultRelationPlan) {
			plan.RequestSHA256 = strings.Repeat("b", 64)
			for index := range plan.Bindings {
				plan.Bindings[index].RequestSHA256 = plan.RequestSHA256
			}
		},
		"binding request hash": func(plan *directCodingApplicationTaskResultRelationPlan) {
			plan.Bindings[0].RequestSHA256 = strings.Repeat("b", 64)
		},
		"candidate hash": func(plan *directCodingApplicationTaskResultRelationPlan) {
			plan.Bindings[0].Receipt.CandidateSHA256 = strings.Repeat("c", 64)
		},
		"non-retainable relation": func(plan *directCodingApplicationTaskResultRelationPlan) {
			plan.Bindings[0].Receipt.Relation = assemblyline.ApplicationRequirementMissingResultRelation
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			candidate := valid
			candidate.Bindings = append(
				[]directCodingApplicationTaskResultRelationBinding(nil), valid.Bindings...,
			)
			mutate(&candidate)
			if err := candidate.validateCompleteFor(workload); err == nil {
				t.Fatalf("drifted result-relation plan was accepted: %+v", candidate)
			}
		})
	}
}

func TestApplicationTaskResultRelationPlanRejectsAgreedFabricatedRequestHash(t *testing.T) {
	workload := directCodingResultRelationTestWorkload(t)
	valid := directCodingTestRequirementRelations(t, workload)
	accepted := make([]assemblyline.ApplicationRequirement, len(workload.Tasks))
	for index, task := range workload.Tasks {
		accepted[index] = assemblyline.ApplicationRequirement{
			ID: task.RequirementID, Statement: task.RequirementQuote,
			RequestSHA256:  strings.Repeat("a", 64),
			ResultRelation: valid.Bindings[index].Receipt,
		}
	}
	if _, err := newDirectCodingApplicationTaskResultRelationPlan(
		workload, accepted, directCodingResultRelationTestAuthority(t),
	); err == nil || !strings.Contains(err.Error(), "authoritative request") {
		t.Fatalf("agreed fabricated request hashes were accepted: %v", err)
	}
	for index := range accepted {
		accepted[index].RequestSHA256 = valid.RequestSHA256
	}
	authority := directCodingResultRelationTestAuthority(t)
	authority.authoritativeRequest += " changed"
	if _, err := newDirectCodingApplicationTaskResultRelationPlan(
		workload, accepted, authority,
	); err == nil || !strings.Contains(err.Error(), "not authenticated") {
		t.Fatalf("unauthenticated request authority was accepted: %v", err)
	}
}

func TestApplicationTaskResultRelationReceiptRemainsOutsideTaskAndFragmentPrompts(t *testing.T) {
	_, workload, program := applicationTaskLifecycleFixture(t)
	program.VersionProfileID = typeScriptBrowserVersionProfileV1
	bindDirectCodingTestRequirementRelations(
		t,
		&program,
		assemblyline.ApplicationRequirementExplicitResultRelation,
		assemblyline.ApplicationRequirementNoDerivedResult,
	)
	context, err := assemblyline.ProjectApplicationTaskContext(workload, "task_001")
	if err != nil {
		t.Fatal(err)
	}
	stage, err := projectDirectCodingApplicationTaskStage(program, context)
	if err != nil {
		t.Fatal(err)
	}
	workloadJSON, err := json.Marshal(workload)
	if err != nil {
		t.Fatal(err)
	}
	contextJSON, err := json.Marshal(context)
	if err != nil {
		t.Fatal(err)
	}
	values := []struct {
		label string
		value string
	}{
		{label: "frozen workload", value: string(workloadJSON)},
		{label: "task context", value: string(contextJSON)},
	}
	for _, blockID := range []string{"feature.001", "acceptance.001"} {
		ref := directCodingTestGeneratedBlockRef(t, stage.Source, blockID)
		input, err := directCodingLanguageFragmentInput(&stage, ref, "typescript")
		if err != nil {
			t.Fatal(err)
		}
		job, err := assemblyline.NewFragmentGenerationJob(input)
		if err != nil {
			t.Fatal(err)
		}
		prompt, err := assemblyline.RenderPortableJob(job)
		if err != nil {
			t.Fatal(err)
		}
		values = append(values, struct {
			label string
			value string
		}{label: blockID + " prompt", value: prompt})
	}
	for _, candidate := range values {
		for _, forbidden := range []string{
			"result_relation",
			assemblyline.ApplicationRequirementCandidateResultRelationSchemaV1,
			assemblyline.ApplicationRequirementNoDerivedResult,
			assemblyline.ApplicationRequirementExplicitResultRelation,
			assemblyline.ApplicationRequirementMissingResultRelation,
		} {
			if strings.Contains(candidate.value, forbidden) {
				t.Fatalf("%s leaked result-relation authority %q: %s", candidate.label, forbidden, candidate.value)
			}
		}
	}
}

func directCodingResultRelationTestWorkload(
	t *testing.T,
) assemblyline.FrozenApplicationWorkload {
	t.Helper()
	workload, err := assemblyline.FreezeApplicationWorkload(
		assemblyline.ApplicationSpecification{
			Surface:      assemblyline.ApplicationSurfaceBrowser,
			ProductQuote: "neutral operations console",
			Requirements: []assemblyline.Requirement{
				{ID: "requirement_001", SourceQuote: "Show one public control."},
				{ID: "requirement_002", SourceQuote: "Derive one observable value from one input."},
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	return workload
}
