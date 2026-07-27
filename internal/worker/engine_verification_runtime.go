package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/specialist"
	"github.com/gryph/omnidex/internal/workspace"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func (s *Service) runVerificationTests(
	ctx context.Context,
	claim *model.ClaimedStep,
	contexts map[string]string,
	directive testDirective,
	mode string,
	timeoutSeconds int,
) testReport {
	report := testReport{
		Notes: append([]string{}, directive.Notes...),
	}

	if mode == "off" {
		report.NotRunReason = "tests disabled by verification mode"
		return report
	}
	if directive.Skip {
		report.NotRunReason = "tests skipped per instruction"
		return report
	}

	shouldRun := mode == "force"
	if mode == "auto" && shouldVerifyWithTests(claim.Job.Instruction, contexts) {
		shouldRun = true
	}
	if !shouldRun {
		report.NotRunReason = "task does not appear to require executable test validation"
		return report
	}

	root := verificationWorkspaceRoot(claim.Job.Metadata, s.workspace)
	report.Root = root
	if root == "" {
		report.NotRunReason = "workspace root unavailable"
		return report
	}
	rootInfo, err := os.Stat(root)
	if err != nil || !rootInfo.IsDir() {
		report.NotRunReason = "workspace root not accessible: " + root
		return report
	}

	commands := selectVerificationTestCommands(root, directive)
	if len(commands) == 0 {
		report.NotRunReason = "no applicable test commands found"
		return report
	}

	for _, command := range commands {
		fullCommand := strings.Join(append([]string{command.Name}, command.Args...), " ")
		res := testResult{
			Command: fullCommand,
			Family:  command.Family,
		}

		if _, err := exec.LookPath(command.Name); err != nil {
			res.Skipped = true
			res.Reason = command.Name + " not found"
			report.Skipped++
			report.Commands = append(report.Commands, res)
			s.emitStepStream(claim.Step.ID, "stderr", "test skipped: "+res.Command+" ("+res.Reason+")")
			continue
		}

		s.emitStepEvent(claim.Step.ID, "verify_test_start", fmt.Sprintf("cmd=%s", fullCommand))
		s.emitStepStream(claim.Step.ID, "stdout", "running test: "+fullCommand)

		runCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSeconds)*time.Second)
		cmd := exec.CommandContext(runCtx, command.Name, command.Args...)
		cmd.Dir = root
		var output bytes.Buffer
		cmd.Stdout = &output
		cmd.Stderr = &output

		started := time.Now()
		err := cmd.Run()
		timedOut := errors.Is(runCtx.Err(), context.DeadlineExceeded)
		cancel()

		res.Duration = time.Since(started)
		res.TimedOut = timedOut
		res.Output = truncateCommandOutput(output.String(), verifyMaxCommandOutputChars)
		report.Attempted++

		if err == nil {
			res.Passed = true
			report.Passed++
			report.Commands = append(report.Commands, res)
			s.emitStepEvent(claim.Step.ID, "verify_test_pass", fmt.Sprintf("cmd=%s duration=%s", fullCommand, res.Duration.Truncate(time.Millisecond)))
			continue
		}

		report.Failed++
		res.Passed = false
		if timedOut {
			res.Reason = "timed out"
		} else {
			res.Reason = err.Error()
		}
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			res.ExitCode = exitErr.ExitCode()
		}
		report.Commands = append(report.Commands, res)
		s.emitStepEvent(claim.Step.ID, "verify_test_fail", fmt.Sprintf("cmd=%s reason=%s", fullCommand, trimForBudget(res.Reason, 260)))
		if strings.TrimSpace(res.Output) != "" {
			s.emitStepStream(claim.Step.ID, "stderr", "test output "+fullCommand+":\n"+trimForBudget(res.Output, 1600))
		}
	}

	return report
}

func shouldVerifyWithTests(instruction string, contexts map[string]string) bool {
	lowerInstruction := strings.ToLower(strings.TrimSpace(instruction))
	if codeKeywordPattern.MatchString(lowerInstruction) {
		return true
	}
	if strings.Contains(lowerInstruction, "implement") ||
		strings.Contains(lowerInstruction, "fix") ||
		strings.Contains(lowerInstruction, "bug") ||
		strings.Contains(lowerInstruction, "test") ||
		strings.Contains(lowerInstruction, "refactor") {
		return true
	}

	planLower := strings.ToLower(strings.TrimSpace(contexts["plan"]))
	if strings.Contains(planLower, "file") ||
		strings.Contains(planLower, "code") ||
		strings.Contains(planLower, "test") ||
		strings.Contains(planLower, "build") {
		return true
	}

	toolingLower := strings.ToLower(strings.TrimSpace(contexts["tooling"]))
	return strings.Contains(toolingLower, "required_tools=")
}

func verificationWorkspaceRoot(metadata json.RawMessage, ws *workspace.Service) string {
	root := strings.TrimSpace(metadataString(metadata, "workspace_root"))
	if root != "" {
		return root
	}
	if ws == nil {
		return ""
	}
	return strings.TrimSpace(ws.Root())
}

func selectVerificationTestCommands(root string, directive testDirective) []testCommand {
	exists := func(rel string) bool {
		if strings.TrimSpace(rel) == "" {
			return false
		}
		info, err := os.Stat(filepath.Join(root, rel))
		return err == nil && !info.IsDir()
	}

	commands := []testCommand{}
	if exists("go.mod") {
		commands = append(commands, testCommand{Family: "go", Name: "go", Args: []string{"test", "./..."}})
	}

	if exists("package.json") {
		manager := "npm"
		if exists("pnpm-lock.yaml") {
			manager = "pnpm"
		} else if exists("yarn.lock") {
			manager = "yarn"
		}
		switch manager {
		case "pnpm":
			commands = append(commands, testCommand{Family: "node", Name: "pnpm", Args: []string{"test"}})
		case "yarn":
			commands = append(commands, testCommand{Family: "node", Name: "yarn", Args: []string{"test"}})
		default:
			commands = append(commands, testCommand{Family: "node", Name: "npm", Args: []string{"test"}})
		}
	}

	if exists("pyproject.toml") || exists("requirements.txt") {
		commands = append(commands, testCommand{Family: "python", Name: "pytest", Args: []string{"-q"}})
	}

	if exists("composer.json") {
		commands = append(commands, testCommand{Family: "php", Name: "composer", Args: []string{"test"}})
	}

	if exists("Makefile") || exists("makefile") {
		commands = append(commands, testCommand{Family: "make", Name: "make", Args: []string{"test"}})
	}

	if len(directive.Focus) > 0 {
		filtered := make([]testCommand, 0, len(commands))
		for _, command := range commands {
			if _, ok := directive.Focus[command.Family]; ok {
				filtered = append(filtered, command)
			}
		}
		commands = filtered
	}

	seen := map[string]struct{}{}
	out := make([]testCommand, 0, len(commands))
	for _, command := range commands {
		key := command.Family + "|" + command.Name + "|" + strings.Join(command.Args, " ")
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, command)
	}
	return out
}

func verificationPassCount(_ model.Job) int {
	return 2
}

func verificationHallucinationRetryLimit(job model.Job, fallback int) int {
	if fallback < 1 {
		fallback = defaultHallucinationRetryLimit
	}
	if fallback > maxHallucinationRetryLimit {
		fallback = maxHallucinationRetryLimit
	}
	for _, key := range []string{"hallucination_retry_limit", "hallucination_retries", "hallucination_loop_limit"} {
		value := metadataInt(job.Metadata, key, 0)
		if value <= 0 {
			continue
		}
		if value > maxHallucinationRetryLimit {
			return maxHallucinationRetryLimit
		}
		return value
	}
	return fallback
}

func hallucinationRetrySignal(consensusNote string, reviewSignals []string, outcome verificationOutcome) (bool, string) {
	if strings.ToLower(strings.TrimSpace(outcome.Status)) != "retry" {
		return false, ""
	}

	note := strings.ToLower(strings.TrimSpace(consensusNote))
	if strings.Contains(note, "no_consensus") || strings.Contains(note, "hallucination") {
		return true, "verification consensus disagreement"
	}

	for _, signal := range reviewSignals {
		if !looksLikeHallucinationSignal(signal) {
			continue
		}
		return true, safeLine(signal, "grounding signal")
	}

	joined := strings.ToLower(strings.TrimSpace(strings.Join([]string{
		outcome.Summary,
		strings.Join(outcome.Gaps, " "),
		outcome.CannotCompleteReason,
	}, " ")))
	for _, indicator := range []string{
		"hallucination",
		"unsupported",
		"without evidence",
		"without web_search context",
		"without web_search evidence",
		"without execution evidence",
		"weakly related",
		"did not agree",
	} {
		if strings.Contains(joined, indicator) {
			return true, indicator
		}
	}
	return false, ""
}

func looksLikeHallucinationSignal(signal string) bool {
	lower := strings.ToLower(strings.TrimSpace(signal))
	if lower == "" {
		return false
	}
	for _, indicator := range []string{
		"unsupported",
		"without web_search context",
		"without web_search evidence",
		"without execution evidence",
		"claims test execution/results without executed tests",
		"claims command/action execution without execution evidence",
		"weakly related",
		"hallucination",
	} {
		if strings.Contains(lower, indicator) {
			return true
		}
	}
	return false
}

func hallucinationLoopUserMessage(restartErr error) string {
	if restartErr == nil {
		return "I detected a hallucination loop during verification, restarted the Ollama service, and stopped this run. Please try again."
	}
	return "I detected a hallucination loop during verification and stopped this run. I could not restart the Ollama service automatically, so please restart it and try again."
}

func parseCommandAttemptSpec(raw string) [][]string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}

	segments := strings.Split(raw, "||")
	attempts := make([][]string, 0, len(segments))
	for _, segment := range segments {
		parts := strings.Fields(strings.TrimSpace(segment))
		if len(parts) == 0 {
			continue
		}
		attempts = append(attempts, parts)
	}
	return attempts
}

func defaultOllamaRestartCommandAttempts() [][]string {
	return [][]string{
		{"docker", "compose", "restart", "ollama"},
		{"docker", "restart", "ollama"},
		{"systemctl", "restart", "ollama"},
		{"service", "ollama", "restart"},
		{"brew", "services", "restart", "ollama"},
	}
}

func ollamaRestartCommandAttempts(job model.Job, configured string) [][]string {
	metadataCommand := strings.TrimSpace(metadataString(job.Metadata, "ollama_restart_command"))
	if metadataCommand == "" {
		metadataCommand = strings.TrimSpace(metadataString(job.Metadata, "ollama_restart_commands"))
	}
	if attempts := parseCommandAttemptSpec(metadataCommand); len(attempts) > 0 {
		return attempts
	}
	if attempts := parseCommandAttemptSpec(configured); len(attempts) > 0 {
		return attempts
	}
	return defaultOllamaRestartCommandAttempts()
}

func commandLineLabel(parts []string) string {
	if len(parts) == 0 {
		return "(empty)"
	}
	return strings.Join(parts, " ")
}

func (s *Service) restartOllamaForHallucinationLoop(ctx context.Context, claim *model.ClaimedStep) (string, error) {
	attempts := ollamaRestartCommandAttempts(claim.Job, s.ollamaRestartCommand)
	if len(attempts) == 0 {
		return "restart command unavailable", fmt.Errorf("no ollama restart command configured")
	}

	timeout := s.ollamaRestartTimeout
	if timeout <= 0 {
		timeout = defaultOllamaRestartTimeout
	}

	failureNotes := make([]string, 0, len(attempts))
	for _, attempt := range attempts {
		if len(attempt) == 0 {
			continue
		}
		commandName := strings.TrimSpace(attempt[0])
		if commandName == "" {
			continue
		}
		label := commandLineLabel(attempt)
		if _, err := exec.LookPath(commandName); err != nil {
			failureNotes = append(failureNotes, label+" -> unavailable")
			continue
		}

		runCtx, cancel := context.WithTimeout(ctx, timeout)
		cmd := exec.CommandContext(runCtx, commandName, attempt[1:]...)
		var output bytes.Buffer
		cmd.Stdout = &output
		cmd.Stderr = &output
		err := cmd.Run()
		timedOut := errors.Is(runCtx.Err(), context.DeadlineExceeded)
		cancel()

		commandOutput := trimForBudget(strings.TrimSpace(output.String()), maxOllamaRestartOutputChars)
		if err == nil {
			note := "restart command succeeded: " + label
			if commandOutput != "" {
				note += " output=" + safeLine(commandOutput, "(none)")
			}
			return note, nil
		}

		reason := safeLine(err.Error(), "failed")
		if timedOut {
			reason = "timed out"
		}
		if commandOutput != "" {
			reason = reason + " output=" + safeLine(commandOutput, "(none)")
		}
		failureNotes = append(failureNotes, trimForBudget(label+" -> "+reason, 320))
	}

	if len(failureNotes) == 0 {
		return "restart command attempts unavailable", fmt.Errorf("no executable ollama restart commands available")
	}
	return strings.Join(failureNotes, " | "), fmt.Errorf("all ollama restart attempts failed")
}

func (s *Service) evaluateVerificationConsensus(
	ctx context.Context,
	claim *model.ClaimedStep,
	contexts map[string]string,
	response string,
	report testReport,
	attempt int,
	maxAttempts int,
	passes int,
) (verificationOutcome, string, error) {
	if passes < 1 {
		passes = 1
	}

	outcomes := make([]verificationOutcome, 0, passes)
	for pass := 1; pass <= passes; pass++ {
		outcome, err := s.evaluateVerification(ctx, claim, contexts, response, report, attempt, maxAttempts, pass, passes)
		if err != nil {
			s.emitStepStream(claim.Step.ID, "stderr", fmt.Sprintf("verification evaluator failed pass=%d: %s", pass, trimForBudget(err.Error(), 300)))
			return verificationOutcome{}, "", fmt.Errorf("verification evaluator pass %d/%d failed: %w", pass, passes, err)
		}
		normalized := normalizeVerificationOutcome(outcome, report)
		outcomes = append(outcomes, normalized)
		s.emitStepEvent(
			claim.Step.ID,
			"verify_pass_ready",
			fmt.Sprintf("attempt=%d/%d pass=%d/%d status=%s confidence=%.2f", attempt, maxAttempts, pass, passes, normalized.Status, normalized.Confidence),
		)
	}

	consensus, hadMajority, note := aggregateVerificationConsensus(outcomes, report)
	if !hadMajority {
		s.emitStepEvent(claim.Step.ID, "verify_consensus_hallucination", fmt.Sprintf("attempt=%d/%d %s", attempt, maxAttempts, note))
	}
	return consensus, note, nil
}

func aggregateVerificationConsensus(outcomes []verificationOutcome, report testReport) (verificationOutcome, bool, string) {
	if len(outcomes) == 0 {
		outcome := verificationOutcome{
			Status:               "retry",
			Confidence:           0,
			Summary:              "verification consensus unavailable; retry required",
			Gaps:                 []string{"no verification pass outputs available"},
			CannotCompleteReason: "verification produced no evaluator outputs",
		}
		return normalizeVerificationOutcome(outcome, report), false, "no_consensus passes=0"
	}
	if len(outcomes) == 2 {
		left := normalizeVerificationOutcome(outcomes[0], report)
		right := normalizeVerificationOutcome(outcomes[1], report)

		leftPass := strings.EqualFold(strings.TrimSpace(left.Status), "pass")
		rightPass := strings.EqualFold(strings.TrimSpace(right.Status), "pass")
		if leftPass && rightPass {
			merged := left
			if right.Confidence > merged.Confidence {
				merged = right
			}
			merged.Confidence = (left.Confidence + right.Confidence) / 2
			merged.Gaps = dedupeStrings(append(append([]string{}, left.Gaps...), right.Gaps...))
			if strings.TrimSpace(merged.Summary) == "" {
				merged.Summary = "both verification judges confirmed completion"
			}
			return normalizeVerificationOutcome(merged, report), true, "dual_confirmation=yes judges=2/2"
		}

		combinedGaps := dedupeStrings(append(append([]string{}, left.Gaps...), right.Gaps...))
		if len(combinedGaps) == 0 {
			combinedGaps = []string{"at least one verification judge reported objective not yet achieved"}
		}
		noConsensus := verificationOutcome{
			Status:               "retry",
			Confidence:           0.20,
			Summary:              "dual verification judges did not both confirm completion",
			Gaps:                 combinedGaps,
			CannotCompleteReason: "",
		}
		return normalizeVerificationOutcome(noConsensus, report), false, fmt.Sprintf("dual_confirmation=no judge1=%s judge2=%s", left.Status, right.Status)
	}

	counts := map[string]int{}
	for _, outcome := range outcomes {
		status := strings.ToLower(strings.TrimSpace(outcome.Status))
		if !verifyStatusPattern.MatchString(status) {
			status = "pass"
		}
		counts[status]++
	}

	order := []string{"pass", "retry", "blocked"}
	majorityStatus := ""
	majorityCount := 0
	required := len(outcomes)/2 + 1
	for _, status := range order {
		count := counts[status]
		if count > majorityCount {
			majorityCount = count
			majorityStatus = status
		}
	}

	statusSummary := summarizeVerificationStatusCounts(counts, len(outcomes))
	if majorityCount < required {
		gaps := []string{"verification passes did not agree (hallucination risk); retry required"}
		for _, outcome := range outcomes {
			gaps = append(gaps, outcome.Gaps...)
		}
		noConsensus := verificationOutcome{
			Status:               "retry",
			Confidence:           0.20,
			Summary:              "verification passes disagreed with no majority",
			Gaps:                 dedupeStrings(gaps),
			CannotCompleteReason: "",
		}
		return normalizeVerificationOutcome(noConsensus, report), false, "no_consensus " + statusSummary
	}

	majorityOutcomes := make([]verificationOutcome, 0, majorityCount)
	for _, outcome := range outcomes {
		status := strings.ToLower(strings.TrimSpace(outcome.Status))
		if !verifyStatusPattern.MatchString(status) {
			status = "pass"
		}
		if status == majorityStatus {
			majorityOutcomes = append(majorityOutcomes, outcome)
		}
	}
	if len(majorityOutcomes) == 0 {
		outcome := verificationOutcome{
			Status:               "retry",
			Confidence:           0,
			Summary:              "verification majority resolution failed; retry required",
			Gaps:                 []string{"internal majority-selection mismatch"},
			CannotCompleteReason: "verification majority could not be resolved",
		}
		return normalizeVerificationOutcome(outcome, report), false, "no_consensus internal_mismatch"
	}

	representative := majorityOutcomes[0]
	for _, candidate := range majorityOutcomes[1:] {
		if candidate.Confidence > representative.Confidence {
			representative = candidate
		}
	}

	gaps := append([]string{}, representative.Gaps...)
	confidenceSum := 0.0
	for _, outcome := range majorityOutcomes {
		confidenceSum += outcome.Confidence
		gaps = append(gaps, outcome.Gaps...)
	}
	representative.Gaps = dedupeStrings(gaps)
	representative.Confidence = confidenceSum / float64(len(majorityOutcomes))
	if strings.TrimSpace(representative.Summary) == "" {
		representative.Summary = fmt.Sprintf("verification majority=%s", majorityStatus)
	}

	return normalizeVerificationOutcome(representative, report), true, fmt.Sprintf("majority=%s count=%d/%d %s", majorityStatus, majorityCount, len(outcomes), statusSummary)
}

func summarizeVerificationStatusCounts(counts map[string]int, total int) string {
	parts := make([]string, 0, 3)
	for _, status := range []string{"pass", "retry", "blocked"} {
		parts = append(parts, fmt.Sprintf("%s=%d", status, counts[status]))
	}
	return fmt.Sprintf("statuses[%s] total=%d", strings.Join(parts, ","), total)
}

func (s *Service) evaluateVerification(
	ctx context.Context,
	claim *model.ClaimedStep,
	contexts map[string]string,
	response string,
	report testReport,
	attempt int,
	maxAttempts int,
	pass int,
	totalPasses int,
) (verificationOutcome, error) {
	prompt := strings.Join([]string{
		"You are a strict verifier.",
		antiRoleplayInstructionForPipeline(claim.Job.Pipeline),
		promptTrustBoundaryInstruction(),
		promptUserAnchor("start", claim.Job.Instruction, contexts["user_feedback"]),
		`Return JSON only: {"status":"pass|retry|blocked","confidence":0.0,"summary":"...","gaps":["..."],"cannot_complete_reason":"..."}`,
		"confidence must be a numeric value in [0.0, 1.0] where 0.0=very uncertain and 1.0=very certain.",
		"Use status=pass only when the response satisfies the instruction and test evidence does not show failures.",
		"Use status=retry when response quality can be improved in another pass.",
		"Use status=blocked when the task cannot be fully completed with available context/test evidence.",
		"Hallucination and relevance rules (strict):",
		"- If the response claims actions happened in this run without clear evidence, set status=retry.",
		"- If the response is weakly related or off-topic vs USER_INSTRUCTION, set status=retry.",
		"- Prefer concise, concrete fixes to unsupported claims.",
		fmt.Sprintf("Attempt: %d/%d", attempt, maxAttempts),
		fmt.Sprintf("Verification Pass: %d/%d", pass, totalPasses),
		"Instruction:",
		trimForBudget(claim.Job.Instruction, 1400),
		"User Feedback:",
		trimForBudget(contexts["user_feedback"], 1000),
		"Plan:",
		trimForBudget(contexts["plan"], 1200),
		"Analyzer:",
		trimForBudget(contexts["analyzer"], 1600),
		"Recent Conversation:",
		trimForBudget(contexts["recent_conversation"], 1400),
		"Tooling:",
		trimForBudget(contexts["tooling"], 1400),
		"Workspace:",
		trimForBudget(contexts["workspace"], 1400),
		"Retrieved Memory:",
		trimForBudget(contexts["retrieval"], 1400),
		"Web Search:",
		trimForBudget(contexts["web_search"], 1400),
		"Action Execution Audit:",
		trimForBudget(contexts["verification_action_audit"], 1400),
		"Current Response:",
		trimForBudget(response, 2200),
		"Test Instructions / Notes:",
		trimForBudget(strings.Join(report.Notes, "\n"), 1000),
		"Test Evidence:",
		trimForBudget(formatTestReportForPrompt(report), 2200),
		promptUserAnchor("end", claim.Job.Instruction, contexts["user_feedback"]),
		"Final grounding check: judge against AUTHORITATIVE_USER_INSTRUCTION_END.",
	}, "\n\n")

	verifyFallback := s.specialistModel(claim.Job, specialist.RoleReviewVerificationSpecialist, s.models.Analyze)
	verifyModel := metadataModel(claim.Job, "model_verify", verifyFallback)
	raw, err := s.llmGenerateWithTrace(
		ctx,
		claim.Step.ID,
		fmt.Sprintf("verify_evaluate_attempt_%d_of_%d_pass_%d_of_%d", attempt, maxAttempts, pass, totalPasses),
		verifyModel,
		prompt,
	)
	if err != nil {
		return verificationOutcome{}, err
	}

	payload := strings.TrimSpace(raw)
	if !strings.HasPrefix(payload, "{") {
		start := strings.Index(payload, "{")
		end := strings.LastIndex(payload, "}")
		if start >= 0 && end > start {
			payload = payload[start : end+1]
		}
	}

	return decodeVerificationOutcome(payload)
}

func decodeVerificationOutcome(payload string) (verificationOutcome, error) {
	payload = strings.TrimSpace(payload)
	if payload == "" {
		return verificationOutcome{}, fmt.Errorf("empty verifier payload")
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(payload), &raw); err != nil {
		if loose, ok := parseLooseVerificationOutcome(payload); ok {
			return loose, nil
		}
		return verificationOutcome{}, err
	}

	var outcome verificationOutcome
	outcome.Status = parseVerificationStringField(raw["status"])
	outcome.Summary = parseVerificationStringField(raw["summary"])
	outcome.CannotCompleteReason = parseVerificationStringField(raw["cannot_complete_reason"])

	if confidence, ok, err := parseVerificationConfidenceField(raw["confidence"]); err == nil && ok {
		outcome.Confidence = confidence
	}

	gaps, err := parseVerificationGapsField(raw["gaps"])
	if err != nil {
		return verificationOutcome{}, err
	}
	outcome.Gaps = gaps

	return outcome, nil
}
