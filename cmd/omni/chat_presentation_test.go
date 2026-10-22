package main

import (
	"fmt"
	"github.com/charmbracelet/x/ansi"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

func TestNativeMultilinePresentationRetainsIndentationBlankLinesAndQuotes(t *testing.T) {
	console, output := planReviewTestConsole(t)
	renderer := chatRenderer{console: console, verbosity: chatVerbose}
	text := "```go\nfunc main() {\n\n\tprintln(\"a  b\")\n}\n```"
	for _, fragment := range []string{text[:17], text[17:30], text[30:]} {
		if err := renderer.realtime(nativeRenderEvent(nativeRenderActivity(model.AgentActivityMessage, model.AgentActivityDelta, fragment))); err != nil {
			t.Fatal(err)
		}
	}
	if err := renderer.realtime(nativeRenderEvent(nativeRenderActivity(model.AgentActivityMessage, model.AgentActivityCompleted, ""))); err != nil {
		t.Fatal(err)
	}
	expected := "omni> " + strings.ReplaceAll(text, "\n", "\n      ") + "\n"
	// x/term converts line endings for the physical terminal.
	if got := strings.ReplaceAll(output.String(), "\r\n", "\n"); got != expected {
		t.Fatalf("multiline text changed:\n%q\nwant %q", got, expected)
	}
}

func TestCommandOutcomeKeepsMultilineCommandAndExit(t *testing.T) {
	console, output := planReviewTestConsole(t)
	renderer := chatRenderer{console: console, verbosity: chatVerbose}
	activity := nativeRenderActivity(model.AgentActivityCommand, model.AgentActivityFailed, "")
	activity.Command = "printf '%s\\n' 'a  b'\n\n  exit 7"
	exit := 7
	activity.ExitCode = &exit
	if err := renderer.realtime(nativeRenderEvent(activity)); err != nil {
		t.Fatal(err)
	}
	got := strings.ReplaceAll(output.String(), "\r\n", "\n")
	want := "[omni] command failed · exit 7\ncommand> printf '%s\\n' 'a  b'\n         \n           exit 7\n"
	if got != want {
		t.Fatalf("outcome/command changed: %q; want %q", got, want)
	}
}

func TestWorkspaceScrolledConversationStaysOnSameTextDuringStream(t *testing.T) {
	console, _, _, _ := workspaceEditor(t)
	for i := 0; i < 30; i++ {
		if err := console.WriteOutput(strings.Repeat("x", i+1) + "\n"); err != nil {
			t.Fatal(err)
		}
	}
	view := console.workspace
	view.mu.Lock()
	view.focus = workspaceConversation
	view.conversationOffset = 8
	view.mu.Unlock()
	before, _, _, _ := workspaceFrame(view)
	if err := console.WriteOutput("new streamed line\n\n    indented output\n"); err != nil {
		t.Fatal(err)
	}
	after, _, _, _ := workspaceFrame(view)
	// Compare the conversation body; the status reports its increased distance
	// from the latest output, and the compose dock remains independently owned.
	for row := 2; row < 15; row++ {
		if before[row] != after[row] {
			t.Fatalf("scrolled row %d moved during stream:\n%q\n%q", row, before[row], after[row])
		}
	}
	if view.focus != workspaceConversation {
		t.Fatal("stream stole focus")
	}
}

func TestWorkspaceScrolledConversationRetainsTopSourceOnResize(t *testing.T) {
	console, _, _, _ := workspaceEditor(t)
	for i := 0; i < 45; i++ {
		if err := console.WriteOutput(fmt.Sprintf("line %02d: quoted 'a  b'\n", i)); err != nil {
			t.Fatal(err)
		}
	}
	view := console.workspace
	view.mu.Lock()
	view.focus = workspaceConversation
	view.conversationOffset = 12
	view.mu.Unlock()
	before, _, _, _ := workspaceFrame(view)
	top := strings.TrimSpace(ansi.Strip(strings.Split(before[2], "│")[0]))
	for _, size := range [][2]int{{54, 18}, {120, 28}} {
		width, height, err := view.Resize(size[0], size[1])
		if err != nil {
			t.Fatal(err)
		}
		if err := console.terminal.SetSize(width, height); err != nil {
			t.Fatal(err)
		}
		frame, _, _, _ := workspaceFrame(view)
		got := strings.TrimSpace(ansi.Strip(strings.Split(frame[2], "│")[0]))
		if got != top {
			t.Fatalf("resize moved top source: %q; want %q", got, top)
		}
	}
}
