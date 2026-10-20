package main

import (
	"context"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/projectroot"
	"github.com/gryph/omnidex/internal/sessionlist"
)

func sessionEditorPair(t *testing.T) (*sessionTerminal, *chatConsole, *chatConsole, *os.File) {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	input, keys, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cancel(); input.Close(); keys.Close() })
	terminal := &sessionTerminal{selected: "one", source: newTerminalContextReader(ctx, input), output: io.Discard}
	first, err := newRetainedSessionConsole(ctx, terminal, "one", 70, 20)
	if err != nil {
		t.Fatal(err)
	}
	second, err := newRetainedSessionConsole(ctx, terminal, "two", 70, 20)
	if err != nil {
		t.Fatal(err)
	}
	return terminal, first, second, keys
}

func sessionRead(t *testing.T, console *chatConsole) <-chan string {
	t.Helper()
	done := make(chan string, 1)
	go func() {
		line, _, _, err := console.ReadLine()
		if err != nil {
			line = "ERROR: " + err.Error()
		}
		done <- line
	}()
	return done
}
func sessionLine(t *testing.T, done <-chan string, expected string) {
	t.Helper()
	select {
	case line := <-done:
		if line != expected {
			t.Fatalf("line %q, want %q", line, expected)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("session input stalled")
	}
}

func TestSessionsRetainDraftCursorHistoryModeScrollAndLedger(t *testing.T) {
	terminal, first, second, keys := sessionEditorPair(t)
	a, b := sessionRead(t, first), sessionRead(t, second)
	modeKeys(t, keys, "界ab\x1b[D\x02eother")
	waitMode(t, first, "interact", "other")
	if err := first.WriteOutput(strings.Repeat("scrollable session line\n", 50)); err != nil {
		t.Fatal(err)
	}
	first.workspace.mu.Lock()
	first.workspace.activityOffset = 3
	first.workspace.conversationOffset = 4
	first.workspace.focus = workspaceActivity
	first.workspace.mu.Unlock()
	if err := first.workspace.acceptLedger(panelLedgerFixture()); err != nil {
		t.Fatal(err)
	}
	first.workspace.mu.Lock()
	first.workspace.ledger.selected = "panel-second"
	first.workspace.mu.Unlock()
	if err := terminal.selectConsole("two", second); err != nil {
		t.Fatal(err)
	}
	modeKeys(t, keys, "second\x1b[D")
	waitMode(t, second, "infer", "second")
	// A delayed response updates only its origin while that session is hidden.
	if err := first.WriteOutput("ORIGIN ONE delayed response\n"); err != nil {
		t.Fatal(err)
	}
	frame, _, _, _ := workspaceFrame(second.workspace)
	if strings.Contains(strings.Join(frame, "\n"), "ORIGIN ONE") {
		t.Fatal("response crossed sessions")
	}
	if err := terminal.selectConsole("one", first); err != nil {
		t.Fatal(err)
	}
	first.workspace.mu.Lock()
	if first.workspace.conversationMode != "interact" || first.workspace.activityOffset != 3 || first.workspace.conversationOffset < 4 || first.workspace.focus != workspaceActivity || first.workspace.ledger.selected != "panel-second" {
		t.Fatal("session selection lost view state")
	}
	first.workspace.focus = workspaceCompose
	first.workspace.conversationOffset = 0
	first.workspace.mu.Unlock()
	modeKeys(t, keys, "\x02fX\r")
	sessionLine(t, a, "界aXb")
	a = sessionRead(t, first)
	modeKeys(t, keys, "\x02e\r")
	sessionLine(t, a, "other")
	a = sessionRead(t, first)
	modeKeys(t, keys, "\x1b[A\r")
	sessionLine(t, a, "other")
	if err := terminal.selectConsole("two", second); err != nil {
		t.Fatal(err)
	}
	modeKeys(t, keys, "!\r")
	sessionLine(t, b, "secon!d")
}

func TestSessionsWaitingReviewAndQuestionAuthoritySurviveSelection(t *testing.T) {
	terminal, first, second, keys := sessionEditorPair(t)
	if err := first.ShowPlanReview("Pending question ONE\n> original selected task"); err != nil {
		t.Fatal(err)
	}
	authority := first.CurrentInputAuthority()
	sessionRead(t, first)
	b := sessionRead(t, second)
	if err := terminal.selectConsole("two", second); err != nil {
		t.Fatal(err)
	}
	modeKeys(t, keys, "ordinary second\r")
	sessionLine(t, b, "ordinary second")
	if first.CurrentInputAuthority() != authority || !first.reviewInput.ReviewEnabled() {
		t.Fatal("waiting authority changed")
	}
	if err := terminal.selectConsole("one", first); err != nil {
		t.Fatal(err)
	}
	frame, _, _, _ := workspaceFrame(first.workspace)
	if !strings.Contains(strings.Join(frame, "\n"), "Pending question ONE") {
		t.Fatal("pending question disappeared")
	}
	modeKeys(t, keys, " ")
	if got := nextPlanReviewKey(t, first.reviewInput); got != planReviewKeyToggle {
		t.Fatalf("review input went elsewhere: %v", got)
	}
}

func TestSessionsPaneKeyboardDuringReviewAndPaste(t *testing.T) {
	view, _ := testWorkspace(t, 36, 12)
	view.sessions.current = "one"
	view.sessions.requests = make(chan workspaceSessionRequest, 1)
	view.sessions.entries = []sessionlist.Entry{{ID: model.ChannelID("one"), Directory: "/work/one", Activity: "running", ObservedAt: time.Now()}, {ID: "two", Directory: "/work/等待", Activity: "waiting_input", ObservedAt: time.Now()}}
	source := &workspaceInputSource{view: view, review: func() bool { return true }}
	for _, value := range []byte("\x025") {
		if err := source.consume(value); err != nil {
			t.Fatal(err)
		}
	}
	<-view.sessions.requests
	for _, value := range []byte("j\r") {
		if err := source.consume(value); err != nil {
			t.Fatal(err)
		}
	}
	request := <-view.sessions.requests
	if request.origin != "one" || request.target.ID != "two" || len(source.queued) != 0 {
		t.Fatal("session selection leaked into approval input", request)
	}
	frame, _, _, visible := workspaceFrame(view)
	if visible || !strings.Contains(strings.Join(frame, "\n"), "waiting_input") {
		t.Fatal("narrow sessions pane missing waiting state")
	}
	for _, value := range append(append([]byte{}, bracketedPasteStart...), []byte("\x025")...) {
		if err := source.consume(value); err != nil {
			t.Fatal(err)
		}
	}
	if view.focus == workspaceSessionsPane {
		t.Fatal("paste invoked session navigation")
	}
}

func TestSessionsSelectionRejectsReplacedDirectoryBeforeOpeningConnection(t *testing.T) {
	owner := strings.Repeat("1", 32)
	current, other := t.TempDir(), t.TempDir()
	physical, err := projectroot.DirectoryIdentity(other)
	if err != nil {
		t.Fatal(err)
	}
	stale, err := projectroot.ClientWorkspaceIdentity(owner, physical)
	if err != nil {
		t.Fatal(err)
	}
	host := &chatSessions{ctx: t.Context(), clientID: owner, sessions: make(map[model.ChannelID]*retainedChatSession), selected: "origin"}
	target := sessionlist.Entry{ID: model.ChannelID("cli-chat-" + strings.Repeat("1", 32)), Directory: current, WorkspaceIdentity: stale, ObservedAt: time.Now(), Activity: "idle"}
	err = host.selectSession(&retainedChatSession{}, target)
	if err == nil || !strings.Contains(err.Error(), "replaced") || host.selected != "origin" || len(host.sessions) != 0 {
		t.Fatal("stale binding changed session authority", err)
	}
}
