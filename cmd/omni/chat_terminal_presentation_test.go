package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/model"
)

// Opt-in PTY pilot: uses the production console, editor, pane keys and resize
// watcher. It has no workflow or execution authority and no service connection.
func TestTerminalPresentationPTY(t *testing.T) {
	if os.Getenv("OMNI_TEST_TERMINAL_PTY") != "1" {
		t.Skip("requires an isolated interactive PTY")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 90*time.Second)
	defer cancel()
	console, err := newChatConsole(ctx, os.Stdin, os.Stdout, os.Stderr)
	if err != nil {
		t.Fatal(err)
	}
	defer console.Close()
	displayErrors, stop := startChatActivityDisplay(ctx, console)
	defer stop()
	renderer := chatRenderer{console: console, verbosity: chatVerbose}
	if err := console.workspace.acceptLedger(panelLedgerFixture()); err != nil {
		t.Fatal(err)
	}
	if err := console.SetActivity(chatActivity{JobID: 41, StepID: 12, Label: "Executing task", Working: true}); err != nil {
		t.Fatal(err)
	}
	command := nativeRenderActivity(model.AgentActivityCommand, model.AgentActivityStarted, "")
	command.Command, command.CWD = "printf '%s\\n' 'a  b'\n\n  exit 0", "/synthetic/path with spaces"
	if err := renderer.realtime(nativeRenderEvent(command)); err != nil {
		t.Fatal(err)
	}
	output := nativeRenderActivity(model.AgentActivityCommandOutput, model.AgentActivityCompleted, "alpha\n\n    beta 'quoted'\n")
	if err := renderer.realtime(nativeRenderEvent(output)); err != nil {
		t.Fatal(err)
	}
	exit := 0
	command.Status, command.ExitCode = model.AgentActivityCompleted, &exit
	if err := renderer.realtime(nativeRenderEvent(command)); err != nil {
		t.Fatal(err)
	}
	if err := renderer.message(model.ChannelMessage{Role: model.ChannelMessageRoleAssistant, Content: "Reply with one exact choice:\n  approve\n  deny\n  cancel"}); err != nil {
		t.Fatal(err)
	}
	streamDone := make(chan error, 1)
	go func() {
		ticker := time.NewTicker(400 * time.Millisecond)
		defer ticker.Stop()
		for i := 0; i < 40; i++ {
			select {
			case <-ctx.Done():
				streamDone <- nil
				return
			case request := <-console.workspace.ledger.requests:
				if request.key != 'r' {
					streamDone <- fmt.Errorf("unexpected pilot mutation %q", request.key)
					return
				}
				if err := console.workspace.acceptLedger(panelLedgerFixture()); err != nil {
					streamDone <- err
					return
				}
				continue
			case <-ticker.C:
			}
			if err := renderer.nativeActivity(nativeRenderEvent(nativeRenderActivity(model.AgentActivityMessage, model.AgentActivityCompleted, "```go\nfunc example() {\n\n    println(\"a  b\")\n}\n```"))); err != nil {
				streamDone <- err
				return
			}
		}
		streamDone <- nil
	}()
	line, _, _, err := console.ReadLine()
	cancel()
	if streamErr := <-streamDone; streamErr != nil {
		t.Fatal(streamErr)
	}
	if err != nil {
		t.Fatal(err)
	}
	if line != "aXb\n  quoted 'value'\n\nend" {
		t.Fatalf("draft/cursor/paste changed: %q", line)
	}
	select {
	case err := <-displayErrors:
		t.Fatal(err)
	default:
	}
	if err := console.Close(); err != nil {
		t.Fatal(err)
	}
	t.Logf("actual PTY draft/cursor preserved; submitted %q; ledger selection %s; terminal restored", strings.ReplaceAll(line, "\n", "\\n"), console.workspace.ledger.selected)
}
