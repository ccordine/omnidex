package omni

import (
	"fmt"
	"strings"
)

const (
	structuredProofTypeUnitTest                  = "unit_test"
	structuredProofTypeIntegrationTest           = "integration_test"
	structuredProofTypeSmokeTest                 = "smoke_test"
	structuredProofTypeGoldenOutput              = "golden_output"
	structuredProofTypeCompilerCheck             = "compiler_check"
	structuredProofTypeLintCheck                 = "lint_check"
	structuredProofTypeSourceVerification        = "source_verification"
	structuredProofTypeManualEvaluatorAcceptance = "manual_evaluator_acceptance"
)

const (
	structuredProofEventTestCreated                  = "test_created"
	structuredProofEventTestValidated                = "test_validated"
	structuredProofEventTestFailedAsExpected         = "test_failed_as_expected"
	structuredProofEventImplementationStarted        = "implementation_started"
	structuredProofEventTestPassed                   = "test_passed"
	structuredProofEventTestModified                 = "test_modified"
	structuredProofEventTestModificationApproved     = "test_modification_approved"
	structuredProofEventTestModificationRejected     = "test_modification_rejected"
	structuredProofEventAcceptanceProbeCreated       = "acceptance_probe_created"
	structuredProofEventAcceptanceProbePassed        = "acceptance_probe_passed"
	structuredProofEventAcceptanceProbeFailed        = "acceptance_probe_failed"
	structuredProofEventEvaluatorAcceptanceCompleted = "evaluator_acceptance_completed"
)

type StructuredProofPlan struct {
	ObjectiveID             string   `json:"objective_id"`
	ProofType               string   `json:"proof_type"`
	FilesToCreate           []string `json:"files_to_create,omitempty"`
	FilesToModify           []string `json:"files_to_modify,omitempty"`
	Commands                []string `json:"commands,omitempty"`
	AcceptanceChecks        []string `json:"acceptance_checks,omitempty"`
	OutOfScope              []string `json:"out_of_scope,omitempty"`
	AllowedObjectiveSources []string `json:"allowed_objective_sources,omitempty"`
}

func structuredDevelopmentLoopPolicy() []string {
	return []string{
		"proof_first: define success before implementation with a focused test, smoke test, golden output, compiler/lint check, source-verification probe, or evaluator acceptance checklist",
		"test_first: for code/app feature work, create or update a focused failing test, smoke test, or deterministic verification probe for the requested behavior before implementation when feasible",
		"implement_second: make the smallest source/build/config change needed to satisfy that test/probe",
		"verify_third: run the focused test/probe after implementation; if it fails, use stdout/stderr as the next correction target",
		"fallback_probe: when a real test runner/compiler is unavailable, write and run a deterministic source-verification probe that checks concrete files, symbols, behavior strings, or command outputs",
		"completion_gate: do not request done=true from implementation evidence alone; finish only after post-write test/probe/readback evidence",
		"scope_gate: proof plans may prove only user_explicit, recipe_required, or evidence_required_prerequisite objectives; memory_suggested and model_inferred items cannot add tests or implementation scope",
		"test_tamper_gate: after a proof test/probe is validated, do not weaken, delete, skip, or rewrite it unless validator evidence shows syntax/tooling invalidity, the user changes the request, or the framework requires an equivalent form",
	}
}

func structuredProofPolicy() []string {
	return []string{
		"contract_first_tdd_loop: create a proof_plan before or with implementation for build/code/app tasks when feasible",
		"allowed_objective_sources: user_explicit, recipe_required, evidence_required_prerequisite",
		"disallowed_scope_sources: memory_suggested, model_inferred",
		"proof_types: unit_test, integration_test, smoke_test, golden_output, compiler_check, lint_check, source_verification, manual_evaluator_acceptance",
		"validator_checks: user_request_alignment, minimal_scope, executable_in_project, avoids_unrequested_features, avoids_brittle_implementation_details, expected_preimplementation_failure_when_applicable, clear_postimplementation_signal",
		"test_tampering: validated tests/probes cannot be weakened, deleted, skipped, or rewritten by the coder without validator-approved correction",
		"allowed_test_corrections: syntax_or_tooling_error, validator_confirms_invalid_test, user_request_changed, framework_requires_equivalent_form",
		"lifecycle_events: test_created, test_validated, test_failed_as_expected, implementation_started, test_passed, test_modified, test_modification_approved, test_modification_rejected",
	}
}

func defaultStructuredProofPlanAllowedSources() []string {
	return []string{
		structuredObjectiveSourceUserExplicit,
		structuredObjectiveSourceRecipeRequired,
		structuredObjectiveSourceEvidenceRequiredPrerequisite,
	}
}

func structuredProofPlanLifecycle() []string {
	return []string{
		structuredProofEventTestCreated,
		structuredProofEventTestValidated,
		structuredProofEventTestFailedAsExpected,
		structuredProofEventImplementationStarted,
		structuredProofEventTestPassed,
		structuredProofEventTestModified,
		structuredProofEventTestModificationApproved,
		structuredProofEventTestModificationRejected,
	}
}

func validateStructuredProofPlan(plan StructuredProofPlan, ledger []StructuredObjective) error {
	objectiveID := strings.TrimSpace(plan.ObjectiveID)
	if objectiveID == "" {
		return fmt.Errorf("proof plan objective_id is required")
	}
	proofType := strings.TrimSpace(plan.ProofType)
	if proofType == "" {
		return fmt.Errorf("proof plan proof_type is required")
	}
	if !structuredProofTypeKnown(proofType) {
		return fmt.Errorf("proof plan proof_type %q is not supported", proofType)
	}
	if len(nonEmptyStrings(plan.Commands)) == 0 && len(nonEmptyStrings(plan.AcceptanceChecks)) == 0 {
		return fmt.Errorf("proof plan must include at least one executable command or acceptance check")
	}
	objective, ok := findStructuredObjectiveByID(ledger, objectiveID)
	if !ok {
		return fmt.Errorf("proof plan objective_id %q is not present in the objective ledger", objectiveID)
	}
	source := normalizeStructuredObjectiveSource(objective.Source)
	if !structuredProofObjectiveSourceAllowed(source, plan.AllowedObjectiveSources) {
		return fmt.Errorf("proof plan objective %q has disallowed source %q", objectiveID, source)
	}
	return nil
}

func hasStructuredProofPlan(plan StructuredProofPlan) bool {
	return strings.TrimSpace(plan.ObjectiveID) != "" ||
		strings.TrimSpace(plan.ProofType) != "" ||
		len(nonEmptyStrings(plan.Commands)) > 0 ||
		len(nonEmptyStrings(plan.AcceptanceChecks)) > 0 ||
		len(nonEmptyStrings(plan.FilesToCreate)) > 0 ||
		len(nonEmptyStrings(plan.FilesToModify)) > 0 ||
		len(nonEmptyStrings(plan.OutOfScope)) > 0
}

func structuredProofTypeKnown(proofType string) bool {
	switch strings.TrimSpace(proofType) {
	case structuredProofTypeUnitTest,
		structuredProofTypeIntegrationTest,
		structuredProofTypeSmokeTest,
		structuredProofTypeGoldenOutput,
		structuredProofTypeCompilerCheck,
		structuredProofTypeLintCheck,
		structuredProofTypeSourceVerification,
		structuredProofTypeManualEvaluatorAcceptance:
		return true
	default:
		return false
	}
}

func structuredProofObjectiveSourceAllowed(source string, allowed []string) bool {
	if len(allowed) == 0 {
		allowed = defaultStructuredProofPlanAllowedSources()
	}
	source = normalizeStructuredObjectiveSource(source)
	for _, candidate := range allowed {
		if source == normalizeStructuredObjectiveSource(candidate) {
			return true
		}
	}
	return false
}

func findStructuredObjectiveByID(ledger []StructuredObjective, id string) (StructuredObjective, bool) {
	for _, objective := range ledger {
		if objective.ID == id {
			return objective, true
		}
	}
	return StructuredObjective{}, false
}

func nonEmptyStrings(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			out = append(out, value)
		}
	}
	return out
}
