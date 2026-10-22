package main

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/gryph/omnidex/internal/client"
	"github.com/gryph/omnidex/internal/model"
)

func testWorkspace(t *testing.T, width, height int) (*chatWorkspace, *bytes.Buffer) {
	t.Helper()
	output := &bytes.Buffer{}
	view, err := newChatWorkspace(output, width, height)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := view.Close(); err != nil {
			t.Error(err)
		}
	})
	return view, output
}

func workspaceFrame(view *chatWorkspace) ([]string, int, int, bool) {
	view.mu.Lock()
	defer view.mu.Unlock()
	return view.frameLocked()
}

func TestWorkspaceFramesFitWideNarrowAndUnicodeTerminals(t *testing.T) {
	for _, size := range [][2]int{{140, 40}, {100, 24}, {80, 24}, {38, 12}, {24, 8}, {8, 8}, {8, 4}} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			view, _ := testWorkspace(t, size[0], size[1])
			if err := view.SetProject("/projects/界🙂/calculator", "feature/terminal"); err != nil {
				t.Fatal(err)
			}
			if _, err := view.Write([]byte("omni> 界🙂é\r\nyou> draft")); err != nil {
				t.Fatal(err)
			}
			frame, row, column, visible := workspaceFrame(view)
			if len(frame) != size[1] {
				t.Fatalf("height %d, want %d", len(frame), size[1])
			}
			for index, line := range frame {
				if ansi.StringWidth(line) > size[0] {
					t.Fatalf("row %d overflows %d cells: %q", index, size[0], line)
				}
			}
			if size[0] >= 24 && size[1] >= 5 && !strings.Contains(ansi.Strip(frame[len(frame)-1]), "Ctrl-B ? keys") {
				t.Fatal("narrow terminal hid discoverable key help")
			}
			if visible && (row < 0 || row >= size[1] || column < 0 || column >= size[0]) {
				t.Fatalf("cursor escaped terminal: %d,%d", row, column)
			}
			if size[0] >= 100 && !strings.Contains(ansi.Strip(strings.Join(frame, "\n")), "02  Activity") {
				t.Fatal("wide terminal lost split activity pane")
			}
		})
	}
}

func TestWorkspaceReviewPreservesConversationAndRestoresTerminal(t *testing.T) {
	view, output := testWorkspace(t, 120, 24)
	for _, text := range []string{"omni> keep this transcript\r\nyou> draft", "\x1b[?1049h\x1b[?25l", "\x1b[H\x1b[2JPLAN APPROVAL\r\n> pending leaf"} {
		if _, err := view.Write([]byte(text)); err != nil {
			t.Fatal(err)
		}
	}
	frame, _, _, visible := workspaceFrame(view)
	if visible || !strings.Contains(ansi.Strip(strings.Join(frame, "\n")), "pending leaf") {
		t.Fatal("review did not replace the conversation view")
	}
	if _, err := view.Write([]byte("\x1b[?25h\x1b[?1049l")); err != nil {
		t.Fatal(err)
	}
	frame, _, _, visible = workspaceFrame(view)
	if !visible || !strings.Contains(ansi.Strip(strings.Join(frame, "\n")), "keep this transcript") {
		t.Fatal("leaving approval lost transcript or composer")
	}
	if err := view.Close(); err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(output.String(), "\x1b[0m\x1b[?25h\x1b[?1049l") {
		t.Fatal("terminal colors/cursor/screen were not restored")
	}
	if _, err := view.Write([]byte("late")); err != io.ErrClosedPipe {
		t.Fatalf("closed view accepted output: %v", err)
	}
}

func TestWorkspaceTypedPaneKeysNeverEnterTheMessage(t *testing.T) {
	view, _ := testWorkspace(t, 120, 24)
	source := workspaceInputSource{view: view}
	for _, value := range []byte("draft\x022kk\x02z\x02?\x1b[A\x021 end") {
		if err := source.consume(value); err != nil {
			t.Fatal(err)
		}
	}
	if string(source.queued) != "draft end" {
		t.Fatalf("pane controls leaked into input: %q", source.queued)
	}
	if view.focus != workspaceCompose || view.conversationOffset != 0 {
		t.Fatal("compose key did not restore live input")
	}
}

func TestWorkspacePastePreservesControlBytesAndExactText(t *testing.T) {
	view, _ := testWorkspace(t, 120, 24)
	view.focus = workspaceActivity
	source := workspaceInputSource{view: view}
	text := "\x1b[200~literal \x02z\n界\x1b[6~\x1b[201~"
	for _, value := range []byte(text) {
		if err := source.consume(value); err != nil {
			t.Fatal(err)
		}
	}
	if string(source.queued) != text {
		t.Fatalf("paste was interpreted as pane commands: %q", source.queued)
	}
	if view.zoom || view.focus != workspaceCompose {
		t.Fatal("paste changed layout instead of preserving text")
	}
}

func TestWorkspaceLiveStatusUsesOnlyObservedEvents(t *testing.T) {
	view, _ := testWorkspace(t, 120, 28)
	console := &chatConsole{workspace: view}
	if err := view.SetActivity(chatActivity{JobID: 41, StepID: 3, Label: "Working", Working: true, StartedAt: time.Now().Add(-3 * time.Second)}, "Checking the selected task"); err != nil {
		t.Fatal(err)
	}
	exit := 1
	event := client.RealtimeEvent{AgentActivity: &model.AgentActivity{Role: "execute", Kind: model.AgentActivityCommand, Status: model.AgentActivityCompleted, Command: "go test ./...", ExitCode: &exit}}
	if err := console.observeWorkspaceEvent(event); err != nil {
		t.Fatal(err)
	}
	frame, _, _, _ := workspaceFrame(view)
	rendered := ansi.Strip(strings.Join(frame, "\n"))
	for _, text := range []string{"Executing task", "go test ./...", "Command exited 1", "Job #41", "Checking the selected task"} {
		if !strings.Contains(rendered, text) {
			t.Fatalf("missing observed %q: %s", text, rendered)
		}
	}
	if strings.Contains(rendered, "100%") || strings.Contains(rendered, "tokens") {
		t.Fatal("view invented progress or token telemetry")
	}
}

func TestWorkspaceNoColorKeepsTheSameUsefulLayout(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	view, _ := testWorkspace(t, 120, 24)
	frame, _, _, _ := workspaceFrame(view)
	for _, line := range frame {
		if strings.Contains(line, "\x1b[") {
			t.Fatal("NO_COLOR retained color sequences")
		}
	}
	if !strings.Contains(strings.Join(frame, "\n"), "INSERT") {
		t.Fatal("monochrome removed mode information")
	}
}

func TestWorkspaceBoundsItsDisplayScrollback(t *testing.T) {
	screen := newWorkspaceScreen(80)
	for index := 0; index < workspaceScrollbackLines+100; index++ {
		for _, r := range fmt.Sprintf("line %d\r\n", index) {
			screen.put(r)
		}
	}
	if len(screen.lines) != workspaceScrollbackLines || screen.row != workspaceScrollbackLines-1 {
		t.Fatalf("unbounded or invalid screen: lines=%d row=%d", len(screen.lines), screen.row)
	}
}

func TestWorkspaceReviewInputKeepsReviewKeysAndAllowsPaneControls(t *testing.T) {
	view, _ := testWorkspace(t, 70, 20)
	keys := "\x02z\x1b[B \r"
	results := make(chan planReviewTestSourceResult, 1)
	results <- planReviewTestSourceResult{data: []byte(keys)}
	source := &workspaceInputSource{
		source: &planReviewTestSource{results: results}, view: view,
		review: func() bool { return true },
	}
	var buffer [64]byte
	count, err := source.Read(buffer[:])
	if err != nil || string(buffer[:count]) != "\x1b[B \r" {
		t.Fatalf("review keys were changed: %q, %v", buffer[:count], err)
	}
	if !view.zoom || view.prefix || view.focus != workspaceCompose {
		t.Fatal("review input did not execute the explicit pane command")
	}
}

func TestWorkspaceUnchangedIdleFrameDoesNotRedraw(t *testing.T) {
	view, output := testWorkspace(t, 120, 24)
	if _, _, err := view.Resize(120, 24); err != nil {
		t.Fatal(err)
	}
	length := output.Len()
	if err := view.SetActivity(chatActivity{}, ""); err != nil {
		t.Fatal(err)
	}
	if output.Len() != length {
		t.Fatal("idle view emitted a redundant frame/cursor redraw")
	}
}
