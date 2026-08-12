package worker

import (
	"context"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/queue"
)

type inexactPreparedProvider struct{}

func (inexactPreparedProvider) Generate(context.Context, string, string) (string, error) {
	return "", nil
}

func (inexactPreparedProvider) PrepareContextModel(
	context.Context,
	string,
	string,
) (llm.PreparedModel, error) {
	return llm.PreparedModel{}, nil
}

func (inexactPreparedProvider) GeneratePrepared(
	context.Context,
	llm.PreparedModel,
) (string, error) {
	return "", nil
}

func (inexactPreparedProvider) CleanupPreparedModel(llm.PreparedModel) {}

func (inexactPreparedProvider) Embedding(context.Context, string) ([]float64, error) {
	return nil, nil
}

func TestNewRejectsProviderWithoutExactPreparedContract(t *testing.T) {
	service, err := New(&queue.Repository{}, inexactPreparedProvider{}, nil, validWorkerOptions())
	if err == nil || !strings.Contains(err.Error(), "does not enforce the exact prepared contract") {
		t.Fatalf("inexact prepared provider service=%v error=%v", service, err)
	}
	if service != nil {
		t.Fatal("inexact prepared provider produced a worker service")
	}
}
