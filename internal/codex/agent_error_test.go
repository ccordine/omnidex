package codex

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/gryph/omnidex/internal/codingagent"
)

func TestAgentNativeRetryNotificationPreservesEvidenceAndWaitsForCompletion(t *testing.T) {
	frames := []string{
		`{"method":"error","params":{"threadId":"thread","turnId":"turn","willRetry":true,"error":{"message":"Stream disconnected; retrying","codexErrorInfo":{"httpConnectionFailed":{"httpStatusCode":503}}}}}`,
		`{"method":"item/started","params":{"threadId":"thread","turnId":"turn","item":{"id":"answer","type":"agentMessage"}}}`,
		`{"method":"item/completed","params":{"threadId":"thread","turnId":"turn","item":{"id":"answer","type":"agentMessage","phase":"final_answer","text":"{}"}}}`,
		`{"method":"turn/completed","params":{"threadId":"thread","turn":{"id":"turn","status":"completed","error":null}}}`,
	}
	result := codingagent.Result{ThreadID: "thread", TurnID: "turn"}
	items := map[string]string{}
	var persisted []json.RawMessage
	for index, raw := range frames {
		finished, err := consumeAgentFrame(decodeAgentTestFrame(t, raw), json.RawMessage(raw), &result, codingagent.Plan, items, func(event codingagent.NativeEvent) error {
			persisted = append(persisted, append(json.RawMessage(nil), event.Frame...))
			return nil
		})
		if err != nil {
			t.Fatalf("native retry sequence failed at frame %d: %v", index, err)
		}
		if finished != (index == len(frames)-1) || result.Completed != finished {
			t.Fatalf("native retry or partial answer became terminal success at frame %d", index)
		}
		if !bytes.Equal(persisted[index], []byte(raw)) {
			t.Fatal("original native receipt changed before durable persistence")
		}
	}
	if len(persisted) != len(frames) || len(items) != 0 {
		t.Fatal("completed native retry sequence lost evidence or native item accounting")
	}
}

func TestAgentTransportTerminationRequiresFreshRetryableSession(t *testing.T) {
	for _, cause := range []error{errors.New("unexpected EOF"), context.DeadlineExceeded} {
		err := retryableAgentSessionError(cause)
		var retryable interface{ Retryable() bool }
		if !errors.As(err, &retryable) || !retryable.Retryable() || !strings.Contains(err.Error(), cause.Error()) {
			t.Fatalf("provider stream termination was not preserved as retryable: %v", err)
		}
	}
}

func TestAgentNativeFinalFailurePersistsOriginalFrameAndReportsDetail(t *testing.T) {
	for _, raw := range []string{
		`{"method":"error","params":{"threadId":"thread","turnId":"turn","willRetry":false,"error":{"message":"The model is unavailable","additionalDetails":"Choose an account-supported model","codexErrorInfo":{"serverOverloaded":{"httpStatusCode":503}}}}}`,
		`{"method":"turn/completed","params":{"threadId":"thread","turn":{"id":"turn","status":"failed","error":{"message":"The model is unavailable","additionalDetails":"Choose an account-supported model","codexErrorInfo":{"serverOverloaded":{"httpStatusCode":503}}}}}}`,
	} {
		result := codingagent.Result{ThreadID: "thread", TurnID: "turn"}
		var persisted json.RawMessage
		finished, err := consumeAgentFrame(decodeAgentTestFrame(t, raw), json.RawMessage(raw), &result, codingagent.Plan, map[string]string{}, func(event codingagent.NativeEvent) error {
			persisted = append(json.RawMessage(nil), event.Frame...)
			return nil
		})
		if err == nil || finished || result.Completed {
			t.Fatal("final native inference failure became accepted success")
		}
		var retryable interface{ Retryable() bool }
		if !errors.As(err, &retryable) || !retryable.Retryable() {
			t.Fatalf("provider capacity failure was not marked retryable: %v", err)
		}
		for _, detail := range []string{"The model is unavailable", "Choose an account-supported model", "httpStatusCode"} {
			if !strings.Contains(err.Error(), detail) {
				t.Fatalf("native failure lost actionable diagnostic %q: %v", detail, err)
			}
		}
		if !bytes.Equal(persisted, []byte(raw)) {
			t.Fatal("final native failure returned before its original receipt was persisted")
		}
	}
}

func TestAgentNativeErrorRejectsUnboundOrMissingRetryAuthority(t *testing.T) {
	for _, raw := range []string{
		`{"method":"error","params":{"threadId":"foreign","turnId":"turn","willRetry":true,"error":{"message":"Retrying"}}}`,
		`{"method":"error","params":{"threadId":"thread","turnId":"foreign","willRetry":true,"error":{"message":"Retrying"}}}`,
		`{"method":"error","params":{"threadId":"thread","turnId":"turn","error":{"message":"Retrying"}}}`,
		`{"method":"error","params":{"threadId":"thread","turnId":"turn","willRetry":null,"error":{"message":"Retrying"}}}`,
		`{"method":"error","params":{"threadId":"thread","turnId":"turn","willRetry":true,"error":null}}`,
	} {
		result := codingagent.Result{ThreadID: "thread", TurnID: "turn"}
		finished, err := collectAgentFrame(decodeAgentTestFrame(t, raw), json.RawMessage(raw), &result, codingagent.Execute)
		if err == nil || finished || result.Completed {
			t.Fatalf("unbound or malformed native retry notification accepted: %s", raw)
		}
	}
}

func TestAgentNativeFailurePersistenceErrorTakesPrecedence(t *testing.T) {
	raw := `{"method":"error","params":{"threadId":"thread","turnId":"turn","willRetry":false,"error":{"message":"Native failed"}}}`
	result := codingagent.Result{ThreadID: "thread", TurnID: "turn"}
	failure := errors.New("native receipt persistence failed")
	finished, err := consumeAgentFrame(decodeAgentTestFrame(t, raw), json.RawMessage(raw), &result, codingagent.Plan, map[string]string{}, func(codingagent.NativeEvent) error { return failure })
	if !errors.Is(err, failure) || finished || result.Completed || len(result.Evidence) != 0 {
		t.Fatal("native failure was processed without required durable evidence")
	}
}

func TestAgentNativeFailureDiagnosticIsBoundedAndValidUTF8(t *testing.T) {
	detail := &agentNativeError{Message: strings.Repeat("界", 8192) + "\x00"}
	message := nativeAgentFailure(detail).Error()
	if len(message) > 8192 || !utf8.ValidString(message) || strings.ContainsRune(message, '\x00') || !strings.HasSuffix(message, "...[truncated]") {
		t.Fatal("native failure escaped the existing diagnostic boundary")
	}
}
