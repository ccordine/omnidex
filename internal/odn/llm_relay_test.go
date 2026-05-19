package odn

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestRelayChecksumIsStable(t *testing.T) {
	first := RelayChecksum("  exact payload  ")
	second := RelayChecksum("exact payload")

	if first != second {
		t.Fatalf("checksums differ: %s vs %s", first, second)
	}
	if len(first) != 64 {
		t.Fatalf("checksum length = %d, want 64", len(first))
	}
}

func TestLLMRelayTwoRolesCommunicateWithExactPayload(t *testing.T) {
	payload := "FACT: command=date stdout=Mon May 18 12:53:03 EDT 2026"
	client, closeServer := fakeOllamaClient(t, []string{
		mustRelayJSON(t, "worker", "manager", payload),
	})
	defer closeServer()

	service := NewLLMRelayService(client).WithTimeout(5 * time.Second)
	hop, err := service.Send(context.Background(), "worker", "manager", payload)
	if err != nil {
		t.Fatal(err)
	}

	if hop.Message.Payload != payload {
		t.Fatalf("payload = %q, want %q", hop.Message.Payload, payload)
	}
	if hop.Message.Checksum != RelayChecksum(payload) {
		t.Fatalf("checksum = %q, want %q", hop.Message.Checksum, RelayChecksum(payload))
	}
}

func TestLLMRelayRejectsHallucinatedPayload(t *testing.T) {
	payload := "FACT: curl title=Example Domain"
	client, closeServer := fakeOllamaClient(t, []string{
		mustRelayJSON(t, "worker", "manager", "FACT: curl title=Different Domain"),
	})
	defer closeServer()

	service := NewLLMRelayService(client).WithTimeout(5 * time.Second)
	_, err := service.Send(context.Background(), "worker", "manager", payload)
	if err == nil {
		t.Fatal("expected relay payload mismatch error")
	}
	if !strings.Contains(err.Error(), "checksum mismatch") && !strings.Contains(err.Error(), "payload mismatch") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestTelephoneGameWorkerToManagerToManagerToFinalLLM(t *testing.T) {
	payload := "WORKER_RESULT: command=pwd stdout=/tmp/project acceptance=workspace_confirmed"
	client, closeServer := fakeOllamaClient(t, []string{
		mustRelayJSON(t, "worker", "manager", payload),
		mustRelayJSON(t, "manager", "manager_manager", payload),
		mustRelayJSON(t, "manager_manager", "final_llm", payload),
	})
	defer closeServer()

	service := NewLLMRelayService(client).WithTimeout(5 * time.Second)
	result, err := service.TelephoneGame(context.Background(), []string{"worker", "manager", "manager_manager", "final_llm"}, payload)
	if err != nil {
		t.Fatal(err)
	}

	if !result.Delivered {
		t.Fatal("telephone game did not mark delivery as complete")
	}
	if len(result.Hops) != 3 {
		t.Fatalf("hops = %d, want 3", len(result.Hops))
	}
	if result.Final.To != "final_llm" {
		t.Fatalf("final to = %q, want final_llm", result.Final.To)
	}
	if result.FinalPayload != payload {
		t.Fatalf("final payload = %q, want %q", result.FinalPayload, payload)
	}
}

func mustRelayJSON(t *testing.T, fromRole, toRole, payload string) string {
	t.Helper()
	blob, err := json.Marshal(RelayMessage{
		From:     fromRole,
		To:       toRole,
		Payload:  payload,
		Checksum: RelayChecksum(payload),
	})
	if err != nil {
		t.Fatal(err)
	}
	return string(blob)
}
