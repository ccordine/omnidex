package worker

import (
	"context"
	"fmt"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/specialist"
	"path/filepath"
	"sort"
	"strings"
)

func (s *Service) runVerifyStep(ctx context.Context, claim *model.ClaimedStep, contexts map[string]string) error {
	responseKey := responseContextKeyForPipeline(claim.Job.Pipeline)
	responseDraft := strings.TrimSpace(contexts[responseKey])
	responseFallback := s.specialistModel(claim.Job, specialist.RoleResponseSpecialist, s.models.Response)
	responseModel := metadataModel(claim.Job, "model_response", responseFallback)
	codeOnly := shouldForceCodeOnlyResponse(claim.Job, contexts, responseModel)
	s.emitStepEvent(claim.Step.ID, "verify_begin", fmt.Sprintf("pipeline=%s", strings.ToLower(strings.TrimSpace(claim.Job.Pipeline))))
	if responseDraft == "" {
		responseDraft = strings.TrimSpace(contexts["response_draft"])
	}
	if responseDraft == "" {
		summary := "verification skipped: no response draft available"
		if !codeOnly {
			summary = ensureResponseHasSources(summary, claim.Job, contexts, nil)
		}
		s.emitStepEvent(claim.Step.ID, "verify_ready", "status=skipped reason=no_response_draft")
		return s.repo.CompleteStep(ctx, claim.Step.ID, summary, "verification", summary)
	}
	if deterministicOutcome, deterministicResponse, ok := evaluateDeterministicLocalActionReview(claim.Job.Instruction); ok {
		report := testReport{
			NotRunReason: "deterministic local-action review",
		}
		verificationSummary := trimForBudget(buildVerificationSummary(deterministicOutcome, report), s.contextBudget)
		finalOutput := strings.TrimSpace(deterministicResponse)
		if codeOnly {
			finalOutput = normalizeCodeOnlyResponse(finalOutput)
		} else {
			finalOutput = ensureResponseHasSources(finalOutput, claim.Job, contexts, &report)
		}
		s.emitStepEvent(claim.Step.ID, "verify_ready", fmt.Sprintf("status=%s attempted=%d failed=%d skipped=%d", deterministicOutcome.Status, report.Attempted, report.Failed, report.Skipped))
		s.emitStepEvent(claim.Step.ID, "verify_deterministic_local_action", "shortcut=true")
		return s.repo.CompleteStep(ctx, claim.Step.ID, finalOutput, "verification", verificationSummary)
	}

	inputs := append([]string{claim.Job.Instruction}, collectContextValuesByKey(claim.Contexts, "user_feedback", "replan_feedback")...)
	directive := parseTestDirective(inputs)

	testMode := resolveVerificationMode(claim.Job.Metadata)
	testTimeout := metadataInt(claim.Job.Metadata, "test_timeout_seconds", verifyDefaultTestTimeoutSeconds)
	if testTimeout < 15 {
		testTimeout = 15
	}
	s.emitStepEvent(claim.Step.ID, "verify_tests", fmt.Sprintf("mode=%s timeout_seconds=%d", testMode, testTimeout))
	testReport := s.runVerificationTests(ctx, claim, contexts, directive, testMode, testTimeout)
	actionAudit := buildVerificationActionAudit(claim.Job, contexts)
	if strings.TrimSpace(actionAudit.Report) != "" {
		contexts["verification_action_audit"] = actionAudit.Report
		s.emitStepContext(claim.Step.ID, "verify_action_audit", trimForBudget(actionAudit.Report, s.contextBudget))
	}

	maxIterations := metadataInt(claim.Job.Metadata, "verification_iterations", verifyDefaultIterations)
	if maxIterations < 1 {
		maxIterations = 1
	}
	if maxIterations > verifyMaxIterations {
		maxIterations = verifyMaxIterations
	}
	reviewAlways := reviewAlwaysEnabled(claim.Job)
	verificationPasses := verificationPassCount(claim.Job)
	hallucinationRetryLimit := verificationHallucinationRetryLimit(claim.Job, s.hallucinationRetryLimit)
	hallucinationRetries := 0
	hallucinationLoopMessage := ""
	s.emitStepEvent(claim.Step.ID, "verify_consensus_config", fmt.Sprintf("passes=%d", verificationPasses))
	s.emitStepEvent(claim.Step.ID, "verify_hallucination_limit", fmt.Sprintf("retries=%d", hallucinationRetryLimit))

	finalResponse := responseDraft
	finalOutcome := verificationOutcome{
		Status:               "blocked",
		Confidence:           0,
		Summary:              "verification did not complete",
		CannotCompleteReason: "verification produced no accepted evaluator outcome",
	}
	for attempt := 1; attempt <= maxIterations; attempt++ {
		outcome, consensusNote, consensusErr := s.evaluateVerificationConsensus(
			ctx,
			claim,
			contexts,
			finalResponse,
			testReport,
			attempt,
			maxIterations,
			verificationPasses,
		)
		if consensusErr != nil {
			s.emitStepEvent(claim.Step.ID, "verify_consensus_failed", fmt.Sprintf("attempt=%d error=%s", attempt, trimForBudget(consensusErr.Error(), 260)))
			return fmt.Errorf("verification consensus failed: %w", consensusErr)
		}
		finalOutcome = normalizeVerificationOutcome(outcome, testReport)
		s.emitStepEvent(claim.Step.ID, "verify_consensus", fmt.Sprintf("attempt=%d status=%s note=%s", attempt, finalOutcome.Status, trimForBudget(consensusNote, 220)))
		s.emitStepContext(claim.Step.ID, "verify_consensus", fmt.Sprintf("attempt=%d %s", attempt, trimForBudget(consensusNote, 1500)))
		var reviewSignals []string
		if reviewAlways {
			finalOutcome, reviewSignals = enforceGroundingReview(finalOutcome, claim.Job, finalResponse, contexts, testReport)
			if len(reviewSignals) > 0 {
				s.emitStepEvent(claim.Step.ID, "verify_grounding_retry", fmt.Sprintf("attempt=%d signals=%d", attempt, len(reviewSignals)))
				s.emitStepContext(claim.Step.ID, "verify_grounding_signals", strings.Join(reviewSignals, " | "))
			}
		}
		if feedback, missing, ok := autoVerifyReplanFeedback(claim.Job, contexts, claim.Contexts, finalOutcome); ok {
			s.emitStepEvent(claim.Step.ID, "verify_auto_replan", fmt.Sprintf("attempt=%d missing_actions=%s", attempt, strings.Join(missing, ",")))
			s.emitStepContext(claim.Step.ID, "verify_auto_replan_feedback", trimForBudget(feedback, s.contextBudget))
			if _, err := s.repo.ReplanJob(ctx, claim.Job.ID, feedback); err != nil {
				finalOutcome.Status = "blocked"
				finalOutcome.Summary = "verification requested replan but restart failed"
				finalOutcome.CannotCompleteReason = trimForBudget(err.Error(), 260)
				break
			}
			return context.Canceled
		}
		if detected, reason := hallucinationRetrySignal(consensusNote, reviewSignals, finalOutcome); detected {
			hallucinationRetries++
			s.emitStepEvent(claim.Step.ID, "verify_hallucination_retry", fmt.Sprintf("attempt=%d retries=%d/%d reason=%s", attempt, hallucinationRetries, hallucinationRetryLimit, trimForBudget(reason, 180)))
			if hallucinationRetries >= hallucinationRetryLimit {
				s.emitStepEvent(claim.Step.ID, "verify_hallucination_loop", fmt.Sprintf("attempt=%d retries=%d/%d", attempt, hallucinationRetries, hallucinationRetryLimit))
				restartNote, restartErr := s.restartOllamaForHallucinationLoop(ctx, claim)
				if restartNote != "" {
					s.emitStepContext(claim.Step.ID, "verify_ollama_restart", trimForBudget(restartNote, s.contextBudget))
				}
				if restartErr != nil {
					s.emitStepEvent(claim.Step.ID, "verify_ollama_restart_failed", trimForBudget(restartErr.Error(), 260))
				} else {
					s.emitStepEvent(claim.Step.ID, "verify_ollama_restart_ok", "restart=ok")
				}

				finalOutcome.Status = "blocked"
				finalOutcome.Summary = "hallucination loop detected during verification"
				if restartErr != nil {
					finalOutcome.CannotCompleteReason = "hallucination loop detected; automatic ollama restart failed"
					if restartNote != "" {
						finalOutcome.Gaps = dedupeStrings(append(finalOutcome.Gaps, "ollama restart failure: "+trimForBudget(restartNote, 260)))
					}
				} else {
					finalOutcome.CannotCompleteReason = "hallucination loop detected; ollama restart attempted"
				}
				hallucinationLoopMessage = hallucinationLoopUserMessage(restartErr)
				finalResponse = hallucinationLoopMessage
				break
			}
		}
		if finalOutcome.Status != "retry" || attempt == maxIterations {
			break
		}
		s.emitStepEvent(claim.Step.ID, "verification_retry", fmt.Sprintf("attempt=%d/%d", attempt+1, maxIterations))
		revised, err := s.reviseResponseForVerification(ctx, claim, contexts, finalResponse, finalOutcome, testReport, attempt, maxIterations, codeOnly)
		if err != nil {
			finalOutcome.Status = "blocked"
			if finalOutcome.Summary == "" {
				finalOutcome.Summary = "verification retry failed"
			}
			finalOutcome.CannotCompleteReason = trimForBudget(err.Error(), 300)
			break
		}
		revised = strings.TrimSpace(revised)
		if revised == "" {
			finalOutcome.Status = "blocked"
			if finalOutcome.Summary == "" {
				finalOutcome.Summary = "verification retry produced empty response"
			}
			break
		}
		finalResponse = revised
	}

	if finalOutcome.Status == "retry" {
		finalOutcome.Status = "blocked"
		if finalOutcome.Summary == "" {
			finalOutcome.Summary = "verification did not converge within retry budget"
		}
		if finalOutcome.CannotCompleteReason == "" {
			finalOutcome.CannotCompleteReason = "max verification iterations reached"
		}
	}

	verificationSummary := trimForBudget(buildVerificationSummary(finalOutcome, testReport), s.contextBudget)
	pipeline := strings.ToLower(strings.TrimSpace(claim.Job.Pipeline))
	sanitizedResponse := finalResponse
	if pipeline == model.PipelineAssistant {
		sanitizedResponse = sanitizeResponseTestClaims(finalResponse, testReport)
	}
	finalOutput := sanitizedResponse
	if strings.TrimSpace(hallucinationLoopMessage) != "" {
		finalOutput = hallucinationLoopMessage
	} else if pipeline == model.PipelineAssistant && !codeOnly {
		executedEvidence := trimForBudget(buildExecutedTestEvidence(testReport), s.contextBudget)
		finalOutput = strings.TrimSpace(strings.Join([]string{
			sanitizedResponse,
			"",
			"Executed Test Evidence",
			executedEvidence,
			"",
			"Verification",
			verificationSummary,
		}, "\n"))
	}
	if codeOnly {
		finalOutput = normalizeCodeOnlyResponse(finalOutput)
	} else {
		finalOutput = ensureResponseHasSources(finalOutput, claim.Job, contexts, &testReport)
	}
	s.emitStepEvent(claim.Step.ID, "verify_ready", fmt.Sprintf("status=%s attempted=%d failed=%d skipped=%d", finalOutcome.Status, testReport.Attempted, testReport.Failed, testReport.Skipped))

	return s.repo.CompleteStep(ctx, claim.Step.ID, finalOutput, "verification", verificationSummary)
}

type deterministicLocalActionReviewInput struct {
	OriginalRequest string
	CapabilityKind  string
	ActionOutput    string
}

func evaluateDeterministicLocalActionReview(instruction string) (verificationOutcome, string, bool) {
	input, ok := parseDeterministicLocalActionReviewInput(instruction)
	if !ok {
		return verificationOutcome{}, "", false
	}
	if !strings.EqualFold(strings.TrimSpace(input.CapabilityKind), "local_shell") {
		return verificationOutcome{}, "", false
	}
	return evaluateDeterministicLocalShellReview(input)
}

func parseDeterministicLocalActionReviewInput(instruction string) (deterministicLocalActionReviewInput, bool) {
	normalized := strings.ReplaceAll(strings.TrimSpace(instruction), "\r\n", "\n")
	if normalized == "" {
		return deterministicLocalActionReviewInput{}, false
	}
	if !strings.Contains(strings.ToLower(normalized), "deterministic post-action review step (required):") {
		return deterministicLocalActionReviewInput{}, false
	}

	sections := map[string][]string{}
	current := ""
	for _, line := range strings.Split(normalized, "\n") {
		switch strings.ToLower(strings.TrimSpace(line)) {
		case "original user request:":
			current = "request"
			continue
		case "local capability kind:":
			current = "kind"
			continue
		case "executed local action output:":
			current = "output"
			continue
		}
		if current == "" {
			continue
		}
		sections[current] = append(sections[current], line)
	}

	input := deterministicLocalActionReviewInput{
		OriginalRequest: strings.TrimSpace(strings.Join(sections["request"], "\n")),
		CapabilityKind:  strings.TrimSpace(strings.Join(sections["kind"], "\n")),
		ActionOutput:    strings.TrimSpace(strings.Join(sections["output"], "\n")),
	}
	if input.ActionOutput == "" {
		return deterministicLocalActionReviewInput{}, false
	}
	return input, true
}

func evaluateDeterministicLocalShellReview(input deterministicLocalActionReviewInput) (verificationOutcome, string, bool) {
	output := strings.TrimSpace(input.ActionOutput)
	if output == "" {
		return verificationOutcome{}, "", false
	}

	if reason, failed := deterministicLocalShellFailureReason(output); failed {
		outcome := normalizeVerificationOutcome(verificationOutcome{
			Status:               "blocked",
			Confidence:           0.98,
			Summary:              "local shell action failed",
			Gaps:                 []string{reason},
			CannotCompleteReason: reason,
		}, testReport{})
		response := "INCOMPLETE: Local shell action failed: " + reason
		return outcome, strings.TrimSpace(response), true
	}

	requested := parseRequestedFileTarget(input.OriginalRequest)
	executed := parseExecutedFileTarget(output)
	hasSuccessEvidence := hasLocalShellSuccessEvidence(output)
	if !hasSuccessEvidence {
		return verificationOutcome{}, "", false
	}

	if requested != "" && executed != "" && !sameFileTarget(requested, executed) {
		next := fmt.Sprintf("touch %q", requested)
		outcome := normalizeVerificationOutcome(verificationOutcome{
			Status:     "retry",
			Confidence: 0.20,
			Summary:    "executed file target did not match requested file target",
			Gaps:       []string{"requested target mismatch with executed action"},
		}, testReport{})
		response := strings.TrimSpace(strings.Join([]string{
			"INCOMPLETE: The executed file target did not match the requested file.",
			fmt.Sprintf("Requested target: `%s`", requested),
			fmt.Sprintf("Executed target: `%s`", executed),
			fmt.Sprintf("Next required action: `%s`", next),
		}, "\n"))
		return outcome, response, true
	}

	target := strings.TrimSpace(executed)
	if target == "" {
		target = strings.TrimSpace(requested)
	}
	displayTarget := target
	if displayTarget == "" {
		displayTarget = "requested file"
	}

	responseLines := []string{
		fmt.Sprintf("COMPLETE: The local shell action succeeded and `%s` is present based on execution evidence.", displayTarget),
	}
	if target != "" {
		responseLines = append(responseLines, fmt.Sprintf("Verification command: `ls -l %q`", target))
	}
	outcome := normalizeVerificationOutcome(verificationOutcome{
		Status:     "pass",
		Confidence: 0.98,
		Summary:    "deterministic local shell evidence confirms task completion",
	}, testReport{})
	return outcome, strings.TrimSpace(strings.Join(responseLines, "\n\n")), true
}

func deterministicLocalShellFailureReason(output string) (string, bool) {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	for _, line := range lines {
		clean := strings.TrimSpace(line)
		lower := strings.ToLower(clean)
		switch {
		case strings.HasPrefix(lower, "local shell action failed:"):
			return strings.TrimSpace(clean[len("Local shell action failed:"):]), true
		case strings.HasPrefix(lower, "local shell action blocked:"):
			return strings.TrimSpace(clean[len("Local shell action blocked:"):]), true
		}
	}
	return "", false
}

func parseRequestedFileTarget(request string) string {
	request = strings.TrimSpace(request)
	if request == "" {
		return ""
	}
	for _, match := range backtickedTokenPattern.FindAllStringSubmatch(request, -1) {
		if len(match) < 2 {
			continue
		}
		if candidate := sanitizeFileTargetToken(match[1]); looksLikeFileTarget(candidate) {
			return candidate
		}
	}
	for _, token := range filePathTokenPattern.FindAllString(request, -1) {
		if candidate := sanitizeFileTargetToken(token); looksLikeFileTarget(candidate) {
			return candidate
		}
	}
	return ""
}

func parseExecutedFileTarget(output string) string {
	for _, prefix := range []string{"Created file:", "File already exists:", "Renamed file to:"} {
		if value := extractOutputValueByPrefix(output, prefix); value != "" {
			return sanitizeFileTargetToken(value)
		}
	}
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		clean := strings.TrimSpace(line)
		lower := strings.ToLower(clean)
		if !strings.HasPrefix(lower, "executed: touch ") {
			continue
		}
		value := strings.TrimSpace(clean[len("Executed: touch "):])
		if value == "" {
			continue
		}
		fields := strings.Fields(value)
		if len(fields) == 0 {
			continue
		}
		return sanitizeFileTargetToken(fields[0])
	}
	return ""
}

func hasLocalShellSuccessEvidence(output string) bool {
	lower := strings.ToLower(strings.TrimSpace(output))
	if lower == "" {
		return false
	}
	for _, marker := range []string{
		"created file:",
		"file already exists:",
		"renamed file to:",
		"executed: touch ",
		"executed: mv ",
		"executed: mkdir -p ",
	} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func extractOutputValueByPrefix(output string, prefix string) string {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return ""
	}
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		clean := strings.TrimSpace(line)
		if strings.HasPrefix(strings.ToLower(clean), strings.ToLower(prefix)) {
			return strings.TrimSpace(clean[len(prefix):])
		}
	}
	return ""
}

func sanitizeFileTargetToken(value string) string {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, " \t\r\n\"'`.,;:!?()[]{}")
	return value
}

func looksLikeFileTarget(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	if strings.Contains(value, "://") {
		return false
	}
	base := filepath.Base(value)
	return strings.Contains(base, ".")
}

func sameFileTarget(left string, right string) bool {
	left = sanitizeFileTargetToken(left)
	right = sanitizeFileTargetToken(right)
	if left == "" || right == "" {
		return false
	}
	if strings.EqualFold(filepath.Clean(left), filepath.Clean(right)) {
		return true
	}
	return strings.EqualFold(filepath.Base(left), filepath.Base(right))
}

func responseContextKeyForPipeline(pipeline string) string {
	switch strings.ToLower(strings.TrimSpace(pipeline)) {
	case model.PipelineChat:
		return "roleplay"
	case model.PipelineStory:
		return "narrate"
	default:
		return "assist"
	}
}

func plannerActionCatalog(job model.Job) string {
	specialistAssignments := plannerSpecialistAssignments(job)
	lines := []string{
		"Core pipeline actions:",
		"- plan: generate an execution plan JSON (goal/tasks/required_tools/clarifications/done_when)",
		"- tooling: evaluate tool availability, install hints, and safety/risk signals",
		"- workspace_scan: inspect repository files when code/project context is needed",
		"- tag: classify instruction intent for retrieval and routing",
		"- retrieve: pull relevant memory context from prior runs",
		"- web_search: fetch external information when required or time-sensitive",
		"- analyze: synthesize context into response guidance",
		"- " + responseContextKeyForPipeline(job.Pipeline) + ": draft the user-facing response",
		"- verify: validate/refine response and run tests when appropriate",
		"",
		"Execution defaults:",
		"- internet/web access is available by default for this run",
		"- treat internet as unavailable only when tooling/environment/output indicates network failure",
		"- use web_search creatively when external or current information can improve results",
		"",
		"Pipeline specialist assignments:",
		specialistAssignments,
		"",
		"Host capability actions (derived from discovered tools):",
	}

	hostActions := deriveHostCapabilityActionsFromMetadata(job)
	if len(hostActions) == 0 {
		lines = append(lines, "- (none discovered from host metadata)")
	} else {
		for _, action := range hostActions {
			lines = append(lines, "- "+action)
		}
	}

	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func plannerPipelineActionsForJob(job model.Job) []string {
	return []string{
		"tooling",
		"workspace_scan",
		"tag",
		"retrieve",
		"plan",
		"web_search",
		"analyze",
		responseContextKeyForPipeline(job.Pipeline),
		"verify",
	}
}

func plannerSpecialistAssignments(job model.Job) string {
	actions := plannerPipelineActionsForJob(job)
	lines := make([]string, 0, len(actions))
	for _, action := range actions {
		role := specialist.ForPipelineAction(action)
		lines = append(lines, fmt.Sprintf("- %s -> %s", strings.TrimSpace(action), specialist.Summary(role)))
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func deriveHostCapabilityActionsFromMetadata(job model.Job) []string {
	toolSet := hostToolSetFromMetadata(job)
	if len(toolSet) == 0 {
		return nil
	}

	has := func(tools ...string) bool {
		for _, tool := range tools {
			normalized := strings.ToLower(strings.TrimSpace(tool))
			if normalized == "" {
				continue
			}
			if _, ok := toolSet[normalized]; ok {
				return true
			}
		}
		return false
	}

	actions := make([]string, 0, 24)
	add := func(action string) {
		action = strings.TrimSpace(action)
		if action == "" {
			return
		}
		actions = append(actions, action)
	}

	if has("sh", "bash", "zsh") {
		add("local_shell.run_command")
		add("local_shell.file_create_rename")
	}
	if has("git") {
		add("repo.inspect_and_diff")
	}
	if has("go") {
		add("repo.go_build_and_test")
	}
	if has("npm", "pnpm", "yarn", "node", "nodejs") {
		add("repo.node_dependency_and_test")
	}
	if has("python3", "python", "pip", "pip3", "pytest") {
		add("repo.python_dependency_and_test")
	}
	if has("docker", "docker-compose", "podman") {
		add("container.build_and_compose_control")
	}
	if has("vlc", "playerctl") {
		add("media.playback_control_and_next_episode")
	}
	if has("ffmpeg") {
		add("media.subtitle_audio_video_processing")
	}
	if has("ip", "ifconfig", "ss", "netstat", "lsof") {
		add("network.local_ip_and_open_ports_inspection")
	}
	if has("dig", "nslookup", "host", "traceroute", "mtr", "whois", "nmap") {
		add("network.dns_route_whois_scan_diagnostics")
	}
	if has("nmcli", "wg", "openvpn") {
		add("network.vpn_detection_and_status")
	}
	packageManagers := resolvePackageManagers(job)
	if len(packageManagers) > 0 {
		add("system.package_install_via_" + strings.Join(packageManagers, "|"))
	}

	sort.Strings(actions)
	out := make([]string, 0, len(actions))
	seen := map[string]struct{}{}
	for _, action := range actions {
		if _, ok := seen[action]; ok {
			continue
		}
		seen[action] = struct{}{}
		out = append(out, action)
	}
	return out
}

func collectContextValuesByKey(contexts []model.StepContext, keys ...string) []string {
	if len(contexts) == 0 || len(keys) == 0 {
		return nil
	}
	lookup := map[string]struct{}{}
	for _, key := range keys {
		clean := strings.TrimSpace(key)
		if clean == "" {
			continue
		}
		lookup[clean] = struct{}{}
	}
	if len(lookup) == 0 {
		return nil
	}

	out := make([]string, 0, len(contexts))
	for _, ctxValue := range contexts {
		if _, ok := lookup[ctxValue.Key]; !ok {
			continue
		}
		value := strings.TrimSpace(ctxValue.Value)
		if value == "" {
			continue
		}
		out = append(out, value)
	}
	return dedupeStrings(out)
}

func parseTestDirective(inputs []string) testDirective {
	directive := testDirective{
		Focus: map[string]struct{}{},
	}
	combined := strings.ToLower(strings.TrimSpace(strings.Join(inputs, "\n")))
	directive.Skip = skipTestsPattern.MatchString(combined)

	for _, input := range inputs {
		if strings.TrimSpace(input) == "" {
			continue
		}
		matches := testLinePattern.FindAllStringSubmatch(input, -1)
		for _, match := range matches {
			if len(match) < 2 {
				continue
			}
			line := strings.TrimSpace(match[1])
			if line == "" {
				continue
			}
			directive.Notes = append(directive.Notes, line)
		}
	}
	directive.Notes = dedupeStrings(directive.Notes)

	if strings.Contains(combined, "go test") || strings.Contains(combined, "golang test") {
		directive.Focus["go"] = struct{}{}
	}
	if strings.Contains(combined, "npm test") ||
		strings.Contains(combined, "pnpm test") ||
		strings.Contains(combined, "yarn test") ||
		strings.Contains(combined, "jest") ||
		strings.Contains(combined, "vitest") {
		directive.Focus["node"] = struct{}{}
	}
	if strings.Contains(combined, "pytest") || strings.Contains(combined, "python test") {
		directive.Focus["python"] = struct{}{}
	}
	if strings.Contains(combined, "phpunit") || strings.Contains(combined, "composer test") {
		directive.Focus["php"] = struct{}{}
	}
	if strings.Contains(combined, "make test") {
		directive.Focus["make"] = struct{}{}
	}

	return directive
}
