package main

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/sessionlist"
)

func TestSessionsPTY(t *testing.T) {
	if os.Getenv("OMNI_TEST_SESSIONS_PTY") != "1" {
		t.Skip("requires an isolated interactive PTY")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	first, err := newChatConsole(ctx, os.Stdin, os.Stdout, os.Stderr)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	source := first.reviewInput.source.(*workspaceInputSource)
	terminal := &sessionTerminal{source: source.source, output: first.workspace.output, selected: "one"}
	source.source = &sessionTerminalInput{terminal: terminal, id: "one", ctx: ctx}
	first.workspace.output = sessionTerminalOutput{terminal, "one"}
	second, err := newRetainedSessionConsole(ctx, terminal, "two", 36, 12)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	requests := make(chan workspaceSessionRequest, 1)
	first.workspace.mu.Lock()
	first.workspace.sessions = workspaceSessions{current: "one", requests: requests, entries: []sessionlist.Entry{{ID: "one", Directory: "/fixture/one", Activity: "running", ObservedAt: time.Now()}, {ID: "two", Directory: "/fixture/two", Activity: "waiting_input", ObservedAt: time.Now()}}}
	first.workspace.mu.Unlock()
	a, b := sessionRead(t, first), sessionRead(t, second)
	fmt.Fprintln(os.Stderr, "SESSIONS_PTY_ONE")
	// Ctrl-B 5 first requests listing refresh, then j/Enter selects session two.
	select {
	case request := <-requests:
		if request.target != nil {
			t.Fatal(request)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	fmt.Fprintln(os.Stderr, "SESSIONS_PTY_PICK")
	select {
	case request := <-requests:
		if request.origin != "one" || request.target == nil || request.target.ID != model.ChannelID("two") {
			t.Fatal(request)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	first.workspace.mu.Lock()
	first.workspace.focus = workspaceCompose
	first.workspace.mu.Unlock()
	if err := terminal.selectConsole("two", second); err != nil {
		t.Fatal(err)
	}
	fmt.Fprintln(os.Stderr, "SESSIONS_PTY_TWO")
	sessionLine(t, b, "second")
	if err := terminal.selectConsole("one", first); err != nil {
		t.Fatal(err)
	}
	fmt.Fprintln(os.Stderr, "SESSIONS_PTY_RETURN")
	sessionLine(t, a, "界aXb")
	fmt.Fprintln(os.Stderr, "SESSIONS_PTY_VERIFIED")
}
