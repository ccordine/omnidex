package omni

import (
	"bytes"
	"io"
	"reflect"
	"strings"
	"testing"
)

func TestAppChatUsesCoreAgentCLI(t *testing.T) {
	var gotArgs []string
	var gotInput string
	app := NewApp(strings.NewReader("follow-up\n"), io.Discard, io.Discard)
	app.agentCLI = func(args []string, input io.Reader) error {
		gotArgs = append([]string(nil), args...)
		blob, err := io.ReadAll(input)
		if err != nil {
			return err
		}
		gotInput = string(blob)
		return nil
	}

	if err := app.Run([]string{"chat", "--agent", "omnidex"}); err != nil {
		t.Fatal(err)
	}
	if want := []string{"chat", "--agent", "omnidex"}; !reflect.DeepEqual(gotArgs, want) {
		t.Fatalf("agent-cli args=%v want %v", gotArgs, want)
	}
	if gotInput != "follow-up\n" {
		t.Fatalf("agent-cli stdin=%q", gotInput)
	}
}

func TestAppRunSendsOneCompleteInstructionToCoreAgentCLI(t *testing.T) {
	var gotArgs []string
	var gotInput string
	app := NewApp(strings.NewReader("build the complete notes app\nwith tests\n"), io.Discard, io.Discard)
	app.agentCLI = func(args []string, input io.Reader) error {
		gotArgs = append([]string(nil), args...)
		blob, err := io.ReadAll(input)
		if err != nil {
			return err
		}
		gotInput = string(blob)
		return nil
	}

	if err := app.Run([]string{"run", "--agent", "omnidex"}); err != nil {
		t.Fatal(err)
	}
	want := []string{"run", "--agent", "omnidex", "build the complete notes app\nwith tests"}
	if !reflect.DeepEqual(gotArgs, want) {
		t.Fatalf("agent-cli args=%v want %v", gotArgs, want)
	}
	if gotInput != "" {
		t.Fatalf("one-shot agent-cli stdin=%q want EOF", gotInput)
	}
}

func TestAppRejectsRemovedLocalAgentCommands(t *testing.T) {
	for _, command := range []string{
		"ledger", "bench", "run:trace", "fastpath", "ollama",
	} {
		t.Run(command, func(t *testing.T) {
			app := NewApp(strings.NewReader(""), io.Discard, io.Discard)
			app.agentCLI = func([]string, io.Reader) error {
				t.Fatal("removed local command reached agent-cli")
				return nil
			}
			err := app.Run([]string{command})
			if err == nil || !strings.Contains(err.Error(), "removed") {
				t.Fatalf("Run(%q) error=%v", command, err)
			}
		})
	}
}

func TestAppRejectsEmptyOneShotInstruction(t *testing.T) {
	app := NewApp(bytes.NewBufferString(" \n"), io.Discard, io.Discard)
	app.agentCLI = func([]string, io.Reader) error {
		t.Fatal("empty instruction reached agent-cli")
		return nil
	}
	err := app.Run([]string{"run"})
	if err == nil || !strings.Contains(err.Error(), "instruction is required") {
		t.Fatalf("error=%v", err)
	}
}
