package worker

import (
	"context"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/queue"
)

type inexactCognitionProvider struct{}

func (inexactCognitionProvider) Generate(context.Context, string, string) (string, error) {
	return "", nil
}

func (inexactCognitionProvider) PrepareContextModel(
	context.Context,
	string,
	string,
) (llm.PreparedModel, error) {
	return llm.PreparedModel{}, nil
}

func (inexactCognitionProvider) GeneratePrepared(
	context.Context,
	llm.PreparedModel,
) (string, error) {
	return "", nil
}

func (inexactCognitionProvider) CleanupPreparedModel(llm.PreparedModel) {}

func (inexactCognitionProvider) Embedding(context.Context, string) ([]float64, error) {
	return nil, nil
}

func TestNewRejectsProviderWithoutExactPreparedCognitionContract(t *testing.T) {
	service, err := New(&queue.Repository{}, inexactCognitionProvider{}, nil, validWorkerOptions())
	if err == nil || !strings.Contains(err.Error(), "does not enforce the exact prepared contract") {
		t.Fatalf("inexact cognition provider service=%v error=%v", service, err)
	}
	if service != nil {
		t.Fatal("inexact cognition provider produced a worker service")
	}
}
