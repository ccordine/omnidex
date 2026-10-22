package main

import (
	"io"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"golang.org/x/term"
)

func workspaceEditor(t *testing.T) (*chatConsole, *io.PipeWriter, chan struct{}, chan string) {
	t.Helper()
	view, _ := testWorkspace(t, 120, 28)
	source, input := io.Pipe()
	t.Cleanup(func() { _ = input.Close(); _ = source.Close() })
	reads := make(chan struct{}, 1)
	paste := newBracketedPasteReader(composerTestReader{source, reads})
	console := &chatConsole{workspace: view, output: view, pasteReader: paste, terminalFD: -1, prompt: "you> "}
	console.terminal = term.NewTerminal(terminalReadWriter{reader: paste, writer: view}, "you> ")
	columns, rows, _ := view.Resize(120, 28)
	console.terminalWidth, console.terminalHeight = columns, rows
	if err := console.terminal.SetSize(columns, rows); err != nil {
		t.Fatal(err)
	}
	done := make(chan string, 1)
	go func() {
		line, _, _, err := console.ReadLine()
		if err != nil {
			done <- "error: " + err.Error()
			return
		}
		done <- line
	}()
	select {
	case <-reads:
	case <-time.After(time.Second):
		t.Fatal("editor did not start")
	}
	return console, input, reads, done
}

func TestWorkspaceDockKeepsDraftDuringStreamingPaneKeysAndResize(t *testing.T) {
	console, input, reads, done := workspaceEditor(t)
	writeComposerKeys(t, input, reads, "ab\x1b[DX")
	if err := console.WriteOutput("omni> Verified source, still implementing.\n"); err != nil {
		t.Fatal(err)
	}
	if err := console.SetActivity(chatActivity{JobID: 7, StepID: 4, Label: "Working", Working: true}); err != nil {
		t.Fatal(err)
	}
	view := console.workspace
	for _, key := range []byte{2, '2', 'k', 2, 'z', 2, '?', 27, 2, '1'} {
		if err := view.key(key, false); err != nil {
			t.Fatal(err)
		}
	}
	columns, rows, err := view.Resize(54, 16)
	if err != nil {
		t.Fatal(err)
	}
	if err := console.terminal.SetSize(columns, rows); err != nil {
		t.Fatal(err)
	}
	frame, _, _, visible := workspaceFrame(view)
	text := ansi.Strip(strings.Join(frame, "\n"))
	for _, wanted := range []string{"03  Compose", "you> aXb", "Verified source"} {
		if !strings.Contains(text, wanted) {
			t.Fatalf("draft/transcript missing %q after UI changes:\n%s", wanted, text)
		}
	}
	if !visible {
		t.Fatal("compose focus lost its editing cursor")
	}
	finishScreenComposer(t, input, done, "aXb")
}

func TestWorkspaceDockPreservesMultilinePaste(t *testing.T) {
	console, input, reads, done := workspaceEditor(t)
	writeComposerKeys(t, input, reads, "before \x1b[200~line one\nline 界\x1b[201~ after")
	if err := console.WriteOutput("[omni] streamed while editing paste\n"); err != nil {
		t.Fatal(err)
	}
	finishScreenComposer(t, input, done, "before line one\nline 界 after")
}

func TestWorkspaceReviewSelectionAndFreezeRemainVisible(t *testing.T) {
	for _, confirming := range []bool{false, true} {
		state := mustPlanReviewState(t, eligiblePlanReviewFixture(t))
		state.selected = len(state.snapshot.Leaves) - 1
		state.confirming = confirming
		text, err := renderPlanReview(state)
		if err != nil {
			t.Fatal(err)
		}
		view, _ := testWorkspace(t, 48, 12)
		if err := view.focusReview(text, state.selected, confirming, false); err != nil {
			t.Fatal(err)
		}
		if _, err := view.Write([]byte("\x1b[?1049h\x1b[?25l\x1b[H\x1b[2J" + strings.ReplaceAll(text, "\n", "\r\n"))); err != nil {
			t.Fatal(err)
		}
		if _, _, err := view.Resize(36, 12); err != nil {
			t.Fatal(err)
		}
		frame, _, _, visible := workspaceFrame(view)
		rendered := ansi.Strip(strings.Join(frame, "\n"))
		want := "> 4. [rejected]"
		if confirming {
			want = "Freeze this work set:"
		}
		if visible || !strings.Contains(rendered, want) {
			t.Fatalf("required approval context lost after resize: %q\n%s", want, rendered)
		}
	}
}

func TestWorkspaceOutputKeepsUntrustedEscapeSequencesLiteral(t *testing.T) {
	console, input, _, done := workspaceEditor(t)
	if err := console.WriteOutput("omni> text \x1b[2J escaped\n"); err != nil {
		t.Fatal(err)
	}
	frame, _, _, _ := workspaceFrame(console.workspace)
	if !strings.Contains(ansi.Strip(strings.Join(frame, "\n")), `text \u001b[2J escaped`) {
		t.Fatal("untrusted text erased or altered the physical terminal")
	}
	finishScreenComposer(t, input, done, "")
}

func TestWorkspaceReflowKeepsSpacesAtSoftWrapBoundaries(t *testing.T) {
	screen := newWorkspaceScreen(6)
	for _, character := range "hello world\r\nnext line" {
		screen.put(character)
	}
	lines, _, _ := workspaceWrappedRows(screen.lines, screen.soft, 40, -1, 0)
	if got := strings.Join(lines, "\n"); got != "hello world\nnext line" {
		t.Fatalf("reflow changed transcript content: %q", got)
	}
}

func TestWorkspaceApprovalCannotBeHiddenByPaneFocusOrHelp(t *testing.T) {
	for _, help := range []bool{false, true} {
		view, _ := testWorkspace(t, 70, 20)
		view.focus, view.zoom, view.help = workspaceActivity, true, help
		if _, err := view.Write([]byte("\x1b[?1049h\x1b[?25l\x1b[H\x1b[2JSelect the frozen task\r\n> pending leaf")); err != nil {
			t.Fatal(err)
		}
		frame, _, _, visible := workspaceFrame(view)
		text := ansi.Strip(strings.Join(frame, "\n"))
		for _, want := range []string{"01  Plan approval", "pending leaf", "Space decide"} {
			if !strings.Contains(text, want) {
				t.Fatalf("approval obscured with help=%v: missing %q\n%s", help, want, text)
			}
		}
		if visible || strings.Contains(text, "KEYS  /") {
			t.Fatal("approval lost exclusive presentation")
		}
	}
}

func TestWorkspaceOldestScrollUsesActualSplitPaneWidth(t *testing.T) {
	view, _ := testWorkspace(t, 120, 24)
	text := "FIRST LINE\r\n" + strings.Repeat("A long conversation line with several words to wrap across the pane. ", 100)
	if _, err := view.Write([]byte(text)); err != nil {
		t.Fatal(err)
	}
	view.beginInput()
	if _, err := view.Write([]byte("\r\nyou> draft")); err != nil {
		t.Fatal(err)
	}
	view.focus = workspaceConversation
	if err := view.key('g', false); err != nil {
		t.Fatal(err)
	}
	frame, _, _, _ := workspaceFrame(view)
	if !strings.Contains(ansi.Strip(frame[2]), "FIRST LINE") {
		t.Fatalf("oldest scroll stopped before the first line: %q", ansi.Strip(frame[2]))
	}
}

func TestWorkspaceTabSwitchPreservesScrollAndEditor(t *testing.T) {
	console, input, reads, done := workspaceEditor(t)
	writeComposerKeys(t, input, reads, "界ab\x1b[DX")
	view := console.workspace
	view.mu.Lock()
	view.conversationOffset, view.activityOffset = 6, 3
	view.mu.Unlock()
	for _, key := range []byte{2, '2', 2, '\t', 2, '[', 2, '1'} {
		if err := view.key(key, false); err != nil {
			t.Fatal(err)
		}
	}
	view.mu.Lock()
	if view.conversationOffset != 6 || view.activityOffset != 3 {
		t.Error("tab switch reset viewport")
	}
	view.mu.Unlock()
	finishScreenComposer(t, input, done, "界aXb")
}

func TestWorkspaceModeShortcutsAndPasteIsolation(t *testing.T) {
	view, _ := testWorkspace(t, 32, 12)
	source := &workspaceInputSource{view: view}
	for _, key := range []byte("\x02f\x02e\x02l\x02r") {
		if err := source.consume(key); err != nil {
			t.Fatal(err)
		}
	}
	for _, want := range []string{"infer", "interact", "ledger", "research"} {
		select {
		case got := <-view.modeRequests:
			if got != want {
				t.Fatalf("mode %q, want %q", got, want)
			}
		default:
			t.Fatal("missing mode request")
		}
	}
	pasted := "\x1b[200~界\x02r\x02\t\x1b[201~"
	for _, key := range []byte(pasted) {
		if err := source.consume(key); err != nil {
			t.Fatal(err)
		}
	}
	if string(source.queued) != pasted {
		t.Fatalf("paste changed: %q", source.queued)
	}
	select {
	case <-view.modeRequests:
		t.Fatal("paste switched mode")
	default:
	}
}
