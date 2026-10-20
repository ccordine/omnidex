package codex

import (
	"bufio"
	"bytes"
	"encoding/json"
	"testing"

	"github.com/gryph/omnidex/internal/codingagent"
)

func TestResearchTransportReadOnlyDiskAndLiveWeb(t *testing.T) {
	writer := &protocolWriter{}
	responses := "{\"id\":1,\"result\":{}}\n{\"id\":2,\"result\":{\"config\":{\"mcp_servers\":{\"external\":{}}}}}\n{\"id\":3,\"result\":{\"model\":\"fixture\",\"thread\":{\"id\":\"research-thread\"}}}\n{\"id\":4,\"result\":{\"turn\":{\"id\":\"research-turn\"}}}\n"
	connection := &connection{input: writer, output: bufio.NewScanner(bytes.NewBufferString(responses))}
	route := codingagent.Route{Provider: "codex", Model: "fixture"}
	thread, err := connection.startResearchThread("/workspace", route)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := connection.startAgentTurn(codingagent.Assignment{Role: codingagent.Plan, Route: route}, thread, "Exact research question"); err != nil {
		t.Fatal(err)
	}
	requests := bufio.NewScanner(bytes.NewReader(writer.Bytes()))
	seen := 0
	for requests.Scan() {
		var request struct {
			Method string         `json:"method"`
			Params map[string]any `json:"params"`
		}
		if err := json.Unmarshal(requests.Bytes(), &request); err != nil {
			t.Fatal(err)
		}
		switch request.Method {
		case "thread/start":
			seen++
			p := request.Params
			config := p["config"].(map[string]any)
			features := config["features"].(map[string]any)
			if p["sandbox"] != "read-only" || p["approvalPolicy"] != "never" || p["ephemeral"] != true || config["web_search"] != "live" || features["shell_tool"] != true || features["unified_exec"] != true || features["apps"] != false || features["plugins"] != false || features["multi_agent"] != false {
				t.Fatal(p)
			}
			if config["mcp_servers"].(map[string]any)["external"].(map[string]any)["enabled"] != false {
				t.Fatal("research inherited MCP access")
			}
		case "turn/start":
			seen++
			policy := request.Params["sandboxPolicy"].(map[string]any)
			if policy["type"] != "readOnly" || policy["networkAccess"] != false || request.Params["approvalPolicy"] != "never" || request.Params["outputSchema"] != nil {
				t.Fatal(request.Params)
			}
		}
	}
	if seen != 2 {
		t.Fatal("missing research thread or turn")
	}
}

func TestResearchProtocolRetainsToolReceiptsAndRejectsWriteItems(t *testing.T) {
	result := codingagent.Result{ThreadID: "research-thread", TurnID: "research-turn"}
	open, closed := map[string]string{}, map[string]string{}
	for _, method := range []string{"item/started", "item/completed"} {
		raw := json.RawMessage(`{"method":"` + method + `","params":{"threadId":"research-thread","turnId":"research-turn","item":{"id":"read-command","type":"commandExecution","status":"completed","aggregatedOutput":"observed disk bytes"}}}`)
		var frame rpcFrame
		if err := json.Unmarshal(raw, &frame); err != nil {
			t.Fatal(err)
		}
		if _, err := consumeResearchFrame(frame, raw, &result, open, closed); err != nil {
			t.Fatal(err)
		}
	}
	if len(result.Evidence) != 2 || len(open) != 0 {
		t.Fatal("lost native receipts", result.Evidence, open)
	}
	for _, kind := range []string{"fileChange", "mcpToolCall", "collabAgentToolCall"} {
		raw := json.RawMessage(`{"method":"item/started","params":{"item":{"type":"` + kind + `"}}}`)
		var frame rpcFrame
		if err := json.Unmarshal(raw, &frame); err != nil {
			t.Fatal(err)
		}
		if _, err := consumeResearchFrame(frame, raw, &result, open, closed); err == nil {
			t.Fatal("forbidden Research item accepted", kind)
		}
	}
}
