package main

import (
	"context"
	"unicode"
	"unicode/utf8"
)

// Probe the line editor on its own input goroutine: it owns the current text,
// cursor, history, and paste projection. No separate draft copy is maintained.
func (console *chatConsole) cancelDraft(ctx context.Context) (bool, error) {
	console.presentationMu.Lock()
	if !console.readingDraft || console.reviewInput.ReviewEnabled() {
		console.presentationMu.Unlock()
		return false, nil
	}
	result := make(chan bool, 1)
	console.draftInterrupt = result
	console.presentationMu.Unlock()
	router := console.reviewInput
	router.source.LockInput()
	router.mu.Lock()
	// Close an unfinished paste before the private probe reaches the editor.
	if source, ok := router.source.(*workspaceInputSource); ok {
		source.paste = false
		source.candidate = nil
	}
	router.appendLineBytesLocked(append(append([]byte{}, bracketedPasteEnd...), 0), router.currentLineAuthorityLocked())
	router.signalReadLocked()
	router.mu.Unlock()
	router.source.UnlockInput()
	select {
	case cleared := <-result:
		return cleared, nil
	case <-ctx.Done():
		console.presentationMu.Lock()
		if console.draftInterrupt == result {
			console.draftInterrupt = nil
		}
		console.presentationMu.Unlock()
		return false, ctx.Err()
	}
}

func (console *chatConsole) interruptDraftKey(line string, pos int, key rune) (string, int, bool) {
	if key != 0 {
		// x/term silently drops typed characters at exactly 4096 runes.
		// Its public edit callback can insert the character without losing the
		// retained editor, cursor, history, or any part of the user's draft.
		if unicode.IsPrint(key) && utf8.RuneCountInString(line) == 4096 {
			return line[:pos] + string(key) + line[pos:], pos + utf8.RuneLen(key), true
		}
		return "", 0, false
	}
	console.presentationMu.Lock()
	defer console.presentationMu.Unlock()
	if console.draftInterrupt == nil {
		return "", 0, false
	}
	cleared := len(line) > 0
	if cleared {
		console.pasteReader.consumeLineState()
	}
	console.finishDraftInterruptLocked(cleared)
	return "", 0, cleared
}

func (console *chatConsole) finishDraftInterruptLocked(cleared bool) {
	if console.draftInterrupt != nil {
		console.draftInterrupt <- cleared
		console.draftInterrupt = nil
	}
}
