package runtime

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// One turn's bounded PTY evidence is compared with its authoritative journal.
// Exceeding the existing transcript budget fails rather than losing call IDs.
type terminalWorkflowActivity struct {
	seen          map[string]bool
	tail          string
	requireOutput bool
	transcript    strings.Builder
}

func (activity *terminalWorkflowActivity) observe(t *testing.T, text string) {
	t.Helper()
	if activity.seen == nil {
		activity.seen = make(map[string]bool)
	}
	if activity.transcript.Len()+len(text) > 256*1024 {
		t.Fatal("terminal turn exceeded its bounded evidence budget")
	}
	activity.transcript.WriteString(text)
	text = activity.tail + text
	for _, marker := range []string{"Candidate:", "Why:", "Options:", "Result:", "Code rule:", "Call:", "thinking>", "command>", "exit 0", "file read", "read ·", "list ·", "output>", "Written files:", "Planning · started", "Planning · completed", "Executing task · started", "Executing task · completed", "Reviewing · started", "Reviewing · completed", "Overall review · started", "Overall review · completed"} {
		activity.seen[marker] = activity.seen[marker] || strings.Contains(text, marker)
	}
	if len(text) > 128 {
		text = text[len(text)-128:]
	}
	activity.tail = text
}

func (activity *terminalWorkflowActivity) assert(t *testing.T) {
	t.Helper()
	for _, marker := range []string{"command>", "exit 0", "list ·", "Written files:", "Planning · started", "Planning · completed", "Executing task · started", "Executing task · completed", "Reviewing · started", "Reviewing · completed", "Overall review · started", "Overall review · completed"} {
		if !activity.seen[marker] {
			t.Errorf("actual terminal workflow omitted evidence %q", marker)
		}
	}
	if !activity.seen["file read"] && !activity.seen["read ·"] {
		t.Error("actual terminal workflow omitted file reads")
	}
	if activity.requireOutput && !activity.seen["output>"] {
		t.Error("verbose terminal workflow omitted actual command output")
	}
	if !activity.seen["thinking>"] {
		t.Log("no native public summary was observed; this default-effort turn does not qualify summary availability")
	}
}

var terminalControlSequence = regexp.MustCompile(`\x1b\[[0-?]*[ -/]*[@-~]`)
var terminalClearLine = regexp.MustCompile(`\x1b\[[012]?K`)
var terminalPlanningHeader = regexp.MustCompile(`(?m)^omni> `)
var terminalPresentationLine = regexp.MustCompile(`(?m)^[^\t \n][^\n]*`)
var terminalCallFooter = regexp.MustCompile(`^      Model: [^\n]+\n      Call: ([1-9][0-9]*)$`)

func terminalPlainText(text string) string {
	text = terminalClearLine.ReplaceAllString(text, "\n")
	text = terminalControlSequence.ReplaceAllString(text, "")
	return strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n")
}

func terminalPlanningBlockStart(text string) int {
	headers := terminalPlanningHeader.FindAllStringIndex(text, -1)
	if len(headers) == 0 {
		return -1
	}
	return headers[len(headers)-1][0]
}

// Rendered content is indented; only a semantic block's final Model/Call
// footer carries identity. Literal labels within returned content stay data.
func terminalVisibleCallMatches(text string) [][]int {
	lines := terminalPresentationLine.FindAllStringIndex(text, -1)
	var matches [][]int
	for index, line := range lines {
		if !strings.HasPrefix(text[line[0]:], "omni> ") {
			continue
		}
		end := len(text)
		if index+1 < len(lines) {
			end = lines[index+1][0]
		}
		block := text[line[0]:end]
		model := strings.LastIndex(block, "\n      Model: ")
		if model < 0 {
			continue
		}
		footer := strings.TrimRight(block[model+1:], "\n\t ")
		match := terminalCallFooter.FindStringSubmatchIndex(footer)
		if match == nil {
			continue
		}
		match[0] = strings.LastIndex(footer, "\n      Call: ") + 1
		for field := range match {
			match[field] += line[0] + model + 1
		}
		matches = append(matches, match)
	}
	return matches
}

func terminalContainsFacts(text string, facts []string) bool {
	text = strings.Join(strings.Fields(text), " ")
	for _, fact := range facts {
		if !strings.Contains(text, strings.Join(strings.Fields(fact), " ")) {
			return false
		}
	}
	return true
}

func terminalHasFullResult(text string, callID int64, expected string) bool {
	var result strings.Builder
	for _, match := range terminalVisibleCallMatches(text) {
		if text[match[2]:match[3]] != strconv.FormatInt(callID, 10) {
			continue
		}
		start := terminalPlanningBlockStart(text[:match[0]])
		if start < 0 {
			continue
		}
		block, value := text[start:match[0]], ""
		if first := strings.Index(block, "\n      Result: "); first >= 0 {
			end := strings.LastIndex(block, "\n      Code rule: ")
			if end < first {
				continue
			}
			value = block[first+len("\n      Result: ") : end]
			value = strings.ReplaceAll(value, "\n      ", "\n")
			result.Reset()
		} else {
			line := strings.IndexByte(block, '\n')
			end := strings.LastIndex(block, "\n      Model: ")
			if line < 0 || end < line {
				continue
			}
			var from, to, total int
			count, err := fmt.Sscanf(block[:line], "omni> Result continued (bytes %d–%d of %d):", &from, &to, &total)
			value = strings.TrimPrefix(block[line+1:end], "      ")
			if count != 3 || err != nil || from != result.Len()+1 || total != len(expected) {
				continue
			}
			value = strings.ReplaceAll(value, "\n      ", "\n")
			if to != from+len(value)-1 {
				continue
			}
		}
		result.WriteString(value)
		if result.String() == expected {
			return true
		}
	}
	return false
}

func (activity *terminalWorkflowActivity) planningCallVisible(call terminalSemanticCall, usedOpenings map[int]bool) (int, bool, error) {
	opening, completed, _, err := terminalSemanticFacts(call)
	if err != nil {
		return 0, false, err
	}
	text := terminalPlainText(activity.transcript.String())
	result := strings.TrimPrefix(completed[1], "Result: ")
	completed[1] = "Result:"
	if !terminalHasFullResult(text, call.id, result) {
		return 0, false, nil
	}
	headers := terminalPlanningHeader.FindAllStringIndex(text, -1)
	matches := terminalVisibleCallMatches(text)
	for matchIndex, match := range matches {
		id, err := strconv.ParseInt(text[match[2]:match[3]], 10, 64)
		if err != nil || id != call.id {
			continue
		}
		start := terminalPlanningBlockStart(text[:match[0]])
		if start < 0 || !terminalContainsFacts(text[start:match[1]], completed) {
			continue
		}
		floor := 0
		for previous := matchIndex - 1; previous >= 0; previous-- {
			if text[matches[previous][2]:matches[previous][3]] != strconv.FormatInt(call.id, 10) {
				floor = matches[previous][1]
				break
			}
		}
		for index := len(headers) - 2; index >= 0; index-- {
			opened, end := headers[index][0], headers[index+1][0]
			if opened < floor || opened >= start || usedOpenings[opened] || strings.HasPrefix(text[opened:], "omni> Options continued (") {
				continue
			}
			for page := index + 1; page+1 < len(headers) && headers[page][0] < start && strings.HasPrefix(text[headers[page][0]:], "omni> Options continued ("); page++ {
				end = headers[page+1][0]
			}
			if terminalContainsFacts(text[opened:end], opening) {
				return opened, true, nil
			}
		}
	}
	return 0, false, nil
}

func (activity *terminalWorkflowActivity) assertSemanticCalls(t *testing.T, calls []terminalSemanticCall) {
	t.Helper()
	if len(calls) == 0 {
		t.Fatal("ordinary terminal job produced no authoritative semantic call evidence")
	}
	expected, observed, openings := make(map[int64]bool), make(map[int64]bool), make(map[int]bool)
	for _, call := range calls {
		expected[call.id] = true
		opening, visible, err := activity.planningCallVisible(call, openings)
		if err != nil || !visible {
			t.Errorf("journal semantic call %d (%s, step %d attempt %d) omitted its actual question, reason, offered options, result, code rule, model or Call ID: %v", call.id, call.kind, call.step, call.attempt, err)
		}
		openings[opening] = true
	}
	text := terminalPlainText(activity.transcript.String())
	for _, match := range terminalVisibleCallMatches(text) {
		label := text[match[2]:match[3]]
		id, err := strconv.ParseInt(label, 10, 64)
		if err != nil || !expected[id] {
			t.Errorf("terminal displayed Call ID %s outside this job's successful semantic journal", label)
		}
		observed[id] = true
	}
	for id := range expected {
		if !observed[id] {
			t.Errorf("successful semantic journal Call ID %d was never visible in the real PTY", id)
		}
	}
}
