package main

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/sessionlist"
)

type workspaceSessionRequest struct {
	origin model.ChannelID
	target *sessionlist.Entry
}

type workspaceSessions struct {
	returnFocus workspaceFocus
	entries     []sessionlist.Entry
	selected    int
	current     model.ChannelID
	requests    chan workspaceSessionRequest
	errorText   string
}

func (view *chatWorkspace) sessionsRequestLocked(target *sessionlist.Entry) error {
	if view.sessions.requests == nil {
		return nil
	}
	select {
	case view.sessions.requests <- workspaceSessionRequest{view.sessions.current, target}:
	default:
		view.sessions.errorText = "Session selection is pending"
	}
	return view.drawLocked()
}

func (view *chatWorkspace) sessionsKeyLocked(key byte) error {
	switch key {
	case 'j':
		view.sessions.selected = min(len(view.sessions.entries)-1, view.sessions.selected+1)
	case 'k':
		view.sessions.selected = max(0, view.sessions.selected-1)
	case 'r':
		return view.sessionsRequestLocked(nil)
	case '\r', '\n':
		if view.sessions.selected >= 0 && view.sessions.selected < len(view.sessions.entries) {
			entry := view.sessions.entries[view.sessions.selected]
			return view.sessionsRequestLocked(&entry)
		}
	case 'i', 27:
		view.focus = workspaceCompose
	}
	return view.drawLocked()
}

func (view *chatWorkspace) sessionsLinesLocked(width int) ([]string, int) {
	lines := []string{"Sessions", ""}
	selectedRow := 0
	if view.sessions.errorText != "" {
		lines = append(lines, strings.Split(ansi.Hardwrap(view.sessions.errorText, max(1, width), true), "\n")...)
		lines = append(lines, "")
	}
	if len(view.sessions.entries) == 0 {
		lines = append(lines, "No sessions observed. Press r to refresh.")
	}
	for index, entry := range view.sessions.entries {
		marker := "  "
		if index == view.sessions.selected {
			selectedRow = len(lines)
			marker = "> "
		}
		active := ""
		if entry.ID == view.sessions.current {
			active = " (selected)"
		}
		text := marker + entry.Directory + active + "\n  " + entry.Activity + " · observed " + entry.ObservedAt.Local().Format("15:04:05") + "\n  " + string(entry.ID)
		for _, line := range strings.Split(safeConsoleText(text), "\n") {
			lines = append(lines, strings.Split(ansi.Hardwrap(line, max(1, width), true), "\n")...)
		}
		lines = append(lines, "")
	}
	return lines, selectedRow
}

func (view *chatWorkspace) openSessionsLocked(previous workspaceFocus) error {
	if previous != workspaceSessionsPane {
		view.sessions.returnFocus = previous
	}
	view.focus = workspaceSessionsPane
	return view.sessionsRequestLocked(nil)
}
