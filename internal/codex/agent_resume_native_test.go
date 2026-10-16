package codex

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/codingagent"
)

func TestInstalledCodexExecutionQuestionResumesSameThread(t *testing.T) {
	client, model := installedAgentClient(t)
	call := nativeAgentFixture(t, model)
	t.Setenv("GIT_CEILING_DIRECTORIES", call.Root)
	call.Task.Goal = "Before any tool use or edits, ask the owner which name to use for verification and wait for the answer. After the owner answers, implement greet according to contract.txt and verify it with that name."
	call.Task.Acceptance = []string{"After the owner provides a verification name, greet must fulfill contract.txt for that name and for an empty name."}
	call.Task.Checks = nil
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	frames := []json.RawMessage{}
	event := func(value codingagent.NativeEvent) error {
		if len(value.Frame) > 0 {
			frames = append(frames, append(json.RawMessage(nil), value.Frame...))
		}
		return nil
	}
	t.Cleanup(func() {
		if !t.Failed() {
			return
		}
		for _, raw := range frames {
			var frame struct {
				Method string `json:"method"`
				Params struct {
					Item struct {
						Type string `json:"type"`
					} `json:"item"`
				} `json:"params"`
			}
			_ = json.Unmarshal(raw, &frame)
			if frame.Method == "item/completed" && frame.Params.Item.Type == "agentMessage" {
				t.Logf("native_message_receipt=%s", raw)
			}
		}
	})
	before, err := os.ReadFile(filepath.Join(call.Root, "allowed.py"))
	if err != nil {
		t.Fatal(err)
	}
	first, err := client.Run(ctx, call, event)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("initial question completed: thread=%s turn=%s", first.ThreadID, first.TurnID)
	after, err := os.ReadFile(filepath.Join(call.Root, "allowed.py"))
	if err != nil || string(before) != string(after) || !first.Completed {
		t.Fatalf("question executed before owner answer: %v", err)
	}
	call.Session = &codingagent.ExecutionSession{ThreadID: first.ThreadID, TurnID: first.TurnID, State: "idle"}
	call.Reply = "Use Ada for verification. Proceed with the already approved task."
	second, err := client.Run(ctx, call, event)
	if err != nil {
		t.Fatal(err)
	}
	if !second.Completed || second.ThreadID != first.ThreadID || second.TurnID == first.TurnID {
		t.Fatal("owner answer did not continue the exact execution thread")
	}
	command := exec.Command("python3", "-B", "-c", `from allowed import greet; assert greet("Ada")=="Hello, Ada!"; assert greet("")=="Hello, !"`)
	command.Dir = call.Root
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("resumed execution did not fulfill the task: %v %s", err, output)
	}
}
