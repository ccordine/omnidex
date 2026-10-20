package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

func (view *chatWorkspace) focusReview(text string, selected int, confirming bool, empty bool) error {
	offset := 0
	if !empty {
		needle := fmt.Sprintf("\n> %d. [", selected+1)
		if confirming {
			needle = "\nFreeze this work set:"
		}
		offset = strings.Index(text, needle)
		if offset < 0 {
			return fmt.Errorf("plan review is missing its code-rendered selection")
		}
		offset++
	}
	view.mu.Lock()
	defer view.mu.Unlock()
	view.reviewFocusPrefix = strings.ReplaceAll(safeConsoleText(text[:offset]), "\n", "\r\n")
	view.reviewFocus = true
	return nil
}

func (view *chatWorkspace) reviewStartLocked(width, height, total int) int {
	screen := newWorkspaceScreen(view.review.width)
	for _, character := range view.reviewFocusPrefix {
		screen.put(character)
	}
	_, row, _ := workspaceWrappedRows(screen.lines, screen.soft, width, screen.row, screen.column)
	return min(max(0, total-height), max(0, row-height/3))
}

func workspaceWrappedRows(rows [][]rune, soft []bool, width, cursorRow, cursorColumn int) ([]string, int, int) {
	var lines []string
	row, column := 0, 0
	var paragraph strings.Builder
	for index, line := range rows {
		text := string(line)
		if index == cursorRow {
			prefix := paragraph.String() + string(line[:min(len(line), max(0, cursorColumn))])
			cells := ansi.StringWidth(prefix)
			row, column = len(lines)+cells/max(1, width), cells%max(1, width)
		}
		paragraph.WriteString(text)
		if index == len(rows)-1 || !soft[index] {
			lines = append(lines, strings.Split(ansi.Hardwrap(paragraph.String(), max(1, width), true), "\n")...)
			paragraph.Reset()
		}
	}
	return lines, row, column
}

func (view *chatWorkspace) contentLocked(width int) ([]string, []string, int, int) {
	if !view.editing || view.reviewing {
		lines, row, column := view.transcriptLocked(width)
		return lines, nil, row, column
	}
	screen := view.conversation
	start := min(len(screen.lines), max(0, view.draftRow-(screen.dropped-view.draftDropped)))
	history := append([][]rune(nil), screen.lines[:start]...)
	draft := append([][]rune(nil), screen.lines[start:]...)
	column := view.draftColumn
	if len(draft) > 0 && column > 0 {
		column = min(column, len(draft[0]))
		history = append(history, draft[0][:column])
		draft[0] = draft[0][column:]
	}
	chat, _, _ := workspaceWrappedRows(history, screen.soft[:len(history)], width, -1, 0)
	editorWidth, _ := view.editorSizeLocked()
	draftColumn := screen.column
	if screen.row == start {
		draftColumn -= column
	}
	input, row, cursor := workspaceWrappedRows(draft, screen.soft[start:], editorWidth, screen.row-start, draftColumn)
	return chat, input, row, cursor
}

func (view *chatWorkspace) composerFrameLocked(frame []string, input []string, row, column int) (int, int) {
	height := min(4, max(1, (view.height-5)/3))
	border := view.height - 3 - height
	label := "  ── 03  Compose "
	color := workspaceMuted
	if view.focus == workspaceCompose {
		label = "  ── 03  Compose  •"
		color = workspaceBlue
	}
	frame[border] = view.paint(workspaceFit(label+" "+strings.Repeat("─", max(0, view.width-ansi.StringWidth(label)-1)), view.width), color)
	start := max(0, row-height+1)
	for index := 0; index < height; index++ {
		text := ""
		if start+index < len(input) {
			text = input[start+index]
		}
		frame[border+1+index] = view.paint(workspaceFit("  "+text, view.width), "38;2;216;222;233")
	}
	return border + 1 + row - start, min(view.width-1, column+2)
}
