package main

import (
	"bytes"
	"errors"
	"io"
)

// Only explicit terminal key sequences are presentation commands. Bracketed
// paste and plan-review input pass unchanged to their existing authority owner.
type workspaceInputSource struct {
	source            planReviewInputSource
	view              *chatWorkspace
	review            func() bool
	candidate, queued []byte
	paste             bool
	rawQueued         []byte
	pendingSwitch     error
}

func (source *workspaceInputSource) LockInput()   { source.source.LockInput() }
func (source *workspaceInputSource) UnlockInput() { source.source.UnlockInput() }
func (source *workspaceInputSource) Read(destination []byte) (int, error) {
	source.LockInput()
	defer source.UnlockInput()
	return source.ReadInputLocked(destination)
}

func (source *workspaceInputSource) ReadInputLocked(destination []byte) (int, error) {
	if len(destination) == 0 {
		return 0, nil
	}
	if len(source.queued) > 0 {
		count := copy(destination, source.queued)
		source.queued = source.queued[count:]
		return count, nil
	}
	if source.pendingSwitch != nil {
		err := source.pendingSwitch
		source.pendingSwitch = nil
		return 0, err
	}
	var raw [48]byte
	values := source.rawQueued
	source.rawQueued = nil
	count, err := len(values), error(nil)
	if count == 0 {
		count, err = source.source.ReadInputLocked(raw[:min(len(raw), max(1, len(destination)-8))])
		values = raw[:count]
	}
	if count == 0 {
		if err == errPlanReviewInputSourceIdle && bytes.Equal(source.candidate, []byte{27}) {
			source.candidate = nil
			if source.reviewing() {
				destination[0] = 27
				return 1, nil
			}
			if keyErr := source.view.key(27, false); keyErr != nil {
				return 0, keyErr
			}
		}
		return 0, err
	}
	for index, value := range values {
		if keyErr := source.consume(value); keyErr != nil {
			var switched workspaceModeSwitch
			if !errors.As(keyErr, &switched) {
				return 0, keyErr
			}
			source.rawQueued = append([]byte(nil), values[index+1:]...)
			if len(source.queued) == 0 {
				return 0, keyErr
			}
			source.pendingSwitch = keyErr
			break
		}
	}
	count = copy(destination, source.queued)
	source.queued = source.queued[count:]
	if count == 0 && err == nil {
		err = errPlanReviewInputSourceIdle
	}
	if err == io.EOF && len(source.candidate) > 0 {
		source.queued = append(source.queued, source.candidate...)
		source.candidate = nil
	}
	return count, err
}

func (source *workspaceInputSource) consume(value byte) error {
	source.view.mu.Lock()
	editor := source.view.focus == workspaceLedgerPane && source.view.ledger.editor != nil && !source.view.prefix
	source.view.mu.Unlock()
	if editor && value != 2 {
		return source.consumeLedgerEditor(value)
	}
	if len(source.candidate) > 0 || value == '\x1b' {
		source.candidate = append(source.candidate, value)
		if bytes.HasPrefix(source.candidate, []byte("\x1b[<")) {
			if value == 'M' || value == 'm' {
				sequence := string(source.candidate)
				source.candidate = nil
				if value == 'M' {
					return source.view.ledgerMouse(sequence)
				}
			}
			return nil
		}
		for _, sequence := range [][]byte{bracketedPasteStart, bracketedPasteEnd, []byte("\x1b[5~"), []byte("\x1b[6~"), []byte("\x1b[A"), []byte("\x1b[B")} {
			if bytes.Equal(source.candidate, sequence) {
				if bytes.Equal(sequence, bracketedPasteStart) {
					source.paste = true
					source.view.mu.Lock()
					source.view.focus, source.view.prefix, source.view.help = workspaceCompose, false, false
					source.view.conversationOffset = 0
					source.view.mu.Unlock()
				}
				if bytes.Equal(sequence, bracketedPasteEnd) {
					source.paste = false
				}
				if !source.paste && !source.reviewing() && (bytes.Equal(sequence, []byte("\x1b[A")) || bytes.Equal(sequence, []byte("\x1b[B"))) {
					source.view.mu.Lock()
					focus := source.view.focus
					source.view.mu.Unlock()
					if focus != workspaceCompose {
						key := byte('k')
						if sequence[2] == 'B' {
							key = 'j'
						}
						source.candidate = nil
						return source.view.key(key, false)
					}
				}
				if !source.paste && !source.reviewing() && (bytes.Equal(sequence, []byte("\x1b[5~")) || bytes.Equal(sequence, []byte("\x1b[6~"))) {
					key := byte('u')
					if sequence[2] == '6' {
						key = 'd'
					}
					source.candidate = nil
					return source.view.key(key, true)
				}
				source.queued = append(source.queued, source.candidate...)
				source.candidate = nil
				return nil
			}
			if bytes.HasPrefix(sequence, source.candidate) {
				return nil
			}
		}
		source.queued = append(source.queued, source.candidate...)
		source.candidate = nil
		return nil
	}
	if source.paste {
		source.queued = append(source.queued, value)
		return nil
	}
	if source.reviewing() {
		source.view.mu.Lock()
		prefix := source.view.prefix
		source.view.mu.Unlock()
		if value != 2 && !prefix {
			source.queued = append(source.queued, value)
			return nil
		}
	}
	if value == 12 {
		source.view.mu.Lock()
		defer source.view.mu.Unlock()
		source.view.lastFrame = nil
		return source.view.drawLocked()
	}
	source.view.mu.Lock()
	prefix, focus, help := source.view.prefix, source.view.focus, source.view.help
	source.view.mu.Unlock()
	if value == 2 && prefix {
		source.queued = append(source.queued, value)
		source.view.mu.Lock()
		defer source.view.mu.Unlock()
		source.view.prefix = false
		return source.view.drawLocked()
	}
	if value == 2 || prefix || focus != workspaceCompose || help {
		return source.view.key(value, false)
	}
	source.queued = append(source.queued, value)
	return nil
}

func (view *chatWorkspace) key(key byte, page bool) error {
	view.mu.Lock()
	defer view.mu.Unlock()
	if view.focus == workspaceLedgerPane && view.ledger.editor != nil && !view.prefix && key != 2 {
		return view.ledgerEditorKeyLocked(key)
	}
	if key == 2 {
		view.prefix = !view.prefix
		return view.drawLocked()
	}
	if view.focus == workspaceSessionsPane && !view.prefix {
		return view.sessionsKeyLocked(key)
	}
	if view.help && !view.prefix {
		delta := 1
		if page {
			delta = max(1, view.height-4)
		}
		switch key {
		case 'j', 'd':
			view.helpOffset += delta
		case 'k', 'u':
			view.helpOffset = max(0, view.helpOffset-delta)
		default:
			view.help = false
		}
		return view.drawLocked()
	}
	if page {
		if view.focus == workspaceCompose {
			view.focus = workspaceConversation
		}
		delta := max(1, view.height-6)
		if key == 'd' {
			delta = -delta
		}
		view.scrollLocked(delta)
		return view.drawLocked()
	}
	if view.prefix {
		previousFocus := view.focus
		view.prefix = false
		if key != '?' {
			view.help = false
		}
		switch key {
		case '1':
			view.focus = workspaceCompose
		case '2':
			view.focus = workspaceConversation
		case '3':
			view.focus = workspaceActivity
		case '5':
			return view.openSessionsLocked(view.focus)
		case '4':
			view.focus = workspaceLedgerPane
		case 'o', '\t', ']':
			view.focus = (view.focus + 1) % 5
		case '[':
			view.focus = (view.focus + 4) % 5
		case 'f', 'e', 'l', 'r':
			mode := map[byte]string{'f': "infer", 'e': "interact", 'l': "ledger", 'r': "research"}[key]
			if view.editorSwitching {
				return workspaceModeSwitch{mode}
			}
			select {
			case view.modeRequests <- mode:
			default:
				select {
				case <-view.modeRequests:
				default:
				}
				view.modeRequests <- mode
			}

		case 'z':
			view.zoom = !view.zoom
		case '?':
			view.help = !view.help
		}

		if view.focus == workspaceSessionsPane {
			return view.openSessionsLocked(previousFocus)
		}
		if view.focus == workspaceLedgerPane && !view.ledger.opened {
			view.ledger.opened = true
			return view.ledgerRequestLocked(workspaceLedgerRequest{key: 'r'})
		}
		return view.drawLocked()
	}
	if view.help {
		view.help = false
		return view.drawLocked()
	}
	if view.focus == workspaceLedgerPane && key == 27 && view.ledger.details {
		return view.ledgerKeyLocked('b')
	}
	if view.focus == workspaceLedgerPane && key != 'i' && key != 27 && key != 'g' && key != 'G' {
		return view.ledgerKeyLocked(key)
	}
	switch key {
	case 'i', 27:
		view.focus = workspaceCompose
	case 'j':
		view.scrollLocked(-1)
	case 'k':
		view.scrollLocked(1)
	case 'g':
		if view.focus == workspaceLedgerPane {
			view.ledger.offset = 0
		} else {
			view.scrollLocked(workspaceScrollbackLines)
		}
	case 'G':
		if view.focus == workspaceLedgerPane {
			view.scrollLocked(-workspaceScrollbackLines)
		} else {
			view.conversationOffset, view.activityOffset = 0, 0
		}
	}
	return view.drawLocked()
}

func (view *chatWorkspace) scrollLocked(delta int) {
	if view.focus == workspaceLedgerPane {
		width, height, _, _ := view.paneGeometryLocked()
		view.ledger.offset = max(0, min(view.ledger.offset-delta, max(0, len(view.ledgerLinesLocked(max(1, width-4)))-height)))
		return
	}
	if view.focus == workspaceActivity {
		view.activityOffset = max(0, min(view.activityOffset+delta, len(view.activityLinesLocked(31))-1))
		return
	}
	leftWidth, bodyHeight, _, _ := view.paneGeometryLocked()
	lines, _, _, _ := view.contentLocked(max(1, leftWidth-4))
	if view.conversationMode == "research" {
		lines = view.researchContentLocked(max(1, leftWidth-4))
	}
	view.conversationOffset = max(0, min(view.conversationOffset+delta, max(0, len(lines)-max(1, bodyHeight))))
}

func (source *workspaceInputSource) reviewing() bool {
	source.view.mu.Lock()
	sessions := source.view.focus == workspaceSessionsPane || source.view.focus == workspaceLedgerPane
	source.view.mu.Unlock()
	return !sessions && source.review != nil && source.review()
}
