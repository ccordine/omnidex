package main

import (
	"encoding/json"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"
	"github.com/gryph/omnidex/internal/client"
	"github.com/gryph/omnidex/internal/model"
)

type ledgerHit struct {
	row, left, right int
	field            int
	key              byte
	item             model.LedgerItemID
}
type ledgerEditor struct {
	mode             string
	task             model.LedgerTask
	fields           []string
	selected, cursor int
	pending          []byte
}

var ledgerFieldNames = []string{"Name", "Instructions", "Owner context (one entry per line)", "Constraints (one per line)", "File responsibilities (JSON)", "Acceptance criteria and verification (JSON)", "Read files (one per line)", "Write files (one per line)", "Delete files (one per line)"}

func ledgerJSON(value any) string { raw, _ := json.MarshalIndent(value, "", "  "); return string(raw) }

func (view *chatWorkspace) openLedgerEditorLocked(mode string) error {
	panel := &view.ledger
	if panel.snapshot.State != "proposed" && panel.snapshot.State != "revising" && panel.snapshot.State != "frozen" {
		panel.feedback = "This ledger is " + panel.snapshot.State + " and is read-only."
		return view.drawLocked()
	}
	var task model.LedgerTask
	for _, item := range panel.snapshot.Ledger.Tasks {
		if item.ID == panel.selected {
			task = item
			break
		}
	}
	if mode == "new" {
		task = model.LedgerTask{}
	}
	if mode != "new" && task.Disposition == model.LedgerDispositionTombstoned {
		panel.feedback = "Restore this item before editing it."
		return view.drawLocked()
	}
	context := []string{}
	for _, entry := range task.OwnerContext {
		context = append(context, entry.Text)
	}
	fields := []string{task.Goal, task.Objective, strings.Join(context, "\n"), strings.Join(task.Constraints, "\n"), ledgerJSON(task.FileResponsibilities), ledgerJSON(task.Acceptance), strings.Join(task.ReadFiles, "\n"), strings.Join(task.WriteFiles, "\n"), strings.Join(task.DeleteFiles, "\n")}
	if mode == "planner" {
		fields = []string{""}
	}
	panel.editor = &ledgerEditor{mode: mode, task: task, fields: fields, cursor: utf8.RuneCountInString(fields[0])}
	panel.offset = 0
	return view.drawLocked()
}

func ledgerTextLines(value string) []string {
	var lines []string
	for _, line := range strings.Split(value, "\n") {
		if strings.TrimSpace(line) != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

func (view *chatWorkspace) saveLedgerEditorLocked() error {
	panel := &view.ledger
	editor := panel.editor
	if editor == nil || panel.saving {
		return view.drawLocked()
	}
	request := workspaceLedgerRequest{jobID: panel.snapshot.JobID, generation: panel.snapshot.Generation, revision: panel.snapshot.Revision}
	if editor.mode == "planner" {
		if strings.TrimSpace(editor.fields[0]) == "" {
			return view.drawLocked()
		}
		request.key, request.text = 'p', editor.fields[0]
		request.action.TargetID = editor.task.ID
	} else {
		task := editor.task
		task.Goal, task.Objective = editor.fields[0], editor.fields[1]
		if strings.TrimSpace(task.Goal) == "" {
			panel.feedback = "Enter an item name."
			panel.notice = panel.feedback
			return view.drawLocked()
		}
		oldContext := []string{}
		for _, entry := range editor.task.OwnerContext {
			oldContext = append(oldContext, entry.Text)
		}
		if editor.fields[2] != strings.Join(oldContext, "\n") {
			task.OwnerContext = nil
			for _, text := range ledgerTextLines(editor.fields[2]) {
				task.OwnerContext = append(task.OwnerContext, model.TaskContext{Text: text})
			}
		}
		task.Constraints = ledgerTextLines(editor.fields[3])
		if err := json.Unmarshal([]byte(editor.fields[4]), &task.FileResponsibilities); err != nil {
			panel.feedback = "File responsibilities: " + err.Error()
			panel.notice = panel.feedback
			return view.drawLocked()
		}
		if err := json.Unmarshal([]byte(editor.fields[5]), &task.Acceptance); err != nil {
			panel.feedback = "Acceptance criteria: " + err.Error()
			panel.notice = panel.feedback
			return view.drawLocked()
		}
		task.ReadFiles, task.WriteFiles, task.DeleteFiles = ledgerTextLines(editor.fields[6]), ledgerTextLines(editor.fields[7]), ledgerTextLines(editor.fields[8])
		kind := model.LedgerMutationAmend
		if editor.mode == "new" {
			kind = model.LedgerMutationAdd
		}
		request.key = 'e'
		request.action = client.DirectPlanningAction{Kind: kind, TargetID: task.ID, Item: &task, Rationale: "Manual ledger item edit."}
	}
	return view.ledgerRequestLocked(request)
}

func (view *chatWorkspace) ledgerEditorKeyLocked(key byte) error {
	e := view.ledger.editor
	if e == nil {
		return nil
	}
	if key == 27 {
		view.ledger.editor = nil
		return view.drawLocked()
	}
	if key == 19 || e.mode == "planner" && key == '\r' {
		return view.saveLedgerEditorLocked()
	}
	if key == '\t' {
		e.selected = (e.selected + 1) % len(e.fields)
		e.cursor = utf8.RuneCountInString(e.fields[e.selected])
		view.ledger.offset = 0
		return view.drawLocked()
	}
	runes := []rune(e.fields[e.selected])
	e.cursor = min(e.cursor, len(runes))
	switch key {
	case 1:
		e.cursor = 0
	case 5:
		e.cursor = len(runes)
	case 21:
		runes = nil
		e.cursor = 0
	case 127, 8:
		if e.cursor > 0 {
			runes = append(runes[:e.cursor-1], runes[e.cursor:]...)
			e.cursor--
		}
	case 3:
		if len(runes) > 0 {
			runes = nil
			e.cursor = 0
		} else {
			view.ledger.editor = nil
			return view.drawLocked()
		}
	default:
		if key == '\r' {
			key = '\n'
		}
		if key >= 32 || key == '\n' {
			e.pending = append(e.pending, key)
			if utf8.FullRune(e.pending) {
				r, _ := utf8.DecodeRune(e.pending)
				e.pending = nil
				runes = append(runes[:e.cursor], append([]rune{r}, runes[e.cursor:]...)...)
				e.cursor++
			}
		}
	}
	e.fields[e.selected] = string(runes)
	return view.drawLocked()
}

func (view *chatWorkspace) ledgerEditorLinesLocked(width int) []string {
	p := &view.ledger
	e := p.editor
	title := "Edit item"
	if e.mode == "new" {
		title = "New item"
	}
	if e.mode == "planner" {
		title = "Planner · " + e.task.Goal
	}
	lines := []string{p.feedback, title, "Ctrl-S save · Tab next field · Esc cancel", ""}
	lines[2] = "[Save] [Cancel] · Ctrl-S save · Tab next field"
	if e.mode == "planner" {
		lines[2] = "[Send] [Back] · Enter send · Ctrl-J newline"
		lines = append(lines, "Edit this item with its conversation and read-only context from preceding incomplete tasks.")
	}
	if e.mode == "planner" {
		for _, message := range p.snapshot.PlanningMessages {
			if message.ItemID == e.task.ID {
				label := "Planner"
				if message.Role == "user" {
					label = "You"
				}
				lines = append(lines, "", label+": "+message.Text)
			}
		}
	}
	fieldRows := map[int]int{}
	for i, value := range e.fields {
		label := "Message"
		if e.mode != "planner" {
			label = ledgerFieldNames[i]
		}
		marker := "  "
		if i == e.selected {
			marker = "> "
		}
		fieldRows[len(lines)] = i + 1
		lines = append(lines, marker+label)
		if i == e.selected {
			lines = append(lines, strings.Split(value, "\n")...)
		} else {
			summary := strings.ReplaceAll(value, "\n", " · ")
			lines = append(lines, "  "+ansi.Truncate(summary, max(1, width-2), "…"))
		}
		lines = append(lines, "")
	}
	var wrapped []string
	p.hits = nil
	for index, line := range lines {
		if index == 2 {
			p.hits = append(p.hits, ledgerHit{row: len(wrapped), left: 0, right: 6, key: 19}, ledgerHit{row: len(wrapped), left: 7, right: 15, key: 27})
		}
		if field := fieldRows[index]; field > 0 {
			p.hits = append(p.hits, ledgerHit{row: len(wrapped), left: 0, right: width, field: field})
		}
		wrapped = append(wrapped, strings.Split(ansi.Hardwrap(safeConsoleText(line), max(1, width), true), "\n")...)
	}
	return wrapped
}

func (source *workspaceInputSource) consumeLedgerEditor(value byte) error {
	view := source.view
	if len(source.candidate) > 0 || value == 27 {
		source.candidate = append(source.candidate, value)
		if strings.HasPrefix(string(source.candidate), "\x1b[<") {
			if value == 'M' || value == 'm' {
				sequence := string(source.candidate)
				source.candidate = nil
				if value == 'M' {
					return view.ledgerMouse(sequence)
				}
			}
			return nil
		}
		for _, sequence := range []string{"\x1b[200~", "\x1b[201~", "\x1b[D", "\x1b[C", "\x1b[Z"} {
			if string(source.candidate) == sequence {
				source.candidate = nil
				view.mu.Lock()
				defer view.mu.Unlock()
				e := view.ledger.editor
				if e == nil {
					return nil
				}
				switch sequence {
				case "\x1b[200~":
					source.paste = true
				case "\x1b[201~":
					source.paste = false
				case "\x1b[D":
					e.cursor = max(0, e.cursor-1)
				case "\x1b[C":
					e.cursor = min(utf8.RuneCountInString(e.fields[e.selected]), e.cursor+1)
				case "\x1b[Z":
					e.selected = (e.selected + len(e.fields) - 1) % len(e.fields)
					e.cursor = utf8.RuneCountInString(e.fields[e.selected])
				}
				return view.drawLocked()
			}
			if strings.HasPrefix(sequence, string(source.candidate)) {
				return nil
			}
		}
		source.candidate = nil
		return nil
	}
	view.mu.Lock()
	defer view.mu.Unlock()
	if source.paste && (value == '\r' || value == '\n') {
		e := view.ledger.editor
		if e != nil {
			old := e.mode
			e.mode = "edit"
			err := view.ledgerEditorKeyLocked('\n')
			e.mode = old
			return err
		}
	}
	return view.ledgerEditorKeyLocked(value)
}

func (view *chatWorkspace) ledgerMouse(sequence string) error {
	parts := strings.Split(strings.TrimSuffix(strings.TrimPrefix(sequence, "\x1b[<"), "M"), ";")
	if len(parts) != 3 {
		return nil
	}
	button, e1 := strconv.Atoi(parts[0])
	x, e2 := strconv.Atoi(parts[1])
	y, e3 := strconv.Atoi(parts[2])
	if e1 != nil || e2 != nil || e3 != nil {
		return nil
	}
	view.mu.Lock()
	defer view.mu.Unlock()
	if view.focus != workspaceLedgerPane {
		return nil
	}
	if button == 64 || button == 65 {
		delta := 3
		if button == 65 {
			delta = -3
		}
		view.scrollLocked(delta)
		return view.drawLocked()
	}
	if button != 0 {
		return nil
	}
	_, height, _, _ := view.paneGeometryLocked()
	if y < 3 || y >= 3+height {
		return nil
	}
	row, column := y-3+view.ledger.offset, x-3
	for _, hit := range view.ledger.hits {
		if row == hit.row && column >= hit.left && column < hit.right {
			if editor := view.ledger.editor; editor != nil {
				if hit.field > 0 {
					editor.selected = hit.field - 1
					editor.cursor = utf8.RuneCountInString(editor.fields[editor.selected])
					return view.drawLocked()
				}
				return view.ledgerEditorKeyLocked(hit.key)
			}
			if hit.item != "" {
				view.ledger.selected, view.ledger.details, view.ledger.offset = hit.item, true, 0
				return view.drawLocked()
			}
			return view.ledgerKeyLocked(hit.key)
		}
	}
	return nil
}

func (view *chatWorkspace) interruptLedgerEditor() bool {
	view.mu.Lock()
	defer view.mu.Unlock()
	if view.ledger.editor == nil {
		return false
	}
	_ = view.ledgerEditorKeyLocked(3)
	return true
}

func (view *chatWorkspace) ledgerEditorCursorLocked(width int) (int, int) {
	e := view.ledger.editor
	label := "> Message"
	if e.mode != "planner" {
		label = "> " + ledgerFieldNames[e.selected]
	}
	lines := view.ledgerEditorLinesLocked(width)
	start := 0
	for i, line := range lines {
		if line == label {
			start = i + 1
			break
		}
	}
	runes := []rune(e.fields[e.selected])
	prefix := string(runes[:min(e.cursor, len(runes))])
	parts := strings.Split(prefix, "\n")
	row := start
	for _, line := range parts[:len(parts)-1] {
		row += max(1, (ansi.StringWidth(safeConsoleText(line))+max(1, width)-1)/max(1, width))
	}
	cells := ansi.StringWidth(safeConsoleText(parts[len(parts)-1]))
	return row + cells/max(1, width), cells % max(1, width)
}
