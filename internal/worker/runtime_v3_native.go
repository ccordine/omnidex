package worker

import (
	"context"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/artifacts"
	"github.com/gryph/omnidex/internal/evidence"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/specialist"
	toolruntime "github.com/gryph/omnidex/internal/tools"
)

const maxDelegatedSubtasks = 6

type nativeRuntimeV3 struct {
	svc      *Service
	ctx      context.Context
	claim    *model.ClaimedStep
	action   string
	contexts map[string]string
}

func (s *Service) runNativeV3Step(ctx context.Context, claim *model.ClaimedStep, contexts map[string]string, action string) error {
	runtime := &nativeRuntimeV3{svc: s, ctx: ctx, claim: claim, action: strings.ToLower(strings.TrimSpace(action)), contexts: contexts}
	return runtime.run()
}

func (r *nativeRuntimeV3) run() error {
	switch r.action {
	case "v3_intent_parse":
		return r.runIntentParse()
	case "v3_capability_audit":
		return r.runCapabilityAudit()
	case "v3_workspace_research":
		return r.runWorkspaceResearch()
	case "v3_memory_retrieval":
		return r.runMemoryRetrieval()
	case "v3_external_research":
		return r.runExternalResearch()
	case "v3_planning":
		return r.runPlanning()
	case "v3_subtask":
		return r.runSubtask()
	case "v3_analysis":
		return r.runAnalysis()
	case "v3_response_draft":
		return r.runResponseDraft()
	case "v3_verification":
		return r.runVerification()
	case "v3_memory_review":
		return r.runMemoryReview()
	case "v3_finalize":
		return r.runFinalize()
	case "v3_chat_fastpath":
		return r.runChatFastPath()
	default:
		return fmt.Errorf("unsupported native v3 action %q", r.action)
	}
}

func (r *nativeRuntimeV3) runIntentParse() error {
	intentInput, err := r.svc.buildV3IntentInput(r.claim.Job)
	if err != nil {
		return err
	}
	payload := map[string]any{
		"current_user_instruction":   intentInput.CurrentInstruction,
		"authority_directives":       intentInput.AuthorityDirectives,
		"pipeline":                   strings.TrimSpace(r.claim.Job.Pipeline),
		"execution_agent":            intentInput.ExecutionAgent,
		"operation_kind":             intentInput.OperationKind,
		"transport_requires_action":  intentInput.TransportRequiresAction,
		"authoritative_task_context": intentInput.TaskContext,
		"capability_catalog":         intentInput.CapabilityCatalog,
	}
	invocation, err := r.invocationFor(
		"prompt_interpreter",
		"interpret_current_user_instruction",
		"Produce the authoritative intent artifact for the current user instruction",
		100,
		[]string{"all user objectives and priorities are explicit", "completion criteria are observable", "required capabilities are exact"},
		nil,
		payload,
	)
	if err != nil {
		return err
	}
	modelName := r.svc.v3SpecialistModel(r.claim.Job, "prompt_interpreter", specialist.RoleIntentTaggingSpecialist, r.svc.models.Tagging)
	validateOutput := func(output map[string]any) error {
		candidate, err := decodeV3TypedOutput[artifacts.IntentArtifact](output)
		if err != nil {
			return err
		}
		if err := validateV3Intent(candidate, knownV3Capabilities()); err != nil {
			return err
		}
		if err := validateV3IntentGrounding(intentInput, candidate); err != nil {
			return err
		}
		return validateV3TransportIntent(intentInput, candidate)
	}
	output, err := r.invokeSpecialist("v3_intent_parse", "prompt_interpreter", modelName, invocation, validateOutput)
	if err != nil {
		return err
	}
	intent, err := decodeV3TypedOutput[artifacts.IntentArtifact](output)
	if err != nil {
		return err
	}
	if len(intent.UnresolvedReferences) > 0 {
		return fmt.Errorf("prompt interpreter found unresolved references: %s", strings.Join(intent.UnresolvedReferences, "; "))
	}
	if err := r.writeArtifact(artifacts.KindIntent, intent); err != nil {
		return err
	}
	summary := strings.Join([]string{
		"goal=" + safeLine(trimForBudget(intent.UserGoal, 160), "none"),
		fmt.Sprintf("requires_action=%t", intent.RequiresAction),
		"required_capabilities=" + csvOrNone(intent.RequiredCapabilities),
		fmt.Sprintf("objectives=%d", len(intent.Objectives)),
		"constraints=" + csvOrNone(intent.Constraints),
	}, "\n")
	return r.complete("intent", summary, summary)
}

func (r *nativeRuntimeV3) runCapabilityAudit() error {
	intent, err := r.readIntentArtifact()
	if err != nil {
		return err
	}
	result, err := r.svc.executeV3Tool(r.ctx, r.claim, "capability_auditor", toolruntime.Call{Name: "tool.registry"})
	if err != nil {
		return fmt.Errorf("capability audit tool registry failed: %w", err)
	}
	catalog, err := decodeToolOutput[struct {
		Summary string `json:"summary"`
		Tools   []struct {
			Name        string         `json:"name"`
			Description string         `json:"description"`
			Aliases     []string       `json:"aliases"`
			InputSchema map[string]any `json:"input_schema"`
		} `json:"tools"`
	}](result)
	if err != nil {
		return fmt.Errorf("capability audit registry output: %w", err)
	}
	availableTools := make([]string, 0, len(catalog.Tools))
	for _, item := range catalog.Tools {
		if clean := strings.TrimSpace(item.Name); clean != "" {
			availableTools = append(availableTools, clean)
		}
	}
	availableTools = filterRuntimeAvailableV3Tools(r.svc, uniqueStrings(availableTools))
	availableCapabilities := availableV3Capabilities(availableTools)
	requiredCapabilities := append([]string(nil), intent.RequiredCapabilities...)
	for _, objective := range intent.Objectives {
		requiredCapabilities = append(requiredCapabilities, objective.RequiredCapabilities...)
	}
	requiredCapabilities = uniqueStrings(requiredCapabilities)
	missingCapabilities := missingV3Capabilities(requiredCapabilities, availableCapabilities)
	missingToolNames := toolsForV3Capabilities(missingCapabilities)
	audit := artifacts.CapabilityAuditArtifact{
		AllowedTools:          differenceStrings(toolsForV3Capabilities(requiredCapabilities), missingToolNames),
		AvailableTools:        availableTools,
		MissingTools:          missingToolNames,
		AvailableCapabilities: availableCapabilities,
		MissingCapabilities:   missingCapabilities,
		WorkspaceOK:           containsString(availableCapabilities, capabilityWorkspaceRead),
		WebSearchOK:           containsString(availableCapabilities, capabilityWebSearch),
		Notes:                 []string{"capabilities derived only from the callable v3 tool registry", "host binaries do not imply agent execution authority"},
	}
	if err := r.writeArtifact(artifacts.KindCapabilityAudit, audit); err != nil {
		return err
	}
	if err := r.writeEvidence(evidence.Record{Kind: evidence.KindModelJudgment, SourceType: "runtime_v3", SourceRef: "capability_audit", Summary: "Callable capability audit completed.", Confidence: 1, Metadata: map[string]any{"required_capabilities": requiredCapabilities, "missing_capabilities": missingCapabilities, "available_tools": availableTools}}); err != nil {
		return err
	}
	output := strings.Join([]string{
		"required_capabilities=" + csvOrNone(requiredCapabilities),
		"available_capabilities=" + csvOrNone(availableCapabilities),
		"missing_capabilities=" + csvOrNone(missingCapabilities),
	}, "\n")
	if len(missingCapabilities) > 0 {
		r.svc.emitStepEvent(r.claim.Step.ID, "capability_audit_blocked", "missing="+strings.Join(missingCapabilities, ","))
		return fmt.Errorf("required capabilities are unavailable: %s", strings.Join(missingCapabilities, ", "))
	}
	return r.complete("capability_audit", output, output)
}
