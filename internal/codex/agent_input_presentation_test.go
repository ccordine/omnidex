package codex

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestNativeChoicesPresentExactDetailsAndResponses(t *testing.T) {
	params := json.RawMessage(`{"threadId":"t","turnId":"u","command":"printf '%s\\n' 'a  b'\n\n  exit 0","cwd":"/synthetic/path with spaces","availableDecisions":["accept","decline","cancel"]}`)
	request, err := nativeInputRequest(rpcFrame{ID: json.RawMessage(`17`), Method: "item/commandExecution/requestApproval", Params: params})
	if err != nil {
		t.Fatal(err)
	}
	var details bytes.Buffer
	if err := json.Indent(&details, params, "", "  "); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(request.Prompt, details.String()) || !strings.HasSuffix(request.Prompt, "Reply with one exact choice:\n  approve\n  deny\n  cancel") {
		t.Fatalf("details/choices changed: %s", request.Prompt)
	}
	for i, label := range []string{"approve", "deny", "cancel"} {
		response, err := request.Reply(label)
		if err != nil || !bytes.Equal(response, request.Choices[i].Response) {
			t.Fatalf("choice %q changed authority: %s %v", label, response, err)
		}
	}
}
