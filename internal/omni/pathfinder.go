package omni

import (
	"context"
	"fmt"
	"io"
	"strings"
)

type PathfinderActionKind string

const (
	PathfinderActionInspect                     PathfinderActionKind = "inspect"
	PathfinderActionResearch                    PathfinderActionKind = "research"
	PathfinderActionCreateOrUpdateProofContract PathfinderActionKind = "create_or_update_proof_contract"
	PathfinderActionPatch                       PathfinderActionKind = "patch"
	PathfinderActionRunVerification             PathfinderActionKind = "run_verification"
	PathfinderActionDelegateExternalAgent       PathfinderActionKind = "delegate_external_agent"
	PathfinderActionAdjustObjectiveLedger       PathfinderActionKind = "adjust_objective_ledger"
	PathfinderActionAdjustContextPlan           PathfinderActionKind = "adjust_context_plan"
	PathfinderActionRelaxRuntimeBlocker         PathfinderActionKind = "relax_runtime_blocker"
	PathfinderActionTightenRuntimeBlocker       PathfinderActionKind = "tighten_runtime_blocker"
	PathfinderActionAskUser                     PathfinderActionKind = "ask_user"
	PathfinderActionFailWithEvidence            PathfinderActionKind = "fail_with_evidence"
)

type ProblemCase struct {
	Problem            string                         `json:"problem"`
	CurrentGoal        string                         `json:"current_goal"`
	CurrentPhase       string                         `json:"current_phase"`
	BlockedObjectives  []string                       `json:"blocked_objectives,omitempty"`
	ObjectiveLedger    []StructuredObjective          `json:"objective_ledger,omitempty"`
	WorksiteSurvey     WorksiteSurvey                 `json:"worksite_survey,omitempty"`
	PrepContextSummary []string                       `json:"prep_context_summary,omitempty"`
	CodebaseRoute      TaskRoute                      `json:"codebase_route,omitempty"`
	RecentObservations []StructuredCommandObservation `json:"recent_observations,omitempty"`
	FailedCommands     []string                       `json:"failed_commands,omitempty"`
	RejectedCommands   []string                       `json:"rejected_commands,omitempty"`
	FalseDoneCount     int                            `json:"false_done_count,omitempty"`
	LoopState          StructuredLoopState            `json:"loop_state,omitempty"`
	AvailableTools     []string                       `json:"available_tools,omitempty"`
	Constraints        []string                       `json:"constraints,omitempty"`
	SuccessCondition   string                         `json:"success_condition"`
}

type CandidateStrategy struct {
	ID                 string `json:"id"`
	Description        string `json:"description"`
	ActionKind         string `json:"action_kind"`
	ProgressGain       int    `json:"progress_gain"`
	EvidenceValue      int    `json:"evidence_value"`
	Confidence         int    `json:"confidence"`
	Cost               int    `json:"cost"`
	Risk               int    `json:"risk"`
	Reversibility      int    `json:"reversibility"`
	ScopeSafety        int    `json:"scope_safety"`
	TimeEstimateBucket string `json:"time_estimate_bucket"`
	Score              int    `json:"score"`
}

type PathfinderNextAction struct {
	Kind       string   `json:"kind"`
	Command    string   `json:"command,omitempty"`
	ToolTask   string   `json:"tool_task,omitempty"`
	Objectives []string `json:"objectives,omitempty"`
	Rationale  string   `json:"rationale,omitempty"`
}

type BreakthroughPacket struct {
	Diagnosis           string                `json:"diagnosis"`
	RealBlocker         string                `json:"real_blocker"`
	Assumptions         []string              `json:"assumptions,omitempty"`
	EvidenceUsed        []string              `json:"evidence_used"`
	CandidateStrategies []CandidateStrategy   `json:"candidate_strategies"`
	SelectedStrategy    CandidateStrategy     `json:"selected_strategy"`
	ExpectedProgress    string                `json:"expected_progress"`
	Risk                string                `json:"risk"`
	NextAction          PathfinderNextAction  `json:"next_action"`
	ForbiddenPaths      []string              `json:"forbidden_paths,omitempty"`
	ProofNeededAfter    []string              `json:"proof_needed_after"`
	ObjectiveUpdates    []StructuredObjective `json:"objective_updates,omitempty"`
	ContextUpdates      []string              `json:"context_updates,omitempty"`
	Confidence          int                   `json:"confidence"`
}

type Pathfinder struct{}

func (Pathfinder) Solve(problem ProblemCase) BreakthroughPacket {
	problem = normalizeProblemCase(problem)
	evidence := pathfinderEvidence(problem)
	candidates := scoreCandidateStrategies(pathfinderCandidateStrategies(problem))
	selected := selectPathfinderStrategy(problem, candidates)
	packet := BreakthroughPacket{
		Diagnosis:           pathfinderDiagnosis(problem),
		RealBlocker:         pathfinderRealBlocker(problem),
		Assumptions:         pathfinderAssumptions(problem),
		EvidenceUsed:        evidence,
		CandidateStrategies: candidates,
		SelectedStrategy:    selected,
		ExpectedProgress:    pathfinderExpectedProgress(problem, selected),
		Risk:                pathfinderRisk(problem, selected),
		NextAction:          pathfinderNextAction(problem, selected),
		ForbiddenPaths:      pathfinderForbiddenPaths(problem),
		ProofNeededAfter:    pathfinderProofNeededAfter(problem, selected),
		Confidence:          selected.Confidence,
	}
	return packet
}

func BuildProblemCaseFromProgression(prompt string, phase string, decision ProgressionDecision, cfg structuredCommandDecisionRunConfig, survey WorksiteSurvey, ledger []StructuredObjective, observations []StructuredCommandObservation) ProblemCase {
	return normalizeProblemCase(ProblemCase{
		Problem:            firstNonEmpty(decision.Reason, "normal progression stalled"),
		CurrentGoal:        prompt,
		CurrentPhase:       phase,
		BlockedObjectives:  pendingStructuredObjectiveIDList(ledger),
		ObjectiveLedger:    mergeStructuredObjectiveLedger(nil, ledger),
		WorksiteSurvey:     survey,
		PrepContextSummary: pathfinderPrepSummary(cfg.PrepContext),
		CodebaseRoute:      cfg.PrepContext.CodebaseRoute,
		RecentObservations: compactStructuredObservationsForContext(observations, 8, 800),
		FailedCommands:     failedCommandList(observations, 6),
		RejectedCommands:   rejectedCommandList(observations, 6),
		FalseDoneCount:     falseDoneObservationCount(observations),
		LoopState:          decision.LoopState,
		AvailableTools: []string{
			"workspace_scan",
			"memory",
			"web",
			"shell",
			"TDD",
			"external_agents",
		},
		Constraints:      appendUniqueStrings(nil, cfg.PrepContext.Constraints...),
		SuccessCondition: "Produce one validated next action that advances the task without satisfying objectives from model claims.",
	})
}

func ValidateBreakthroughPacket(problem ProblemCase, packet BreakthroughPacket) error {
	if strings.TrimSpace(packet.RealBlocker) == "" {
		return fmt.Errorf("pathfinder packet rejected: real_blocker is required")
	}
	if len(packet.EvidenceUsed) == 0 {
		return fmt.Errorf("pathfinder packet rejected: evidence_used is required")
	}
	if len(packet.CandidateStrategies) < 3 {
		return fmt.Errorf("pathfinder packet rejected: at least 3 candidate strategies are required")
	}
	if strings.TrimSpace(packet.NextAction.Kind) == "" {
		return fmt.Errorf("pathfinder packet rejected: next_action.kind is required")
	}
	if !allowedPathfinderActionKind(packet.NextAction.Kind) {
		return fmt.Errorf("pathfinder packet rejected: unsupported next_action kind %q", packet.NextAction.Kind)
	}
	if packet.NextAction.Kind == "done" || packet.NextAction.Kind == "complete" {
		return fmt.Errorf("pathfinder packet rejected: pathfinder cannot mark objectives complete")
	}
	if len(packet.ProofNeededAfter) == 0 {
		return fmt.Errorf("pathfinder packet rejected: proof_needed_after is required")
	}
	if selectedMatchesExhaustedCommand(problem, packet) {
		return fmt.Errorf("pathfinder packet rejected: selected strategy repeats recently exhausted command")
	}
	if contradictsPathfinderConstraints(problem, packet) {
		return fmt.Errorf("pathfinder packet rejected: next action contradicts current constraints")
	}
	return nil
}

func emitPathfinderEvents(step int, problem ProblemCase, packet BreakthroughPacket, onEvent func(StructuredCommandEvent)) {
	emitStructuredCommandEvent(onEvent, "pathfinder_started", "Pathfinder invoked for stalled run", map[string]string{
		"step":  fmt.Sprintf("%d", step),
		"phase": problem.CurrentPhase,
	})
	emitStructuredCommandEvent(onEvent, "pathfinder_problem_framed", "Pathfinder framed blocker from current evidence", map[string]string{
		"problem":      truncateStructuredTimelineValue(problem.Problem),
		"real_blocker": truncateStructuredTimelineValue(packet.RealBlocker),
	})
	emitStructuredCommandEvent(onEvent, "pathfinder_candidates_scored", "Pathfinder scored candidate strategies", map[string]string{
		"count": fmt.Sprintf("%d", len(packet.CandidateStrategies)),
	})
	emitStructuredCommandEvent(onEvent, "pathfinder_strategy_selected", "Pathfinder selected next evidence-producing action", map[string]string{
		"strategy": packet.SelectedStrategy.ID,
		"kind":     packet.NextAction.Kind,
		"command":  truncateStructuredTimelineValue(packet.NextAction.Command),
	})
}

func runPathfinderForProgression(ctx context.Context, step int, prompt string, decision ProgressionDecision, cfg structuredCommandDecisionRunConfig, survey WorksiteSurvey, stdout, stderr io.Writer, onEvent func(StructuredCommandEvent), result *CommandDecisionResult) (bool, error) {
	problem := BuildProblemCaseFromProgression(prompt, "progression_gate", decision, cfg, survey, result.ObjectiveLedger, result.Observations)
	packet := Pathfinder{}.Solve(problem)
	emitPathfinderEvents(step, problem, packet, onEvent)
	if err := ValidateBreakthroughPacket(problem, packet); err != nil {
		emitStructuredCommandEvent(onEvent, "pathfinder_packet_rejected", "Pathfinder packet failed deterministic validation", map[string]string{
			"step":  fmt.Sprintf("%d", step),
			"error": truncateStructuredTimelineValue(err.Error()),
		})
		return false, nil
	}
	emitStructuredCommandEvent(onEvent, "pathfinder_packet_validated", "Pathfinder packet passed deterministic validation", map[string]string{
		"step": fmt.Sprintf("%d", step),
		"kind": packet.NextAction.Kind,
	})
	if packet.NextAction.Kind != string(PathfinderActionInspect) && packet.NextAction.Kind != string(PathfinderActionRunVerification) {
		return false, nil
	}
	command := strings.TrimSpace(packet.NextAction.Command)
	if command == "" {
		return false, nil
	}
	if err := validateStructuredCommandForRunWithArchitect(command, prompt, packet.NextAction.ToolTask, "", result.Observations, cfg.CurrentWorkingDirectory, result.ObjectiveLedger, survey); err != nil {
		emitStructuredCommandEvent(onEvent, "pathfinder_action_rejected", "Pathfinder action rejected by normal command policy", map[string]string{
			"step":    fmt.Sprintf("%d", step),
			"command": truncateStructuredTimelineValue(command),
			"reason":  truncateStructuredTimelineValue(err.Error()),
		})
		return false, nil
	}
	emitStructuredCommandEvent(onEvent, "pathfinder_action_dispatched", "Pathfinder action dispatched through normal runtime policy", map[string]string{
		"step":    fmt.Sprintf("%d", step),
		"command": truncateStructuredTimelineValue(command),
	})
	if err := runStructuredPayloadCommand(ctx, step, command, cfg.CurrentWorkingDirectory, cfg.EnableCommandCache, cfg.CommandCacheRoot, stdout, stderr, onEvent, result); err != nil {
		return true, err
	}
	emitStructuredCommandEvent(onEvent, "pathfinder_action_result", "Pathfinder action produced runtime observation", map[string]string{
		"step":      fmt.Sprintf("%d", step),
		"exit_code": fmt.Sprintf("%d", result.ExitCode),
	})
	return true, nil
}

func normalizeProblemCase(problem ProblemCase) ProblemCase {
	problem.Problem = strings.TrimSpace(problem.Problem)
	problem.CurrentGoal = strings.TrimSpace(problem.CurrentGoal)
	problem.CurrentPhase = strings.TrimSpace(problem.CurrentPhase)
	problem.SuccessCondition = strings.TrimSpace(problem.SuccessCondition)
	if problem.Problem == "" {
		problem.Problem = "Omnidex is not making forward progress"
	}
	if problem.CurrentPhase == "" {
		problem.CurrentPhase = "unknown"
	}
	if problem.SuccessCondition == "" {
		problem.SuccessCondition = "Select the smallest evidence-producing next action."
	}
	if len(problem.BlockedObjectives) == 0 {
		problem.BlockedObjectives = pendingStructuredObjectiveIDList(problem.ObjectiveLedger)
	}
	if problem.LoopState.Status == "" {
		problem.LoopState = structuredLoopStateFromState(problem.ObjectiveLedger, problem.RecentObservations)
	}
	return problem
}

func pathfinderCandidateStrategies(problem ProblemCase) []CandidateStrategy {
	candidates := []CandidateStrategy{
		{ID: "inspect_workspace_shape", Description: "Inspect package metadata and source tree to replace stale assumptions with concrete file evidence.", ActionKind: string(PathfinderActionInspect), ProgressGain: 7, EvidenceValue: 9, Confidence: 8, Cost: 2, Risk: 1, Reversibility: 10, ScopeSafety: 10, TimeEstimateBucket: "short"},
		{ID: "patch_targeted_source", Description: "Patch the smallest relevant source file once a valid target is known.", ActionKind: string(PathfinderActionPatch), ProgressGain: 8, EvidenceValue: 6, Confidence: 6, Cost: 5, Risk: 4, Reversibility: 7, ScopeSafety: 7, TimeEstimateBucket: "medium"},
		{ID: "create_proof_contract", Description: "Create or repair a narrow proof contract before more implementation work.", ActionKind: string(PathfinderActionCreateOrUpdateProofContract), ProgressGain: 6, EvidenceValue: 8, Confidence: 7, Cost: 4, Risk: 2, Reversibility: 8, ScopeSafety: 9, TimeEstimateBucket: "medium"},
	}
	if strings.Contains(strings.ToLower(problem.Problem+" "+problem.LoopState.LastBlocker), "artifact") || latestArtifactValidationFailure(problem.RecentObservations) {
		candidates = append(candidates, CandidateStrategy{ID: "repair_artifact_source", Description: "Patch empty or placeholder artifacts before another completion attempt.", ActionKind: string(PathfinderActionPatch), ProgressGain: 9, EvidenceValue: 7, Confidence: 8, Cost: 4, Risk: 3, Reversibility: 7, ScopeSafety: 8, TimeEstimateBucket: "medium"})
	}
	if problem.FalseDoneCount > 0 || strings.Contains(strings.ToLower(problem.Problem), "done") {
		candidates = append(candidates, CandidateStrategy{ID: "tighten_completion_proof", Description: "Reject model-authored completion and require deterministic proof before another done attempt.", ActionKind: string(PathfinderActionTightenRuntimeBlocker), ProgressGain: 5, EvidenceValue: 8, Confidence: 8, Cost: 2, Risk: 1, Reversibility: 9, ScopeSafety: 10, TimeEstimateBucket: "short"})
	}
	if strings.Contains(strings.ToLower(problem.Problem), "exhausted") {
		candidates = append(candidates, CandidateStrategy{ID: "fail_with_evidence", Description: "Stop the loop and report the concrete blocker when no safe action remains.", ActionKind: string(PathfinderActionFailWithEvidence), ProgressGain: 3, EvidenceValue: 7, Confidence: 8, Cost: 1, Risk: 1, Reversibility: 5, ScopeSafety: 10, TimeEstimateBucket: "short"})
	}
	return candidates
}

func scoreCandidateStrategies(candidates []CandidateStrategy) []CandidateStrategy {
	for i := range candidates {
		candidates[i].Score = candidates[i].ProgressGain + candidates[i].EvidenceValue + candidates[i].Confidence + candidates[i].Reversibility + candidates[i].ScopeSafety - candidates[i].Cost - candidates[i].Risk
	}
	return candidates
}

func selectPathfinderStrategy(problem ProblemCase, candidates []CandidateStrategy) CandidateStrategy {
	if len(candidates) == 0 {
		return CandidateStrategy{ID: "fail_with_evidence", ActionKind: string(PathfinderActionFailWithEvidence), Confidence: 5}
	}
	if latestENOENTObservation(problem.RecentObservations) != nil || strings.Contains(strings.ToLower(problem.Problem), "file path was invalid") {
		return candidateByID(candidates, "inspect_workspace_shape", candidates[0])
	}
	if latestArtifactValidationFailure(problem.RecentObservations) {
		return candidateByID(candidates, "repair_artifact_source", candidates[0])
	}
	best := candidates[0]
	for _, candidate := range candidates[1:] {
		if candidate.Score > best.Score {
			best = candidate
		}
	}
	return best
}

func pathfinderNextAction(problem ProblemCase, selected CandidateStrategy) PathfinderNextAction {
	switch selected.ActionKind {
	case string(PathfinderActionInspect):
		return PathfinderNextAction{
			Kind:      selected.ActionKind,
			Command:   `find . -maxdepth 3 \( -path './node_modules' -o -path './.git' -o -path './dist' -o -path './build' -o -path './coverage' -o -path './.omni' \) -prune -o -type f \( -name 'package.json' -o -path './src/*' -o -name 'go.mod' -o -name 'Cargo.toml' \) -print | sort | head -80`,
			ToolTask:  "Inspect package metadata and source files to replace stale file assumptions with current workspace evidence.",
			Rationale: "A bounded read-only inventory changes the state of the problem without mutating the workspace.",
		}
	case string(PathfinderActionRunVerification):
		return PathfinderNextAction{Kind: selected.ActionKind, Command: "npm run build", ToolTask: "Run the narrowest available build proof.", Rationale: "A verifier command produces deterministic evidence."}
	case string(PathfinderActionFailWithEvidence):
		return PathfinderNextAction{Kind: selected.ActionKind, Rationale: "No safe action remains after repeated recovery exhaustion."}
	default:
		return PathfinderNextAction{Kind: selected.ActionKind, ToolTask: selected.Description, Rationale: "Return to the normal runtime with a narrower strategy."}
	}
}

func pathfinderDiagnosis(problem ProblemCase) string {
	text := strings.ToLower(problem.Problem + " " + problem.LoopState.LastBlocker)
	switch {
	case strings.Contains(text, "file path was invalid") || latestENOENTObservation(problem.RecentObservations) != nil:
		return "The current route is relying on a file/path assumption that workspace evidence has contradicted."
	case strings.Contains(text, "done"):
		return "The loop attempted completion before objective-specific proof was present."
	case strings.Contains(text, "artifact"):
		return "Artifact validation found source/test/config output that is not substantive enough for completion."
	case strings.Contains(text, "same command") || strings.Contains(text, "repeated"):
		return "The loop is repeating a strategy that does not change objective evidence."
	default:
		return "The normal execution path stalled and needs a smaller evidence-producing action."
	}
}

func pathfinderRealBlocker(problem ProblemCase) string {
	if latestENOENTObservation(problem.RecentObservations) != nil {
		return "The runtime has not remapped the actual source tree after a missing-file failure."
	}
	if latestArtifactValidationFailure(problem.RecentObservations) {
		return "The workspace contains empty or placeholder artifacts that cannot satisfy completion gates."
	}
	if problem.FalseDoneCount > 0 {
		return "Completion was requested while pending objectives still lack deterministic evidence."
	}
	if strings.TrimSpace(problem.LoopState.LastBlocker) != "" {
		return problem.LoopState.LastBlocker
	}
	return problem.Problem
}

func pathfinderExpectedProgress(problem ProblemCase, selected CandidateStrategy) string {
	switch selected.ActionKind {
	case string(PathfinderActionInspect):
		return "Replace stale assumptions with concrete package/source file evidence."
	case string(PathfinderActionPatch):
		return "Produce a scoped source mutation that can be checked by artifact and proof gates."
	case string(PathfinderActionCreateOrUpdateProofContract):
		return "Clarify the evidence required before implementation or completion can proceed."
	default:
		return "Move the run out of the exhausted strategy path."
	}
}

func pathfinderRisk(problem ProblemCase, selected CandidateStrategy) string {
	if selected.Risk <= 1 {
		return "low: read-only or policy-tightening action"
	}
	if selected.Risk <= 3 {
		return "moderate: scoped runtime change requires validation"
	}
	return "high: mutation or delegation requires normal proof gates"
}

func pathfinderProofNeededAfter(problem ProblemCase, selected CandidateStrategy) []string {
	proof := []string{"pathfinder action produced a runtime observation"}
	switch selected.ActionKind {
	case string(PathfinderActionInspect):
		proof = append(proof, "source files identified", "next patch target maps to observed workspace")
	case string(PathfinderActionPatch):
		proof = append(proof, "artifact_validation_passed", "source files are non-empty", "configured build/test proof passes")
	case string(PathfinderActionCreateOrUpdateProofContract):
		proof = append(proof, "proof contract maps to pending objectives", "proof command/probe is executable")
	default:
		proof = append(proof, "objective evidence predicates remain pending until independently satisfied")
	}
	return proof
}

func pathfinderAssumptions(problem ProblemCase) []string {
	return []string{
		"Recent observations reflect the current run state.",
		"Memory and prep context are advisory, not execution authority.",
		"Completion remains blocked until deterministic evidence predicates pass.",
	}
}

func pathfinderForbiddenPaths(problem ProblemCase) []string {
	out := []string{
		"Do not declare done from Pathfinder output.",
		"Do not rerun an exhausted command without new state.",
		"Do not bypass command policy or artifact validation.",
	}
	if latestENOENTObservation(problem.RecentObservations) != nil {
		out = append(out, "Do not retry the missing path before remapping the source tree.")
	}
	return out
}

func pathfinderEvidence(problem ProblemCase) []string {
	out := []string{}
	if strings.TrimSpace(problem.Problem) != "" {
		out = append(out, "problem:"+problem.Problem)
	}
	if strings.TrimSpace(problem.LoopState.LastBlocker) != "" {
		out = append(out, "loop_state:"+problem.LoopState.LastBlocker)
	}
	for _, objective := range problem.BlockedObjectives {
		out = append(out, "pending_objective:"+objective)
	}
	for _, command := range problem.FailedCommands {
		out = append(out, "failed_command:"+command)
	}
	for _, command := range problem.RejectedCommands {
		out = append(out, "rejected_command:"+command)
	}
	for _, evidence := range problem.WorksiteSurvey.Evidence {
		out = append(out, "worksite:"+evidence)
	}
	if len(out) == 0 && len(problem.RecentObservations) > 0 {
		latest := problem.RecentObservations[len(problem.RecentObservations)-1]
		out = append(out, "latest_observation:"+firstNonEmpty(latest.Stderr, latest.Stdout, latest.Command, latest.RejectedCommand))
	}
	return appendUniqueStrings(nil, out...)
}

func allowedPathfinderActionKind(kind string) bool {
	switch PathfinderActionKind(strings.TrimSpace(kind)) {
	case PathfinderActionInspect, PathfinderActionResearch, PathfinderActionCreateOrUpdateProofContract, PathfinderActionPatch, PathfinderActionRunVerification, PathfinderActionDelegateExternalAgent, PathfinderActionAdjustObjectiveLedger, PathfinderActionAdjustContextPlan, PathfinderActionRelaxRuntimeBlocker, PathfinderActionTightenRuntimeBlocker, PathfinderActionAskUser, PathfinderActionFailWithEvidence:
		return true
	default:
		return false
	}
}

func selectedMatchesExhaustedCommand(problem ProblemCase, packet BreakthroughPacket) bool {
	command := normalizeStructuredCommandForComparison(packet.NextAction.Command)
	if command == "" {
		return false
	}
	for _, rejected := range append(problem.RejectedCommands, problem.FailedCommands...) {
		if command == normalizeStructuredCommandForComparison(rejected) {
			return true
		}
	}
	return false
}

func contradictsPathfinderConstraints(problem ProblemCase, packet BreakthroughPacket) bool {
	text := strings.ToLower(packet.NextAction.Command + " " + packet.NextAction.ToolTask + " " + packet.SelectedStrategy.Description)
	for _, constraint := range problem.Constraints {
		c := strings.ToLower(strings.TrimSpace(constraint))
		if c == "" {
			continue
		}
		if strings.Contains(c, "do not add router") && strings.Contains(text, "react-router") {
			return true
		}
		if strings.Contains(c, "no backend") && (strings.Contains(text, "express") || strings.Contains(text, "database")) {
			return true
		}
	}
	return false
}

func pendingStructuredObjectiveIDList(ledger []StructuredObjective) []string {
	out := []string{}
	for _, objective := range pendingStructuredObjectives(ledger) {
		if strings.TrimSpace(objective.ID) != "" {
			out = append(out, objective.ID)
		}
	}
	return out
}

func failedCommandList(observations []StructuredCommandObservation, limit int) []string {
	out := []string{}
	for i := len(observations) - 1; i >= 0 && len(out) < limit; i-- {
		obs := observations[i]
		if obs.ExitCode != 0 && strings.TrimSpace(obs.Command) != "" {
			out = append(out, obs.Command)
		}
	}
	return out
}

func rejectedCommandList(observations []StructuredCommandObservation, limit int) []string {
	out := []string{}
	for i := len(observations) - 1; i >= 0 && len(out) < limit; i-- {
		if command := strings.TrimSpace(observations[i].RejectedCommand); command != "" {
			out = append(out, command)
		}
	}
	return out
}

func falseDoneObservationCount(observations []StructuredCommandObservation) int {
	count := 0
	for _, obs := range observations {
		if strings.Contains(strings.ToLower(obs.Stderr), "done=true rejected") || strings.Contains(strings.ToLower(obs.Stderr), "done rejected") {
			count++
		}
	}
	return count
}

func latestArtifactValidationFailure(observations []StructuredCommandObservation) bool {
	for i := len(observations) - 1; i >= 0; i-- {
		if strings.Contains(strings.ToLower(observations[i].Stderr), "artifact_validation_failed") {
			return true
		}
	}
	return false
}

func candidateByID(candidates []CandidateStrategy, id string, fallback CandidateStrategy) CandidateStrategy {
	for _, candidate := range candidates {
		if candidate.ID == id {
			return candidate
		}
	}
	return fallback
}

func pathfinderPrepSummary(prep PrepContextBundle) []string {
	out := []string{}
	for _, brief := range allPrepBriefs(prep) {
		if strings.TrimSpace(brief.Content) != "" {
			out = append(out, brief.Kind+": "+truncateStructuredTimelineValue(brief.Content))
		}
		if len(out) >= 4 {
			break
		}
	}
	return out
}
