package main

import (
	"bytes"
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
	var raw [48]byte
	count, err := source.source.ReadInputLocked(raw[:min(len(raw), max(1, len(destination)-8))])
	if count == 0 {
		if err == errPlanReviewInputSourceIdle && bytes.Equal(source.candidate, []byte{27}) {
			source.candidate = nil
			if keyErr := source.view.key(27, false); keyErr != nil {
				return 0, keyErr
			}
		}
		return 0, err
	}
	if source.review != nil && source.review() {
		source.candidate = nil
		return copy(destination, raw[:count]), err
	}
	for _, value := range raw[:count] {
		if keyErr := source.consume(value); keyErr != nil {
			return 0, keyErr
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
	if len(source.candidate) > 0 || value == '\x1b' {
		source.candidate = append(source.candidate, value)
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
				if !source.paste && (bytes.Equal(sequence, []byte("\x1b[A")) || bytes.Equal(sequence, []byte("\x1b[B"))) {
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
				if !source.paste && (bytes.Equal(sequence, []byte("\x1b[5~")) || bytes.Equal(sequence, []byte("\x1b[6~"))) {
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
	if key == 2 {
		view.prefix = !view.prefix
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
		view.prefix = false
		switch key {
		case '1':
			view.focus = workspaceCompose
			view.conversationOffset = 0
		case '2':
			view.focus = workspaceConversation
		case '3':
			view.focus = workspaceActivity
		case '4':
			view.focus = workspaceLedgerPane
		case 'o':
			view.focus = (view.focus + 1) % 4
		case 'z':
			view.zoom = !view.zoom
		case '?':
			view.help = !view.help
		}
		if view.focus == workspaceCompose {
			view.conversationOffset = 0
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
	if view.focus == workspaceLedgerPane && key != 'i' && key != 27 && key != 'g' && key != 'G' {
		return view.ledgerKeyLocked(key)
	}
	switch key {
	case 'i', 27:
		view.focus = workspaceCompose
		view.conversationOffset = 0
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
	view.conversationOffset = max(0, min(view.conversationOffset+delta, max(0, len(lines)-max(1, bodyHeight))))
}
