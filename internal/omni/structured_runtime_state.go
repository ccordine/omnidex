package omni

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
)

func completedActionsFromState(ledger []StructuredObjective, observations []StructuredCommandObservation) []CompletedAction {
	actions := []CompletedAction{}
	seen := map[string]struct{}{}
	for _, obs := range observations {
		command := strings.TrimSpace(obs.Command)
		if obs.ExitCode != 0 || command == "" || strings.HasPrefix(command, "SKIPPED_REPEAT_SUCCESS:") {
			continue
		}
		normalized := normalizeStructuredCommandForComparison(command)
		if normalized == "" {
			continue
		}
		key := "command:" + normalized
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		actions = append(actions, CompletedAction{
			ID:       completedActionID("command", normalized),
			Kind:     completedActionKindForCommand(command),
			Summary:  completedActionSummaryForCommand(command),
			Command:  command,
			Evidence: structuredObjectiveEvidenceFromObservation(obs),
			Step:     obs.Step,
		})
	}
	for _, objective := range mergeStructuredObjectiveLedger(nil, ledger) {
		if !structuredObjectiveSatisfied(objective) {
			continue
		}
		id := strings.TrimSpace(objective.ID)
		if id == "" {
			continue
		}
		key := "objective:" + id
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		summary := strings.TrimSpace(objective.Description)
		if summary == "" {
			summary = id
		}
		actions = append(actions, CompletedAction{
			ID:          completedActionID("objective", id),
			Kind:        "objective",
			Summary:     "Satisfied objective: " + summary,
			ObjectiveID: id,
			Evidence:    truncateStructuredObservation(objective.Evidence),
		})
	}
	sort.SliceStable(actions, func(i, j int) bool {
		if actions[i].Step == actions[j].Step {
			return actions[i].ID < actions[j].ID
		}
		if actions[i].Step == 0 {
			return false
		}
		if actions[j].Step == 0 {
			return true
		}
		return actions[i].Step < actions[j].Step
	})
	return actions
}

func completedActionID(kind, value string) string {
	clean := strings.ToLower(strings.TrimSpace(kind + " " + value))
	var b strings.Builder
	lastUnderscore := false
	for _, r := range clean {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastUnderscore = false
			continue
		}
		if !lastUnderscore {
			b.WriteByte('_')
			lastUnderscore = true
		}
	}
	id := strings.Trim(b.String(), "_")
	if id == "" {
		return "completed_action"
	}
	if len(id) > 96 {
		id = strings.TrimRight(id[:96], "_")
	}
	return id
}

func completedActionKindForCommand(command string) string {
	fields := strings.Fields(strings.TrimSpace(command))
	if len(fields) == 0 {
		return "command"
	}
	root := fields[0]
	switch root {
	case "mkdir":
		return "file"
	case "npm", "pnpm", "yarn", "bun", "go", "cargo", "composer", "pip":
		return "dependency_or_verification"
	case "test", "cat", "sed", "rg", "find", "ls", "git":
		return "inspection"
	default:
		return "command"
	}
}

func completedActionSummaryForCommand(command string) string {
	return "Completed command: " + truncateStructuredObservation(normalizeStructuredCommandForComparison(command))
}

func structuredLoopStateFromState(ledger []StructuredObjective, observations []StructuredCommandObservation) StructuredLoopState {
	pendingIDs := structuredObjectiveIDs(pendingStructuredObjectives(ledger))
	state := StructuredLoopState{
		Status:              "progressing",
		PendingObjectiveIDs: pendingIDs,
	}
	if len(observations) == 0 {
		state.Status = "not_started"
		state.Instruction = "Start with a command or patch that gathers evidence or satisfies the first objective."
		return state
	}
	if latestRealObservationSucceeded(observations) {
		state.Instruction = "Latest command completed successfully; choose the next evidence-producing command, patch, verification, or completion check from the current objective ledger."
		return state
	}
	if blocker := latestStructuredObservationBlocker(observations); blocker != "" {
		state.LastBlocker = blocker
	}
	if count, pending := latestPrematureDoneRejectionRun(observations); count > 0 {
		state.RepeatKind = "premature_done"
		state.RepeatCount = count
		if len(pendingIDs) == 0 && strings.TrimSpace(pending) != "" {
			state.PendingObjectiveIDs = strings.Split(pending, ",")
		}
		if count >= maxRepeatedPrematureDoneRejections {
			state.Status = "blocked"
			state.Instruction = "Stop returning done=true; choose a command or patch that satisfies a pending objective."
		} else {
			state.Status = "stuck"
			state.Instruction = "The previous done=true was rejected; advance a pending objective before trying done again."
		}
		return state
	}
	if count, command := latestRejectedCommandRun(observations); count > 0 {
		state.RepeatKind = "rejected_command"
		state.RepeatCount = count
		state.RepeatedCommand = command
		if count >= 2 {
			state.Status = "blocked"
		} else {
			state.Status = "stuck"
		}
		state.Instruction = "The latest proposal was rejected before execution: " + truncateStructuredTimelineValue(command) + ". Rejected proposals are evidence only, not completed actions and not forbidden commands. Choose a valid command, use tool=shell with a narrower task, inspect existing files, or use tool=patch.apply for source edits."
		return state
	}
	return state
}

func latestStructuredObservationBlocker(observations []StructuredCommandObservation) string {
	for i := len(observations) - 1; i >= 0; i-- {
		obs := observations[i]
		if obs.ExitCode == 0 {
			continue
		}
		if strings.TrimSpace(obs.Stderr) != "" {
			return truncateStructuredTimelineValue(obs.Stderr)
		}
		if strings.TrimSpace(obs.EvaluationFeedback) != "" {
			return truncateStructuredTimelineValue(obs.EvaluationFeedback)
		}
		if strings.TrimSpace(obs.RejectedCommand) != "" {
			return "rejected command: " + truncateStructuredTimelineValue(obs.RejectedCommand)
		}
	}
	return ""
}

func latestPrematureDoneRejectionRun(observations []StructuredCommandObservation) (int, string) {
	count := 0
	pending := ""
	for i := len(observations) - 1; i >= 0; i-- {
		stderr := strings.TrimSpace(observations[i].Stderr)
		if !strings.Contains(stderr, "done rejected: pending objective(s) remain:") &&
			!strings.Contains(stderr, "anti_loop: planner returned done=true") {
			if count > 0 {
				break
			}
			continue
		}
		current := extractPendingObjectivesFromDoneRejection(stderr)
		if pending == "" {
			pending = current
		}
		if current != "" && pending != "" && current != pending {
			break
		}
		count++
	}
	return count, pending
}

func extractPendingObjectivesFromDoneRejection(stderr string) string {
	for _, marker := range []string{"pending objective(s) remain:", "same pending objective(s) remain:"} {
		idx := strings.Index(stderr, marker)
		if idx < 0 {
			continue
		}
		rest := strings.TrimSpace(stderr[idx+len(marker):])
		if semi := strings.Index(rest, ";"); semi >= 0 {
			rest = rest[:semi]
		}
		if dot := strings.Index(rest, "."); dot >= 0 {
			rest = rest[:dot]
		}
		return strings.TrimSpace(rest)
	}
	return ""
}

func latestRejectedCommandRun(observations []StructuredCommandObservation) (int, string) {
	count := 0
	command := ""
	for i := len(observations) - 1; i >= 0; i-- {
		current := normalizeStructuredCommandForComparison(observations[i].RejectedCommand)
		if current == "" {
			if count > 0 {
				break
			}
			continue
		}
		if command == "" {
			command = current
		}
		if current != command {
			break
		}
		count++
	}
	return count, command
}

func startsWithShellRedirectionToken(command string) bool {
	fields := strings.Fields(strings.TrimSpace(command))
	if len(fields) == 0 {
		return false
	}
	return isShellRedirectToken(fields[0])
}

func isPureEchoCommand(command string) bool {
	trimmed := strings.TrimSpace(command)
	if commandSegmentsContainShellControl(trimmed, "|", ">", "<", "$(", "`") {
		return false
	}
	segments := structuredCommandSegments(trimmed)
	if len(segments) == 0 {
		return false
	}
	for _, segment := range segments {
		if len(segment) == 0 {
			return false
		}
		root := cleanCommandPathToken(segment[0])
		if root != "echo" {
			return false
		}
	}
	return true
}

func isPurePrintCommand(command string) bool {
	trimmed := strings.TrimSpace(command)
	if commandSegmentsContainShellControl(trimmed, "|", ">", "<", "$(", "`") {
		return false
	}
	segments := structuredCommandSegments(trimmed)
	if len(segments) == 0 {
		return false
	}
	for _, segment := range segments {
		if len(segment) == 0 {
			return false
		}
		switch cleanCommandPathToken(segment[0]) {
		case "echo", "printf":
		default:
			return false
		}
	}
	return true
}

func commandPrintsFalseCapabilityLimitation(command string) bool {
	lower := strings.ToLower(command)
	for _, phrase := range []string{
		"do not have access",
		"don't have access",
		"cannot access real-time",
		"can't access real-time",
		"cannot check",
		"can't check",
		"check a weather website",
		"check the current time with",
	} {
		if strings.Contains(lower, phrase) {
			return true
		}
	}
	return false
}

func commandSegmentsContainShellControl(command string, markers ...string) bool {
	for _, marker := range markers {
		if strings.Contains(command, marker) {
			return true
		}
	}
	return false
}

func isNonEvidenceShellCommand(command string) bool {
	trimmed := strings.TrimSpace(command)
	lower := strings.ToLower(trimmed)
	switch lower {
	case "bash", "sh", "zsh", "fish", "dash", "true", ":", "exit", "exit 0":
		return true
	default:
		return false
	}
}

func validateWTTRCommand(command string) error {
	lower := strings.ToLower(command)
	if !strings.Contains(lower, "wttr.in") {
		return nil
	}
	if !strings.Contains(lower, "wttr.in/") {
		return fmt.Errorf("wttr.in command must include an explicit location path")
	}
	if strings.Contains(lower, "wttr.in/?") || strings.Contains(lower, "wttr.in/ ") || strings.HasSuffix(strings.TrimSpace(lower), "wttr.in/") {
		return fmt.Errorf("wttr.in command must include a non-empty location path")
	}
	if !strings.Contains(lower, "format=") {
		return fmt.Errorf("wttr.in command must use a concise format query")
	}
	return nil
}

func validateDateCommand(command string) error {
	trimmed := strings.TrimSpace(command)
	lower := strings.ToLower(trimmed)
	if !strings.Contains(lower, "date") {
		return nil
	}
	fields := strings.Fields(lower)
	for i, field := range fields {
		if field == "date" || strings.HasSuffix(field, "/date") {
			if i+1 < len(fields) && fields[i+1] == "-t" {
				return fmt.Errorf("date command must not use invalid -t timezone option; prefix with TZ=Area/City before date")
			}
		}
	}
	if strings.Contains(lower, "date ") && strings.Contains(lower, "tz=") && !strings.HasPrefix(lower, "tz=") && !strings.Contains(lower, " tz=") {
		return fmt.Errorf("date command must prefix TZ=Area/City before date, not pass TZ as a date argument")
	}
	if strings.Contains(lower, "date ") && strings.Contains(lower, "-d") && strings.Contains(lower, "tz=") && !strings.HasPrefix(lower, "tz=") {
		return fmt.Errorf("date command must prefix TZ=Area/City before date, not pass TZ through -d")
	}
	return nil
}

func validateGoogleNewsRSSCommand(command string) error {
	lower := strings.ToLower(command)
	if !strings.Contains(lower, "news.google.com/rss/search") {
		return nil
	}
	if !curlCommandFollowsRedirects(command) {
		return fmt.Errorf("Google News RSS command must use curl -L or curl -fsSL so redirects produce evidence")
	}
	if !curlCommandHasSilentFlag(command) {
		return fmt.Errorf("Google News RSS command must use curl -s or curl -fsSL to avoid progress-meter noise in evidence")
	}
	if !curlCommandHasUserAgent(command) {
		return fmt.Errorf("Google News RSS command must set a user agent with curl -A 'Mozilla/5.0'")
	}
	if !strings.Contains(lower, "ceid=") {
		return fmt.Errorf("Google News RSS command must include hl/gl/ceid query parameters for stable localized results")
	}
	return nil
}

func curlCommandFollowsRedirects(command string) bool {
	lower := strings.ToLower(command)
	if strings.Contains(lower, "--location") {
		return true
	}
	for _, field := range strings.Fields(lower) {
		if strings.HasPrefix(field, "-") && strings.Contains(field, "l") {
			return true
		}
	}
	return false
}

func curlCommandHasSilentFlag(command string) bool {
	lower := strings.ToLower(command)
	for _, field := range strings.Fields(lower) {
		if strings.HasPrefix(field, "-") && strings.Contains(field, "s") {
			return true
		}
	}
	return false
}

func curlCommandHasUserAgent(command string) bool {
	lower := strings.ToLower(command)
	return strings.Contains(lower, " -a ") || strings.Contains(lower, "\t-a ") || strings.Contains(lower, "--user-agent")
}

func emitStructuredCommandEvent(onEvent func(StructuredCommandEvent), eventType, summary string, details map[string]string) {
	if onEvent == nil {
		return
	}
	onEvent(StructuredCommandEvent{Type: eventType, Summary: summary, Details: details})
}

func rejectMixedAskCommandPayload(step int, payload StructuredCommandPayload, onEvent func(StructuredCommandEvent), result *CommandDecisionResult) bool {
	if !payload.Ask {
		return false
	}
	command := strings.TrimSpace(payload.Command)
	tool := strings.TrimSpace(payload.Tool)
	if command == "" && tool == "" && !payload.Done {
		return false
	}
	emitStructuredCommandEvent(onEvent, "structured_payload_rejected_mixed_ask_command", "Structured payload rejected for mixed ask and executable intent", map[string]string{
		"step":    fmt.Sprintf("%d", step),
		"command": truncateStructuredTimelineValue(command),
		"tool":    truncateStructuredTimelineValue(tool),
		"done":    fmt.Sprintf("%t", payload.Done),
	})
	if result != nil {
		result.Observations = append(result.Observations, StructuredCommandObservation{
			Step:            step,
			RejectedCommand: truncateStructuredObservation(command),
			ExitCode:        1,
			Stderr:          "payload rejected: ask=true cannot be combined with command, tool, or done=true; ask mode may only carry a clarification question",
		})
	}
	return true
}

func structuredQuestionAsksUserToRunCommand(question string) bool {
	lower := strings.ToLower(strings.TrimSpace(question))
	if lower == "" {
		return false
	}
	manualRun := strings.Contains(lower, "please run") ||
		strings.Contains(lower, "run this command") ||
		strings.Contains(lower, "run the command") ||
		strings.Contains(lower, "you run") ||
		strings.Contains(lower, "execute this command") ||
		strings.Contains(lower, "execute the command")
	if !manualRun {
		return false
	}
	return strings.Contains(lower, "npm ") ||
		strings.Contains(lower, "npx ") ||
		strings.Contains(lower, "go ") ||
		strings.Contains(lower, "cargo ") ||
		strings.Contains(lower, "python") ||
		strings.Contains(lower, "bash") ||
		strings.Contains(lower, "sh ") ||
		strings.Contains(lower, "vite")
}

func isStructuredUserInputCancelled(ctx context.Context, err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled)
}

func markStructuredUserInputCancelled(step int, question string, onEvent func(StructuredCommandEvent), result *CommandDecisionResult) {
	emitStructuredCommandEvent(onEvent, "structured_user_input_cancelled", "Structured user input request was cancelled", map[string]string{
		"step":     fmt.Sprintf("%d", step),
		"question": truncateStructuredTimelineValue(question),
	})
	if result == nil {
		return
	}
	result.ExitCode = 1
	result.Observations = append(result.Observations, StructuredCommandObservation{
		Step:     step,
		ExitCode: 1,
		Question: question,
		Stderr:   "user input cancelled: no pending command was approved or dispatched",
	})
}

func hasRealCommandObservation(observations []StructuredCommandObservation) bool {
	for _, obs := range observations {
		if strings.TrimSpace(obs.Command) != "" {
			return true
		}
	}
	return false
}

func hasSuccessfulCommandObservation(observations []StructuredCommandObservation) bool {
	for _, obs := range observations {
		if strings.TrimSpace(obs.Command) != "" && obs.ExitCode == 0 {
			return true
		}
	}
	return false
}

func latestSuccessfulCommandObservation(observations []StructuredCommandObservation) (StructuredCommandObservation, bool) {
	for i := len(observations) - 1; i >= 0; i-- {
		if strings.TrimSpace(observations[i].Command) != "" && observations[i].ExitCode == 0 {
			return observations[i], true
		}
	}
	return StructuredCommandObservation{}, false
}

func enforcePostWriteValidationBeforeCompletion(step int, prompt string, previousLedger, ledger []StructuredObjective, observations []StructuredCommandObservation, onEvent func(StructuredCommandEvent), result *CommandDecisionResult) []StructuredObjective {
	if len(pendingStructuredObjectives(ledger)) > 0 || !structuredCompletionNeedsPostWriteValidation(prompt, previousLedger, observations) {
		return ledger
	}
	pendingBefore := pendingStructuredObjectives(previousLedger)
	if len(pendingBefore) == 0 {
		return ledger
	}
	reset := make([]StructuredObjective, 0, len(pendingBefore))
	for _, objective := range pendingBefore {
		objective.Status = "pending"
		objective.Evidence = ""
		reset = append(reset, objective)
	}
	emitStructuredCommandEvent(onEvent, "completion_check_validation_required", "Completion requires readback evidence after a write command", map[string]string{
		"step":       fmt.Sprintf("%d", step),
		"objectives": strings.Join(structuredObjectiveIDs(reset), ","),
	})
	if result != nil && !latestObservationIsPostWriteValidationRejection(result.Observations) {
		result.Observations = append(result.Observations, StructuredCommandObservation{
			Step:     step,
			ExitCode: 1,
			Stderr:   "completion rejected: write/edit/package mutation was observed, but no later readback or verification command proves the requested final state; run cat/sed/rg/grep/ls/test/jq/npm pkg get/npm ls or equivalent evidence before done=true",
		})
	}
	return forceStructuredObjectivesPending(ledger, reset)
}

func latestObservationIsPostWriteValidationRejection(observations []StructuredCommandObservation) bool {
	if len(observations) == 0 {
		return false
	}
	return strings.Contains(observations[len(observations)-1].Stderr, "completion rejected: write/edit/package mutation")
}

func deterministicCompletionEnforcerAcceptsDone(prompt string, ledger []StructuredObjective, observations []StructuredCommandObservation) bool {
	if len(pendingStructuredObjectives(ledger)) > 0 {
		return false
	}
	if !latestRealCommandSucceeded(observations) {
		return false
	}
	return !structuredCompletionNeedsPostWriteValidation(prompt, ledger, observations)
}

func forceStructuredObjectivesPending(ledger, reset []StructuredObjective) []StructuredObjective {
	out := mergeStructuredObjectiveLedger(nil, ledger)
	byID := map[string]StructuredObjective{}
	for _, objective := range reset {
		normalized, ok := normalizeStructuredObjective(objective)
		if ok {
			byID[normalized.ID] = normalized
		}
	}
	for i, objective := range out {
		if replacement, ok := byID[objective.ID]; ok {
			if strings.TrimSpace(replacement.Description) == "" {
				replacement.Description = objective.Description
			}
			if strings.TrimSpace(replacement.Source) == "" || replacement.Source == structuredObjectiveSourceModelInferred {
				replacement.Source = objective.Source
			}
			if !replacement.Required {
				replacement.Required = objective.Required
			}
			out[i] = replacement
			delete(byID, objective.ID)
		}
	}
	for _, objective := range byID {
		out = append(out, objective)
	}
	return out
}

func structuredCompletionNeedsPostWriteValidation(prompt string, ledger []StructuredObjective, observations []StructuredCommandObservation) bool {
	if !structuredTaskLooksLikeWriteOrEdit(prompt, ledger) {
		return false
	}
	lastMutation := -1
	for i, obs := range observations {
		if obs.ExitCode != 0 || strings.TrimSpace(obs.Command) == "" {
			continue
		}
		if structuredCommandMutatesWorkspace(obs.Command) {
			if structuredMutatingCommandIncludesValidation(obs.Command) {
				lastMutation = -1
				continue
			}
			lastMutation = i
		}
	}
	if lastMutation < 0 {
		return false
	}
	for _, obs := range observations[lastMutation+1:] {
		if obs.ExitCode == 0 && structuredCommandValidatesWorkspace(obs.Command) {
			return false
		}
	}
	return true
}

func structuredTaskLooksLikeWriteOrEdit(prompt string, ledger []StructuredObjective) bool {
	text := strings.ToLower(prompt + " " + structuredLedgerText(ledger))
	for _, marker := range []string{
		" add ", " create ", " edit ", " modify ", " update ", " write ", " install ", " initialize ", " set up ", " setup ",
		"package.json", "script", "dependency", "dependencies", "file", "directory", "project", "build artifact",
	} {
		if strings.Contains(" "+text+" ", marker) {
			return true
		}
	}
	return false
}

func structuredLedgerText(ledger []StructuredObjective) string {
	parts := []string{}
	for _, objective := range ledger {
		parts = append(parts, objective.ID, objective.Description)
	}
	return strings.Join(parts, " ")
}

func structuredCommandMutatesWorkspace(command string) bool {
	lower := strings.ToLower(command)
	for _, marker := range []string{
		"npm pkg set", "npm set-script", "npm install", "npm add", "npm init", "pnpm add", "yarn add",
		"sed -i", "perl -pi", "writefile", "writefilesync", "mkdir", "touch ", " tee ", "mv ", "cp ",
		">", ">>",
	} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func structuredMutatingCommandIncludesValidation(command string) bool {
	lower := strings.ToLower(command)
	for _, marker := range []string{
		"&& cat ", "&& sed -n", "&& rg ", "&& grep ", "&& ls ", "&& test ", "&& jq ", "&& npm pkg get", "&& npm ls", "&& node -e",
		"\ncat ", "\nsed -n", "\nrg ", "\ngrep ", "\nls ", "\ntest ", "\njq ", "\nnpm pkg get", "\nnpm ls", "\nnode -e",
		" curl ", "\ncurl ", " go test ", "\ngo test ", " go build ", "\ngo build ", " npm run build", "\nnpm run build",
		" docker inspect ", "\ndocker inspect ", " docker logs ", "\ndocker logs ",
	} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func structuredCommandValidatesWorkspace(command string) bool {
	lower := strings.ToLower(strings.TrimSpace(command))
	for _, marker := range []string{
		"cat ", "sed -n", "rg ", "grep ", "ls", "test ", "[ -", "jq ",
		"npm pkg get", "npm ls", "npm test", "npm run build", "npm run smoke", "npm run test", "npm run lint", "npm run typecheck", "node -e",
		"go test", "go build", "docker build", "docker inspect", "docker logs", "curl ",
	} {
		if strings.HasPrefix(lower, marker) || strings.Contains(lower, "&& "+marker) {
			return true
		}
	}
	return false
}

func structuredObservationsHavePackageManagerEvidence(observations []StructuredCommandObservation) bool {
	for _, obs := range observations {
		if strings.TrimSpace(obs.Command) == "" || obs.ExitCode != 0 {
			continue
		}
		if structuredCommandDiscoversPackageManager(obs.Command) {
			return true
		}
	}
	return false
}

func structuredCommandDiscoversPackageManager(command string) bool {
	lower := strings.ToLower(command)
	if !strings.Contains(lower, "command -v") && !strings.Contains(lower, "which ") && !strings.Contains(lower, "type -p") {
		return false
	}
	for _, manager := range []string{"pacman", "apt", "dnf", "yum", "zypper", "apk"} {
		if strings.Contains(lower, manager) {
			return true
		}
	}
	return false
}

func structuredCommandLooksLikeOSIdentification(command string) bool {
	lower := strings.ToLower(command)
	hasOSRelease := strings.Contains(lower, "/etc/os-release") || strings.Contains(lower, "os-release") || strings.Contains(lower, "pretty_name")
	hasUname := strings.Contains(lower, "uname")
	return hasOSRelease && hasUname
}

func structuredCommandLooksLikePartialOSIdentification(command string) bool {
	lower := strings.ToLower(command)
	for _, marker := range []string{"/etc/os-release", "os-release", "pretty_name", "uname", "lsb_release", "hostnamectl", "dpkg", "apt"} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func structuredCommandLooksLikeStableCurrentEventsEvidence(command string) bool {
	lower := strings.ToLower(command)
	return strings.Contains(lower, "news.google.com/rss/search") &&
		curlCommandFollowsRedirects(command) &&
		curlCommandHasSilentFlag(command) &&
		curlCommandHasUserAgent(command) &&
		strings.Contains(lower, "ceid=") &&
		!strings.Contains(lower, "```")
}
