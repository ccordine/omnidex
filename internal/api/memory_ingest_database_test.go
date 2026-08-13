package api

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
)

func TestPostgresMemoryBatchEmbeddingFailureWritesNothing(t *testing.T) {
	pool := openIsolatedAPIMigrationPool(t)
	repository := queue.New(pool)
	if err := repository.EnsureSchema(t.Context(), loadAPITestMigrationBundle(t)); err != nil {
		t.Fatal(err)
	}
	channel, err := repository.CreateChannel(t.Context(), model.Channel{
		ID: "api-memory-batch", Scope: model.ChannelScopeUser, Name: "API memory batch",
		WorkspaceRoot: "/srv/workspaces/api-memory-batch",
	})
	if err != nil {
		t.Fatal(err)
	}
	client := &countingEmbeddingClient{embedding: make([]float64, 768), failAt: 2}
	server := &Server{repo: repository, embeddingClient: client}
	body := fmt.Sprintf(`{"memories":[`+
		`{"scope":{"project_id":%d,"channel_id":%q},"source":"batch:first","kind":"reference","content":"batch first","tags":[],"categories":["research"]},`+
		`{"scope":{"project_id":%d,"channel_id":%q},"source":"batch:second","kind":"reference","content":"batch second","tags":[],"categories":["research"]}`+
		`]}`, channel.ProjectID, channel.ID, channel.ProjectID, channel.ID)
	request := httptest.NewRequest(http.MethodPost, "/v1/memory/batch", bytes.NewBufferString(body))
	response := httptest.NewRecorder()
	server.addMemoryBatch(response, request)
	if response.Code != http.StatusBadGateway || !strings.Contains(response.Body.String(), "injected embedding failure") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var count int
	if err := pool.QueryRow(t.Context(), `
		SELECT COUNT(*) FROM memory_chunks WHERE source IN ('batch:first','batch:second')
	`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("failed embedding batch persisted %d memory chunks", count)
	}
}
