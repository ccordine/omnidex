package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/gryph/omnidex/internal/sessionintent"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/model"
	"golang.org/x/term"
)

func modeEditorConsole(t *testing.T, parents ...context.Context) (*chatConsole, *os.File) {
	t.Helper()
	parent := t.Context()
	if len(parents) > 0 {
		parent = parents[0]
	}
	ctx, cancel := context.WithCancel(parent)
	input, keys, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cancel(); input.Close(); keys.Close() })
	view, _ := testWorkspace(t, 70, 20)
	view.editorSwitching = true
	source := &workspaceInputSource{source: newTerminalContextReader(ctx, input), view: view}
	router, err := newPlanReviewInputRouter(source)
	if err != nil {
		t.Fatal(err)
	}
	source.review = router.ReviewEnabled
	paste := newBracketedPasteReader(router, model.MaxFreeFormTurnBytes)
	console := &chatConsole{workspace: view, output: view, reviewInput: router, pasteReader: paste, terminalFD: -1, editorMode: "infer", prompt: "infer> "}
	console.terminal = term.NewTerminal(terminalReadWriter{reader: paste, writer: view}, "infer> ")
	console.terminal.AutoCompleteCallback = console.boundTerminalInput
	console.terminal.SetBracketedPasteMode(true)
	columns, rows, _ := view.Resize(70, 20)
	console.terminalWidth, console.terminalHeight = columns, rows
	if err := console.terminal.SetSize(columns, rows); err != nil {
		t.Fatal(err)
	}
	return console, keys
}

func modeKeys(t *testing.T, keys *os.File, text string) {
	t.Helper()
	if _, err := keys.WriteString(text); err != nil {
		t.Fatal(err)
	}
}

func waitMode(t *testing.T, console *chatConsole, mode, text string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		console.workspace.mu.Lock()
		frame, _, _, _ := console.workspace.frameLocked()
		ready := console.workspace.conversationMode == mode && strings.Contains(strings.Join(frame, "\n"), text)
		console.workspace.mu.Unlock()
		if ready {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("mode %s did not display %q", mode, text)
}

func TestWorkspaceModeEditorsPreserveIndependentDraftCursorHistoryAndPaste(t *testing.T) {
	console, keys := modeEditorConsole(t)
	done := make(chan string, 1)
	go func() {
		line, _, _, err := console.ReadLine()
		if err != nil {
			done <- "error: " + err.Error()
		} else {
			done <- line
		}
	}()
	modeKeys(t, keys, "界ab\x1b[D\x02eother\x02fX")
	waitMode(t, console, "infer", "界aXb")
	if err := console.WriteOutput("Pending reply arrived while editing\n"); err != nil {
		t.Fatal(err)
	}
	console.presentationMu.Lock()
	columns, rows, resizeErr := console.workspace.Resize(36, 12)
	if resizeErr == nil {
		resizeErr = console.terminal.SetSize(columns, rows)
	}
	console.terminalWidth, console.terminalHeight = columns, rows
	console.presentationMu.Unlock()
	if resizeErr != nil {
		t.Fatal(resizeErr)
	}
	modeKeys(t, keys, "\x02e\x1b[D!\r")
	select {
	case got := <-done:
		if got != "othe!r" {
			t.Fatalf("interact draft/cursor = %q", got)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("mode editor did not submit")
	}
	go func() {
		line, _, _, err := console.ReadLine()
		if err != nil {
			done <- err.Error()
		} else {
			done <- line
		}
	}()
	modeKeys(t, keys, "\x02f\r")
	select {
	case got := <-done:
		if got != "界aXb" {
			t.Fatalf("infer draft = %q", got)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("infer editor did not submit")
	}
	go func() {
		line, _, _, err := console.ReadLine()
		if err != nil {
			done <- err.Error()
		} else {
			done <- line
		}
	}()
	modeKeys(t, keys, "\x02e\x1b[A\r")
	select {
	case got := <-done:
		if got != "othe!r" {
			t.Fatalf("per-mode history = %q", got)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("history did not submit")
	}
	go func() {
		line, _, _, err := console.ReadLine()
		if err != nil {
			done <- err.Error()
		} else {
			done <- line
		}
	}()
	modeKeys(t, keys, "\x1b[200~line 界\n  text\x1b[201~\x02f\x02e\r")
	select {
	case got := <-done:
		if got != "line 界\n  text" {
			t.Fatalf("retained paste = %q", got)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("paste did not submit")
	}
	console.workspace.mu.Lock()
	frame, _, _, _ := console.workspace.frameLocked()
	console.workspace.mu.Unlock()
	if !strings.Contains(strings.Join(frame, "\n"), "Pending reply arrived") {
		t.Fatal("switch lost streamed reply")
	}
}

func TestWorkspaceModeSwitchPreservesPendingReviewInputAuthority(t *testing.T) {
	console, keys := modeEditorConsole(t)
	authority, err := console.BeginPlanReviewNote()
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan chatInput, 1)
	go func() {
		line, pasted, auth, err := console.ReadLine()
		done <- chatInput{Text: line, Pasted: pasted, Authority: auth, Err: err}
	}()
	modeKeys(t, keys, "draft approval\x1b[D\x02rquestion\r")
	select {
	case got := <-done:
		if got.Err != nil || got.Text != "question" || got.Authority.Mode != terminalInputOrdinary {
			t.Fatalf("research input = %+v", got)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("research unavailable during pending planning reply")
	}
	if current := console.CurrentInputAuthority(); current != authority {
		t.Fatalf("switch changed planning authority: %+v -> %+v", authority, current)
	}
	go func() {
		line, pasted, auth, err := console.ReadLine()
		done <- chatInput{Text: line, Pasted: pasted, Authority: auth, Err: err}
	}()
	modeKeys(t, keys, "\x02f!\r")
	select {
	case got := <-done:
		if got.Err != nil || got.Text != "draft approva!l" || got.Authority != authority {
			t.Fatalf("retained planning reply = %+v", got)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("planning reply was lost")
	}
	if err := console.EndPlanReviewNote(authority); err != nil {
		t.Fatal(err)
	}
}

func TestWorkspaceModeSwitchDuringInFlightResearchLeavesRequestUntouched(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	posts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			json.NewEncoder(w).Encode([]sessionintent.ResearchTurn{})
			return
		}
		posts++
		close(entered)
		<-release
		json.NewEncoder(w).Encode(sessionintent.ResearchTurn{OperationID: "lifecycle_operation_pending", Question: "pending", Answer: "response"})
	}))
	defer server.Close()
	released := false
	defer func() {
		if !released {
			close(release)
		}
	}()
	console, keys := modeEditorConsole(t)
	session := newChatOperationTestSession(t, server.URL)
	session.renderer = chatRenderer{console: console}
	pending := &sessionintent.ResearchRequest{Text: "pending", OperationID: "lifecycle_operation_pending"}
	session.pendingResearch = pending
	state := mustPlanReviewState(t, eligiblePlanReviewFixture(t))
	session.planReview = &state
	active := &model.JobDetails{Job: model.Job{ID: 41, Status: model.JobStatusRunning, CurrentGeneration: 7}}
	session.active = active
	result := make(chan error, 1)
	go func() { result <- session.acceptResearchQuestion("pending") }()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("request did not start")
	}
	lines := make(chan string, 1)
	go func() {
		line, _, _, err := console.ReadLine()
		if err != nil {
			lines <- err.Error()
		} else {
			lines <- line
		}
	}()
	modeKeys(t, keys, "saved\x02eother\x02fX")
	waitMode(t, console, "infer", "savedX")
	if session.pendingResearch != pending || session.planReview != &state || session.active != active || active.Job.CurrentGeneration != 7 || posts != 1 {
		t.Fatal("presentation switch changed pending work")
	}
	modeKeys(t, keys, "\r")
	select {
	case got := <-lines:
		if got != "savedX" {
			t.Fatal(got)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("input unavailable during request")
	}
	close(release)
	released = true
	select {
	case err := <-result:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("switch interrupted research response")
	}
	if posts != 1 || session.active != active || session.planReview != &state {
		t.Fatal("switch duplicated work or discarded review")
	}
}

func TestWorkspaceModeResizePreservesActiveAndInactiveScrollAnchors(t *testing.T) {
	for _, mode := range []string{"infer", "interact", "research"} {
		for _, inactive := range []bool{false, true} {
			t.Run(mode+"/inactive="+fmt.Sprint(inactive), func(t *testing.T) {
				console, _ := modeEditorConsole(t)
				view := console.workspace
				for i := 0; i < 70; i++ {
					text := fmt.Sprintf("row %02d Unicode 界 %s", i, strings.Repeat("text ", 12))
					view.researchLines = append(view.researchLines, text)
					if _, err := view.Write([]byte(text + "\r\n")); err != nil {
						t.Fatal(err)
					}
				}
				view.beginInput()
				if err := console.selectEditor(mode); err != nil {
					t.Fatal(err)
				}
				view.Resize(110, 28)
				left, body, _, _ := view.paneGeometryLocked()
				lines, _, _, _ := view.contentLocked(max(1, left-4))
				if mode == "research" {
					lines = view.researchContentLocked(max(1, left-4))
				}
				for index, line := range lines {
					if strings.HasPrefix(line, "row 23 ") {
						view.conversationOffset = len(lines) - body - index
						break
					}
				}
				topLine := func() string {
					left, body, _, _ := view.paneGeometryLocked()
					lines, _, _, _ := view.contentLocked(max(1, left-4))
					if mode == "research" {
						lines = view.researchContentLocked(max(1, left-4))
					}
					return lines[max(0, len(lines)-body-view.conversationOffset)]
				}
				before := topLine()
				if inactive {
					other := "research"
					if mode == other {
						other = "infer"
					}
					if err := console.selectEditor(other); err != nil {
						t.Fatal(err)
					}
				}
				for _, size := range [][2]int{{36, 12}, {110, 28}} {
					if _, _, err := view.Resize(size[0], size[1]); err != nil {
						t.Fatal(err)
					}
				}
				if inactive {
					if err := console.selectEditor(mode); err != nil {
						t.Fatal(err)
					}
				}
				if after := topLine(); after != before {
					t.Fatalf("scroll anchor changed: %q -> %q", before, after)
				}
			})
		}
	}
}

func TestWorkspaceModeRepliesWhileLedgerPreserveWrappedViewport(t *testing.T) {
	for _, tab := range []bool{false, true} {
		for _, mode := range []string{"infer", "interact", "research"} {
			t.Run(fmt.Sprintf("%s/tab=%t", mode, tab), func(t *testing.T) {
				console, _ := modeEditorConsole(t)
				view := console.workspace
				session := newChatOperationTestSession(t, "http://unused.invalid")
				session.renderer = chatRenderer{console: console}
				var history strings.Builder
				for i := 0; i < 70; i++ {
					fmt.Fprintf(&history, "row %02d %s\n", i, strings.Repeat("x", 100))
				}
				turns := []sessionintent.ResearchTurn{{Question: "first", Answer: history.String()}}
				if err := session.renderResearch(turns); err != nil {
					t.Fatal(err)
				}
				if _, err := view.Write([]byte(strings.ReplaceAll(history.String(), "\n", "\r\n"))); err != nil {
					t.Fatal(err)
				}
				view.beginInput()
				if err := console.selectEditor(mode); err != nil {
					t.Fatal(err)
				}
				if _, _, err := view.Resize(110, 28); err != nil {
					t.Fatal(err)
				}
				topLine := func() string {
					left, body, _, _ := view.paneGeometryLocked()
					lines, _, _, _ := view.contentLocked(max(1, left-4))
					if mode == "research" {
						lines = view.researchContentLocked(max(1, left-4))
					}
					if view.conversationOffset == 0 {
						for index, line := range lines {
							if strings.HasPrefix(line, "row 23 ") {
								view.conversationOffset = len(lines) - body - index
								break
							}
						}
					}
					return lines[max(0, len(lines)-body-view.conversationOffset)]
				}
				before := topLine()
				if !strings.HasPrefix(before, "row 23 ") {
					t.Fatalf("missing initial anchor: %q", before)
				}
				if tab {
					view.key(2, false)
					if err := view.key('4', false); err != nil {
						t.Fatal(err)
					}
				} else if err := console.selectEditor("ledger"); err != nil {
					t.Fatal(err)
				}
				reply := strings.Repeat(strings.Repeat("z", 100)+"\n", 8)
				if mode == "research" {
					turns = append(turns, sessionintent.ResearchTurn{Question: "second", Answer: reply})
					if err := session.renderResearch(turns); err != nil {
						t.Fatal(err)
					}
				} else {
					if err := console.WriteOutput(reply); err != nil {
						t.Fatal(err)
					}
				}
				expectedMode := "ledger"
				if tab {
					expectedMode = mode
				}
				if view.conversationMode != expectedMode || view.focus != workspaceLedgerPane {
					t.Fatal("reply changed active view")
				}
				if tab {
					view.key(2, false)
					if err := view.key('1', false); err != nil {
						t.Fatal(err)
					}
				} else if err := console.selectEditor(mode); err != nil {
					t.Fatal(err)
				}
				if after := topLine(); after != before {
					t.Fatalf("reply shifted saved viewport: %q -> %q", before, after)
				}
			})
		}
	}
}

func TestWorkspaceModeSwitchDuringIncomingReplies(t *testing.T) {
	console, _ := modeEditorConsole(t)
	done := make(chan error, 1)
	go func() {
		for i := 0; i < 100; i++ {
			if !console.IsTerminal() {
				done <- fmt.Errorf("terminal unavailable during switching")
				return
			}
			if err := console.WriteOutput("Incoming reply\n"); err != nil {
				done <- err
				return
			}
		}
		done <- nil
	}()
	for i := 0; i < 100; i++ {
		for _, mode := range []string{"interact", "research", "ledger", "infer"} {
			if err := console.selectEditor(mode); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestWorkspaceModeSwitchDuringPlanReviewUpdates(t *testing.T) {
	console, _ := modeEditorConsole(t)
	done := make(chan error, 1)
	go func() {
		for i := 0; i < 100; i++ {
			if err := console.ShowPlanReview("Pending review"); err != nil {
				done <- err
				return
			}
			if err := console.HidePlanReview(); err != nil {
				done <- err
				return
			}
			authority, err := console.BeginPlanReviewNote()
			if err != nil {
				done <- err
				return
			}
			if err := console.EndPlanReviewNote(authority); err != nil {
				done <- err
				return
			}
		}
		done <- nil
	}()
	for i := 0; i < 100; i++ {
		for _, mode := range []string{"interact", "research", "ledger", "infer"} {
			if err := console.selectEditor(mode); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if err := console.HidePlanReview(); err != nil {
		t.Fatal(err)
	}
}
