package worker

import (
	"context"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/llm"
)

type advisoryWorkerLLM struct {
	advisoryCalls int
	normalCalls   int
	response      llm.AdvisoryResponse
	prepared      llm.PreparedModel
}

func (*advisoryWorkerLLM) Generate(context.Context, string, string) (string, error) {
	return "", nil
}

func (*advisoryWorkerLLM) PrepareContextModel(context.Context, string, string) (llm.PreparedModel, error) {
	return llm.PreparedModel{}, nil
}

func (client *advisoryWorkerLLM) GeneratePrepared(_ context.Context, _ llm.PreparedModel) (string, error) {
	client.normalCalls++
	return "normal fallback", nil
}

func (*advisoryWorkerLLM) CleanupPreparedModel(llm.PreparedModel) {}

func (*advisoryWorkerLLM) Embedding(context.Context, string) ([]float64, error) {
	return nil, nil
}

func (client *advisoryWorkerLLM) GeneratePreparedAdvisory(_ context.Context, prepared llm.PreparedModel) (llm.AdvisoryResponse, error) {
	client.advisoryCalls++
	client.prepared = prepared
	return client.response, nil
}

func TestGeneratePreparedResponseRoutesNativeThinkingOnlyToAdvisoryTransport(t *testing.T) {
	client := &advisoryWorkerLLM{response: llm.AdvisoryResponse{Thinking: "exact thought", Content: "exact memo"}}
	service := &Service{llm: client}
	prepared := llm.PreparedModel{ThinkingEnabled: true}

	response, err := service.generatePreparedResponseWithProgress(context.Background(), 7, "portable_advisory_worker", prepared)
	if err != nil {
		t.Fatal(err)
	}
	if response != client.response || client.advisoryCalls != 1 || client.normalCalls != 0 {
		t.Fatalf("response=%#v advisory_calls=%d normal_calls=%d", response, client.advisoryCalls, client.normalCalls)
	}
	if !client.prepared.ThinkingEnabled {
		t.Fatal("advisory transport received thinking disabled")
	}
}

func TestGeneratePreparedResponseRejectsUnsupportedAdvisoryProviderWithoutFallback(t *testing.T) {
	client := &recordingWorkerLLM{}
	service := &Service{llm: client}

	_, err := service.generatePreparedResponseWithProgress(context.Background(), 7, "portable_advisory_worker", llm.PreparedModel{ThinkingEnabled: true})
	if err == nil || !strings.Contains(err.Error(), "does not implement native advisory generation") {
		t.Fatalf("error=%v", err)
	}
}

func TestAdvisoryEvidencePreservesBothProviderFieldsButNormalEvidenceRemainsRaw(t *testing.T) {
	response := llm.AdvisoryResponse{Thinking: "\nexact thought\n", Content: "\nexact memo\n"}
	raw, err := llmResponseEvidence(response, true)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := llm.DecodeAdvisoryEvidence(raw)
	if err != nil {
		t.Fatal(err)
	}
	if decoded != response {
		t.Fatalf("decoded=%#v want %#v", decoded, response)
	}
	normal, err := llmResponseEvidence(response, false)
	if err != nil {
		t.Fatal(err)
	}
	if normal != response.Content {
		t.Fatalf("normal evidence=%q want exact content %q", normal, response.Content)
	}
}
