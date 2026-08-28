package worker

import (
	"context"
	"testing"

	goadapter "github.com/gryph/omnidex/internal/repository/adapters/golang"
	repositoryindex "github.com/gryph/omnidex/internal/repository/indexing"
)

func captureGoRepositoryIndexForTest(
	t *testing.T,
	ctx context.Context,
	indexer *repositoryindex.Service,
	projectID int64,
	root string,
) repositoryindex.Result {
	t.Helper()
	result, err := indexer.Capture(ctx, projectID, root)
	if err != nil {
		t.Fatal(err)
	}
	analysis, err := indexer.Analyze(
		ctx, projectID, result.Snapshot, goadapter.AdapterName,
	)
	if err != nil {
		t.Fatal(err)
	}
	result.Analyses = append(result.Analyses, analysis)
	return result
}
