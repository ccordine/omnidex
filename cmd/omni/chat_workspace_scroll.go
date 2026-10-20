package main

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// Locate the source character at the top of a scrolled viewport before reflow.
// Soft wraps belong to the same source paragraph; hard newlines remain distinct.
func workspaceRowAnchor(screen *workspaceScreen, width, target int) (int, int) {
	base := 0
	for first := 0; first < len(screen.lines); {
		end := first
		var paragraph strings.Builder
		for {
			paragraph.WriteString(string(screen.lines[end]))
			end++
			if end == len(screen.lines) || !screen.soft[end-1] {
				break
			}
		}
		count := len(strings.Split(ansi.Hardwrap(paragraph.String(), max(1, width), true), "\n"))
		if target < base+count {
			prefix := []rune(ansi.Truncate(paragraph.String(), max(0, target-base)*max(1, width), ""))
			remaining := len(prefix)
			for row := first; row < end; row++ {
				if remaining <= len(screen.lines[row]) {
					return row, remaining
				}
				remaining -= len(screen.lines[row])
			}
			return end - 1, len(screen.lines[end-1])
		}
		base += count
		first = end
	}
	return screen.row, screen.column
}

type workspaceResizeAnchor struct {
	mode        string
	row, column int
}

func (view *chatWorkspace) scrollScreenLocked(mode string) workspaceScreen {
	if mode != "research" {
		return view.conversation
	}
	screen := newWorkspaceScreen(1)
	screen.lines, screen.soft = nil, nil
	for _, line := range view.researchLines {
		for _, row := range strings.Split(line, "\n") {
			screen.lines = append(screen.lines, []rune(row))
			screen.soft = append(screen.soft, false)
		}
	}
	return screen
}

func (view *chatWorkspace) resizeAnchorsLocked() []workspaceResizeAnchor {
	active, focus := view.conversationMode, view.focus
	defer func() { view.conversationMode, view.focus = active, focus }()
	offsets := make(map[string]int, len(view.modeOffsets)+1)
	for mode, offset := range view.modeOffsets {
		offsets[mode] = offset
	}
	offsets[active] = view.conversationOffset
	var anchors []workspaceResizeAnchor
	for mode, offset := range offsets {
		if offset <= 0 || mode == "ledger" {
			continue
		}
		view.conversationMode = mode
		view.focus = workspaceCompose
		// Ledger tab geometry does not describe the hidden conversation viewport.
		if mode == active && focus != workspaceLedgerPane {
			view.focus = focus
		}
		if view.reviewVisibleLocked() {
			continue
		}
		left, body, _, _ := view.paneGeometryLocked()
		lines, _, _, _ := view.contentLocked(max(1, left-4))
		if mode == "research" {
			lines = view.researchContentLocked(max(1, left-4))
		}
		screen := view.scrollScreenLocked(mode)
		row, column := workspaceRowAnchor(&screen, max(1, left-4), max(0, len(lines)-body-offset))
		anchors = append(anchors, workspaceResizeAnchor{mode, row, column})
	}
	return anchors
}

func (view *chatWorkspace) restoreResizeAnchorsLocked(anchors []workspaceResizeAnchor) {
	active, focus := view.conversationMode, view.focus
	defer func() { view.conversationMode, view.focus = active, focus }()
	for _, anchor := range anchors {
		view.conversationMode, view.focus = anchor.mode, workspaceCompose
		if anchor.mode == active && focus != workspaceLedgerPane {
			view.focus = focus
		}
		left, body, _, _ := view.paneGeometryLocked()
		lines, _, _, _ := view.contentLocked(max(1, left-4))
		if anchor.mode == "research" {
			lines = view.researchContentLocked(max(1, left-4))
		}
		screen := view.scrollScreenLocked(anchor.mode)
		_, top, _ := workspaceWrappedRows(screen.lines, screen.soft, max(1, left-4), anchor.row, anchor.column)
		offset := max(0, len(lines)-body-top)
		view.modeOffsets[anchor.mode] = offset
		if anchor.mode == active {
			view.conversationOffset = offset
		}
	}
}
