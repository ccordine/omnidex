package main

import (
	"context"
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/model"
)

func TestCtrlCClearsDraftBeforeInterruptingSelectedSession(t *testing.T) {
	for _, draft := range []string{"discard this", "discard this\x1b[", "界ab\x1b[D", "   ", "\x1b[200~first\nsecond\x1b[201~", "\x1b[200~unfinished paste"} {
		t.Run(draft, func(t *testing.T) {
			terminal, first, second, keys := sessionEditorPair(t)
			signals := make(chan os.Signal, 2)
			other := make(chan os.Signal, 2)
			host := &chatSessions{ctx: t.Context(), terminal: terminal, selected: "one", sessions: map[model.ChannelID]*retainedChatSession{
				"one": {config: chatSessionConfig{Console: first}, signals: signals},
				"two": {config: chatSessionConfig{Console: second}, signals: other},
			}}
			line := sessionRead(t, first)
			sessionRead(t, second)
			modeKeys(t, keys, draft)
			first.reviewInput.source.LockInput()
			first.reviewInput.source.UnlockInput()
			// A visible prefix proves the actual editor owns text before the signal.
			expected := "discard this"
			switch draft {
			case "界ab\x1b[D":
				expected = "界ab"
			case "   ":
				expected = "you> "
			case "\x1b[200~first\nsecond\x1b[201~":
				expected = "first"
			case "\x1b[200~unfinished paste":
				expected = "unfinished paste"
			}
			waitMode(t, first, "infer", expected)
			if draft == "   " {
				deadline := time.Now().Add(3 * time.Second)
				for {
					first.workspace.mu.Lock()
					ready := first.workspace.conversation.column >= first.workspace.draftColumn+len("you> ")+3
					first.workspace.mu.Unlock()
					if ready {
						break
					}
					if time.Now().After(deadline) {
						t.Fatal("whitespace draft was not displayed")
					}
					time.Sleep(time.Millisecond)
				}
			}
			if err := host.signalSelected(os.Interrupt); err != nil {
				t.Fatal(err)
			}
			select {
			case s := <-signals:
				t.Fatalf("draft cancellation reached job controls: %v", s)
			default:
			}
			select {
			case <-line:
				t.Fatal("draft cancellation submitted or ended input")
			default:
			}
			if err := host.signalSelected(os.Interrupt); err != nil {
				t.Fatal(err)
			}
			select {
			case s := <-signals:
				if s != os.Interrupt {
					t.Fatalf("empty prompt signal=%v", s)
				}
			default:
				t.Fatal("empty prompt did not use existing cancellation")
			}
			select {
			case <-other:
				t.Fatal("interrupt reached a background session")
			default:
			}
			modeKeys(t, keys, "\x1b[Afresh\r")
			sessionLine(t, line, "fresh")
		})
	}
}

func TestDraftCancellationPreservesOtherModesHistoryAndTermination(t *testing.T) {
	console, keys := modeEditorConsole(t)
	previous := sessionRead(t, console)
	modeKeys(t, keys, "previous\r")
	sessionLine(t, previous, "previous")
	line := sessionRead(t, console)
	modeKeys(t, keys, "keep\x02ediscard")
	waitMode(t, console, "interact", "discard")
	if cleared, err := console.cancelDraft(t.Context()); err != nil || !cleared {
		t.Fatalf("clear draft=%t error=%v", cleared, err)
	}
	modeKeys(t, keys, "\x02f\r")
	sessionLine(t, line, "keep")
	line = sessionRead(t, console)
	modeKeys(t, keys, "\x1b[A\r")
	sessionLine(t, line, "keep")
	signals := make(chan os.Signal, 1)
	host := &chatSessions{ctx: context.Background(), selected: "one", sessions: map[model.ChannelID]*retainedChatSession{"one": {config: chatSessionConfig{Console: console}, signals: signals}}}
	if err := host.signalSelected(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	if signal := <-signals; signal != syscall.SIGTERM {
		t.Fatalf("termination signal=%v", signal)
	}
	if cleared, err := console.cancelDraft(t.Context()); err != nil || cleared {
		t.Fatalf("inactive prompt clear=%t error=%v", cleared, err)
	}
}
