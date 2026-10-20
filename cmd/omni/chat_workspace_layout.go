package main

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

const (
	workspaceMuted  = "38;2;139;148;170"
	workspaceBlue   = "38;2;122;162;247"
	workspacePurple = "38;2;187;154;247"
	workspaceGreen  = "38;2;158;206;106"
	workspaceAmber  = "38;2;224;175;104"
)

func (view *chatWorkspace) paint(text, color string) string {
	if !view.color {
		return text
	}
	if !strings.Contains(color, "48;") {
		color += ";48;2;22;22;30"
	}
	return "\x1b[" + color + "m" + text + "\x1b[0m"
}

func workspaceFit(text string, width int) string {
	width = max(0, width)
	text = ansi.Truncate(text, width, "")
	return text + strings.Repeat(" ", max(0, width-ansi.StringWidth(text)))
}

func (view *chatWorkspace) transcriptLocked(width int) ([]string, int, int) {
	screen := view.screenLocked()
	return workspaceWrappedRows(screen.lines, screen.soft, width, screen.row, screen.column)
}

func (view *chatWorkspace) activityLinesLocked(width int) []string {
	label := view.activity.Label
	if label == "" {
		label = "No active task"
	}
	lines := []string{label, ""}
	if view.activity.JobID > 0 {
		lines = append(lines, fmt.Sprintf("Job #%d · step %d", view.activity.JobID, view.activity.StepID))
	}
	if view.role != "" {
		lines = append(lines, view.role)
	}
	if view.model != "" {
		lines = append(lines, view.model)
	}
	if elapsed := view.elapsedLocked(); elapsed != "" {
		lines = append(lines, "Elapsed  "+elapsed)
	}
	if view.detail != "" {
		lines = append(lines, "", view.detail)
	}
	lines = append(lines, "", "RECENT ACTIVITY", "")
	if len(view.events) == 0 {
		lines = append(lines, "Commands, file changes and", "independent reviews appear", "here as they happen.")
	}
	for index := len(view.events) - 1; index >= 0; index-- {
		lines = append(lines, view.events[index], "")
	}
	var wrapped []string
	for _, line := range lines {
		wrapped = append(wrapped, strings.Split(ansi.Hardwrap(line, max(1, width), true), "\n")...)
	}
	return wrapped
}

func (view *chatWorkspace) paneGeometryLocked() (leftWidth, bodyHeight int, side, composeVisible bool) {
	reviewing := view.reviewVisibleLocked()
	leftWidth, bodyHeight = view.width, view.height-4
	side = view.width >= 100 && !view.zoom && view.focus != workspaceLedgerPane && view.focus != workspaceSessionsPane
	if side {
		leftWidth -= 35
	}
	composeVisible = view.editing && !reviewing && (!view.zoom || view.focus == workspaceCompose) && view.height >= 8
	if composeVisible {
		bodyHeight -= 1 + min(4, max(1, (view.height-5)/3))
	}
	return
}

func (view *chatWorkspace) frameLocked() ([]string, int, int, bool) {
	w, h := view.width, view.height
	reviewing := view.reviewVisibleLocked()
	frame := make([]string, h)
	name := filepath.Base(view.project)
	if view.project == "" {
		name = "workspace"
	}
	heading := "  " + strings.ToUpper(view.conversationMode) + "  /  " + name
	if view.branch != "" {
		heading += "  ·  " + view.branch
	}
	frame[0] = view.paint(workspaceFit(heading, w), "1;"+workspaceBlue)
	if h < 5 {
		return frame, 0, 0, false
	}
	leftWidth, bodyHeight, side, composeVisible := view.paneGeometryLocked()
	columns := max(1, leftWidth-4)
	title := "  01  Conversation"
	if reviewing {
		title = "  01  Plan approval"
	}
	if view.focus == workspaceActivity && !side && !reviewing {
		title = "  02  Activity"
	}
	if view.focus == workspaceConversation && !reviewing {
		title = "▸ " + strings.TrimSpace(title)
	}
	if view.focus == workspaceLedgerPane && !reviewing {
		title = "▸ 03  Ledger"
	}
	if view.focus == workspaceSessionsPane {
		title = "▸ 04 Sessions"
	}
	frame[1] = view.paint(workspaceFit(title, leftWidth), workspacePurple)
	if side {
		activityTitle := "│ 02  Activity"
		if view.focus == workspaceActivity {
			activityTitle = "│ ▸ 02  Activity"
		}
		frame[1] += view.paint(workspaceFit(activityTitle, 35), workspacePurple)
	}
	lines, input, cursorRow, cursorColumn := view.contentLocked(columns)
	start := max(0, len(lines)-bodyHeight-view.conversationOffset)
	padding := max(0, bodyHeight-len(lines))
	if reviewing && view.reviewFocus {
		start = view.reviewStartLocked(columns, bodyHeight, len(lines))
		padding = 0
	}
	if view.conversationMode == "research" && !reviewing {
		lines = view.researchContentLocked(columns)
		start, padding = max(0, len(lines)-bodyHeight-view.conversationOffset), 0
	}
	activityWidth := 31
	if !side {
		activityWidth = columns
	}
	activity := view.activityLinesLocked(max(1, activityWidth))
	if view.focus == workspaceLedgerPane && !reviewing {
		lines = view.ledgerLinesLocked(columns)
		if view.ledger.followSelection {
			view.revealLedgerSelectionLocked(columns, bodyHeight)
			view.ledger.followSelection = false
		}
		view.ledger.offset = min(view.ledger.offset, max(0, len(lines)-bodyHeight))
		start, padding = view.ledger.offset, 0
	}
	if view.focus == workspaceSessionsPane {
		var selectedRow int
		lines, selectedRow = view.sessionsLinesLocked(columns)
		start, padding = max(0, selectedRow-bodyHeight/2), 0
	}

	for index := 0; index < bodyHeight; index++ {
		text := ""
		lineIndex := start + index - padding
		if lineIndex >= 0 && lineIndex < len(lines) {
			text = lines[lineIndex]
		}
		if view.focus == workspaceActivity && !side && !reviewing {
			text = ""
			activityIndex := index + view.activityOffset
			if activityIndex < len(activity) {
				text = activity[activityIndex]
			}
		}
		color := workspaceTextColor(text)
		frame[index+2] = view.paint(workspaceFit("  "+text, leftWidth), color)
		if side {
			text = ""
			if index+view.activityOffset < len(activity) {
				text = activity[index+view.activityOffset]
			}
			frame[index+2] += view.paint("│ ", workspaceMuted) + view.paint(workspaceFit(text, 33), workspaceMuted)
		}
	}
	mode := " INSERT "
	if view.focus != workspaceCompose {
		mode = " CHAT "
		if view.focus == workspaceActivity {
			mode = " ACTIVITY "
		}
	}
	if view.focus == workspaceLedgerPane {
		mode = " LEDGER "
	}
	if view.focus == workspaceSessionsPane {
		mode = " SESSIONS "
	}
	if reviewing {
		mode = " APPROVAL "
	}
	mode = ansi.Truncate(mode, w, "")
	status := view.activity.Label
	if status == "" {
		status = "Ready"
	}
	if view.activity.JobID > 0 {
		status = fmt.Sprintf("#%d  %s", view.activity.JobID, status)
	}
	if elapsed := view.elapsedLocked(); elapsed != "" {
		status += "  ·  " + elapsed
	}
	if view.role != "" {
		status += "  ·  " + view.role
	}
	if view.conversationOffset > 0 {
		status += fmt.Sprintf("  ·  %d lines up", view.conversationOffset)
	}
	frame[h-2] = view.paint(mode, "1;38;2;22;22;30;48;2;122;162;247") + view.paint(workspaceFit("  "+status, max(0, w-len(mode))), workspaceBlue)
	hint := "  Ctrl-B ? keys · Enter send · Ctrl-B tabs · Ctrl-C interrupt"
	if view.focus != workspaceCompose {
		hint = "  j/k scroll · PgUp/PgDn page · G latest · i compose · Ctrl-B z zoom"
	}
	if view.focus == workspaceLedgerPane {
		hint = "  j/k select · PgUp/PgDn scroll · a agree · d discuss · x remove · s restore · [/] reorder · r refresh · t retry"
	}
	if view.focus == workspaceSessionsPane {
		hint = "  j/k select · Enter switch · r refresh · i compose"
	}
	if view.prefix {
		hint = "  ? keys · f/e/l/r modes · 1 compose · 2 conversation · 3 activity · 4 ledger · 5 sessions · o cycle"
	}
	if reviewing {
		hint = "  ↑/↓ select · Space decide · Enter confirm · N note · Ctrl-D exit"
	}
	if view.help && !reviewing {
		hint = "  j/k scroll keys · Esc close"
	}
	frame[h-1] = view.paint(workspaceFit(hint, w), workspaceMuted)
	row := 2 + padding + cursorRow - start
	column := min(w-1, cursorColumn+2)
	if composeVisible {
		row, column = view.composerFrameLocked(frame, input, cursorRow, cursorColumn)
	}
	if view.help && !reviewing {
		view.helpFrameLocked(frame)
	}
	visible := view.cursorVisible && view.focus == workspaceCompose && !view.help && view.conversationOffset == 0 && row >= 2 && row < h-2
	return frame, row, column, visible
}

func workspaceTextColor(text string) string {
	trimmed := strings.TrimSpace(text)
	switch {
	case strings.HasPrefix(trimmed, "you"), strings.HasPrefix(trimmed, ">"):
		return "1;" + workspaceBlue
	case strings.HasPrefix(trimmed, "omni>"):
		return workspaceGreen
	case strings.HasPrefix(trimmed, "#"), strings.HasPrefix(trimmed, "PLAN APPROVAL"):
		return "1;" + workspacePurple
	case strings.HasPrefix(trimmed, "[omni]"):
		return workspaceMuted
	case strings.HasPrefix(trimmed, "run>"), strings.HasPrefix(trimmed, "check>"):
		return workspaceAmber
	default:
		return "38;2;216;222;233"
	}
}

func (view *chatWorkspace) helpFrameLocked(frame []string) {
	for index := 2; index < len(frame)-2; index++ {
		frame[index] = view.paint(workspaceFit("", view.width), workspaceMuted)
	}
	lines := []string{"KEYS  /  terminal workspace", "Ctrl-B f/e/l/r  Infer/Interact/Ledger/Research", "Ctrl-B Tab or [ / ]  Cycle tabs", "Ctrl-B 1   Compose your message", "Ctrl-B 2   Browse the conversation", "Ctrl-B 3   Inspect live activity", "Ctrl-B 4   Inspect the current ledger", "Ctrl-B 5   Switch sessions (background work continues)", "Ledger: j/k select; PgUp/PgDn scroll", "a agree · d discuss · x remove · s restore", "[ / ] reorder · r refresh · t retry", "Ctrl-B o   Focus the next pane", "Ctrl-B z   Zoom / restore the focused pane", "Ctrl-B ?   Show / hide this key guide", "", "j / k      Scroll the focused pane", "PgUp/PgDn  Scroll a page", "g / G      Oldest / latest", "i / Esc    Return to composing", "", "Enter      Send the current message", "↑ / ↓      Recall input history", "Ctrl-C     Interrupt using existing job controls", "Ctrl-D     Exit with an empty draft", "/help      Existing chat commands", "", "Esc closes this guide. Your draft stays intact."}
	var wrapped []string
	for _, line := range lines {
		wrapped = append(wrapped, strings.Split(ansi.Hardwrap(line, max(1, view.width-2), true), "\n")...)
	}
	view.helpOffset = min(view.helpOffset, max(0, len(wrapped)-max(1, len(frame)-4)))
	for index, line := range wrapped[view.helpOffset:] {
		if index+2 >= len(frame)-2 {
			break
		}
		frame[index+2] = view.paint(workspaceFit("  "+line, view.width), workspaceBlue)
	}
}

func (view *chatWorkspace) drawLocked() error {
	if view.closed {
		return nil
	}
	frame, row, column, visible := view.frameLocked()
	var output strings.Builder
	changed := false
	for index, line := range frame {
		if len(view.lastFrame) == len(frame) && view.lastFrame[index] == line {
			continue
		}
		changed = true
		fmt.Fprintf(&output, "\x1b[%d;1H\x1b[2K%s", index+1, line)
	}
	if !changed && row == view.lastCursorRow && column == view.lastCursorColumn && visible == view.lastCursorVisible {
		return nil
	}
	commands := "\x1b[?25l" + output.String()
	output.Reset()
	output.WriteString(commands)
	if visible {
		fmt.Fprintf(&output, "\x1b[%d;%dH\x1b[?25h", row+1, column+1)
	}
	if err := writeTerminalControl(view.output, output.String()); err != nil {
		return err
	}
	view.lastFrame = frame
	view.lastCursorRow, view.lastCursorColumn, view.lastCursorVisible = row, column, visible
	return nil
}

func (view *chatWorkspace) researchContentLocked(width int) []string {
	var lines []string
	for _, line := range view.researchLines {
		lines = append(lines, strings.Split(ansi.Hardwrap(line, max(1, width), true), "\n")...)
	}
	return lines
}
