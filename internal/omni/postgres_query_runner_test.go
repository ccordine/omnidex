package omni

import (
	"context"
	"strings"
	"testing"
)

type queryOnlyRunner struct{}

func (queryOnlyRunner) Query(
	context.Context,
	string,
	...any,
) ([]MemorySQLRow, error) {
	return nil, nil
}

func TestMemorySQLRunnerExposesReadOnlyQuerySurface(t *testing.T) {
	var _ MemorySQLRunner = queryOnlyRunner{}
	for _, runner := range []*PgxMemoryRunner{nil, NewPgxMemoryRunner(nil)} {
		if _, err := runner.Query(context.Background(), "SELECT 1"); err == nil ||
			!strings.Contains(err.Error(), "requires a pool and context") {
			t.Fatalf("Query error=%v, want missing-authority rejection", err)
		}
	}
}
