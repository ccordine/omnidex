package codex

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/codingagent"
)

func TestAgentDoesNotDispatchExecutionAfterContextInjectionFailure(t *testing.T) {
	call := agentTestScope(t)
	command := filepath.Join(call.Root, "codex-fixture")
	transcript := filepath.Join(call.Root, "requests.jsonl")
	fixture := fmt.Sprintf(`#!/usr/bin/env python3
import json, sys
for line in sys.stdin:
    request = json.loads(line)
    with open(%q, 'a') as trace:
        trace.write(line)
    if 'id' not in request:
        continue
    response = {'id': request['id'], 'result': {}}
    if request['method'] == 'thread/start':
        response['result'] = {'model': 'fixture-model', 'thread': {'id': 'thread'}}
    elif request['method'] == 'thread/inject_items':
        response = {'id': request['id'], 'error': {'code': -32601, 'message': 'history injection unavailable'}}
    print(json.dumps(response), flush=True)
`, transcript)
	if err := os.WriteFile(command, []byte(fixture), 0o700); err != nil {
		t.Fatal(err)
	}
	client, err := New(command, call.Root, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	dispatched := false
	result, err := client.Run(ctx, call, func(event codingagent.NativeEvent) error {
		dispatched = dispatched || event.Session != nil
		return nil
	})
	var failure *agentInferenceFailure
	if !errors.As(err, &failure) || !failure.Retryable() || failure.Error() != "Codex RPC thread/inject_items failed with code -32601" || result.Completed || dispatched {
		t.Fatalf("failed injection dispatched execution: result=%+v dispatched=%v error=%v", result, dispatched, err)
	}
	raw, err := os.ReadFile(transcript)
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		var request struct {
			Method string `json:"method"`
		}
		if err := json.Unmarshal([]byte(line), &request); err != nil {
			t.Fatal(err)
		}
		if request.Method == "turn/start" {
			t.Fatal("execution fell back to a context-stuffed user prompt")
		}
	}
}
