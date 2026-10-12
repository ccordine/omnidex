package codex

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/llm"
)

func TestNativeFramesRejectEffectsAndForeignTurns(t *testing.T) {
	for _, raw := range []string{
		`{"method":"item/started","params":{"threadId":"thread","turnId":"turn","item":{"type":"commandExecution"}}}`,
		`{"method":"item/completed","params":{"threadId":"foreign","turnId":"turn","item":{"type":"agentMessage","phase":"final_answer"}}}`,
		`{"method":"turn/completed","params":{"threadId":"thread","turn":{"id":"turn","status":"interrupted"}}}`,
	} {
		var frame rpcFrame
		if err := json.Unmarshal([]byte(raw), &frame); err != nil {
			t.Fatal(err)
		}
		var capture llm.CodexResponseCapture
		if _, err := collectGenerationFrame(frame, []byte(raw), &capture, "thread", "turn"); err == nil {
			t.Fatal("effect, foreign turn, or interrupted completion accepted")
		}
	}
}

func TestNativeRequestCannotConsumeForeignContext(t *testing.T) {
	client, err := New("codex", ".", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	request := nativeTestRequest("model", "One value.")
	request.RetainedContext = []int{1}
	if _, err := client.PrepareExactRequest(request); err == nil {
		t.Fatal("foreign token context accepted")
	}
}

func nativeTestRequest(model, prompt string) llm.PreparedModel {
	return llm.PreparedModel{Protocol: llm.ExactPreparedProtocolCodexV1, BaseModel: model, ContextModel: model,
		Prompt: prompt, MaxOutputTokens: -1, ContextTokens: 131072, OutputLimitMode: llm.ExactPreparedOutputLimitExplicit, RetainProviderContext: true}
}

// Uses the production adapter, installed authentication and original RPC receipts.
// It also proves context remains usable after the first app-server process exits.
func TestInstalledCodexGenerationAndContinuation(t *testing.T) {
	command, model := os.Getenv("OMNI_TEST_CODEX_COMMAND"), os.Getenv("OMNI_TEST_CODEX_MODEL")
	if command == "" || model == "" {
		t.Skip("installed Codex qualification not selected")
	}
	client, err := New(command, ".", 2*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	request := nativeTestRequest(model, "Remember the arbitrary label SAPPHIRE73. Return only SAPPHIRE73.")
	first, err := client.GeneratePreparedExact(context.Background(), request)
	if err != nil {
		if native, ok := err.(*rpcError); ok && native.method == "turn/start" {
			t.Fatalf("qualification request rejected: %s", native.message)
		}
		t.Fatal(err)
	}
	if err := llm.ValidateExactPreparedGenerationForRequest(request, first); err != nil {
		t.Fatal(err)
	}
	if first.Content != "SAPPHIRE73" {
		t.Fatalf("first native answer = %q", first.Content)
	}
	request.RetainedProviderContext, err = llm.DecodeCodexRetainedContext(first.ProviderResponseCapture)
	if err != nil {
		t.Fatal(err)
	}
	request.Prompt = "Return only the arbitrary label from the preceding turn."
	second, err := client.GeneratePreparedExact(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if err := llm.ValidateExactPreparedGenerationForRequest(request, second); err != nil {
		t.Fatal(err)
	}
	if second.Content != "SAPPHIRE73" {
		t.Fatalf("continued native answer = %q", second.Content)
	}
	t.Logf("actual native usage: first=%+v continuation=%+v", first.Usage, second.Usage)
}

func TestInstalledCodexCancellation(t *testing.T) {
	command := os.Getenv("OMNI_TEST_CODEX_COMMAND")
	if command == "" {
		t.Skip("installed Codex qualification not selected")
	}
	client, err := New(command, ".", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result, err := client.GeneratePreparedExact(ctx, nativeTestRequest("model", "One value."))
	if err == nil || result.ProviderRequestDisposition != llm.ProviderRequestNotDispatched || result.ProviderDone {
		t.Fatal("canceled request was dispatched or claimed completion")
	}
}

func TestInstalledCodexProcessCancellation(t *testing.T) {
	command := os.Getenv("OMNI_TEST_CODEX_COMMAND")
	if command == "" {
		t.Skip("installed Codex qualification not selected")
	}
	ctx, cancel := context.WithCancel(context.Background())
	connection, err := startConnection(ctx, command, ".")
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	cancel()
	if _, err := connection.next(); err == nil {
		t.Fatal("canceled process retained its RPC stream")
	}
	connection.close()
	if connection.process.ProcessState == nil {
		t.Fatal("canceled child process was not reaped")
	}
}
