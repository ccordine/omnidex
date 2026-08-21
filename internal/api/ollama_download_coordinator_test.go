package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/ollama"
	"github.com/gryph/omnidex/internal/queue"
)

type downloadStoreProbe struct {
	mu        sync.Mutex
	item      queue.OllamaModelDownload
	progress  []ollama.PullProgress
	completed chan queue.OllamaModelDownload
}

func newDownloadStoreProbe() *downloadStoreProbe {
	return &downloadStoreProbe{completed: make(chan queue.OllamaModelDownload, 1)}
}

func (store *downloadStoreProbe) CreateOllamaModelDownload(
	_ context.Context,
	model string,
) (queue.OllamaModelDownload, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	now := time.Now().UTC()
	store.item = queue.OllamaModelDownload{
		ID: "omd_0123456789abcdef0123456789abcdef", Model: model,
		State: queue.OllamaModelDownloadQueued, Status: "Queued",
		CreatedAt: now, UpdatedAt: now,
	}
	return store.item, nil
}

func (store *downloadStoreProbe) StartOllamaModelDownload(
	_ context.Context,
	_ string,
) (queue.OllamaModelDownload, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	now := time.Now().UTC()
	store.item.State, store.item.Status = queue.OllamaModelDownloadRunning, "Connecting to Ollama"
	store.item.StartedAt, store.item.UpdatedAt = &now, now
	return store.item, nil
}

func (store *downloadStoreProbe) RecordOllamaModelDownloadProgress(
	_ context.Context,
	_ string,
	progress ollama.PullProgress,
) (queue.OllamaModelDownload, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.progress = append(store.progress, progress)
	store.item.Status, store.item.Digest = progress.Status, progress.Digest
	store.item.TotalBytes, store.item.CompletedBytes = progress.Total, progress.Completed
	store.item.UpdatedAt = time.Now().UTC()
	return store.item, nil
}

func (store *downloadStoreProbe) CompleteOllamaModelDownload(
	_ context.Context,
	_ string,
) (queue.OllamaModelDownload, error) {
	store.mu.Lock()
	now := time.Now().UTC()
	store.item.State, store.item.Status = queue.OllamaModelDownloadCompleted, "Installed"
	store.item.FinishedAt, store.item.UpdatedAt = &now, now
	item := store.item
	store.mu.Unlock()
	store.completed <- item
	return item, nil
}

func (store *downloadStoreProbe) FailOllamaModelDownload(
	_ context.Context,
	_ string,
	reason string,
) (queue.OllamaModelDownload, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	now := time.Now().UTC()
	store.item.State, store.item.Status, store.item.Error = queue.OllamaModelDownloadFailed, "Failed", reason
	store.item.FinishedAt, store.item.UpdatedAt = &now, now
	return store.item, nil
}

func (store *downloadStoreProbe) ListOllamaModelDownloads(
	context.Context,
	int,
	int,
) (queue.OllamaModelDownloadPage, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	return queue.OllamaModelDownloadPage{Items: []queue.OllamaModelDownload{store.item}}, nil
}

func (*downloadStoreProbe) ListActiveOllamaModelDownloads(context.Context) ([]queue.OllamaModelDownload, error) {
	return []queue.OllamaModelDownload{}, nil
}

type downloadLifecycleProbe struct {
	started chan struct{}
	release chan struct{}
}

func (probe *downloadLifecycleProbe) PullModelProgress(
	ctx context.Context,
	_ string,
	observe func(ollama.PullProgress) error,
) error {
	close(probe.started)
	select {
	case <-probe.release:
	case <-ctx.Done():
		return ctx.Err()
	}
	for _, progress := range []ollama.PullProgress{
		{Status: "pulling layer", Digest: "sha256:one", Total: 100, Completed: 50},
		{Status: "success"},
	} {
		if err := observe(progress); err != nil {
			return err
		}
	}
	return nil
}

func (*downloadLifecycleProbe) HasModel(context.Context, string) (bool, error) { return true, nil }
func (*downloadLifecycleProbe) DeleteModel(context.Context, string) error      { return nil }

func TestOllamaPullReturnsBeforeDurableBackgroundDownloadCompletes(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	store := newDownloadStoreProbe()
	lifecycle := &downloadLifecycleProbe{started: make(chan struct{}), release: make(chan struct{})}
	server := NewServerWithOptions(nil, nil, ServerOptions{
		LifecycleContext: ctx, OllamaDownloads: store, OllamaModelLifecycle: lifecycle,
	})
	request := httptest.NewRequest(
		http.MethodPost, "/v1/ollama/models",
		bytes.NewBufferString(`{"model":"story-model:latest"}`),
	)
	response := httptest.NewRecorder()
	server.handleOllamaModels(response, request)
	if response.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var payload struct {
		Download queue.OllamaModelDownload `json:"download"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Download.State != queue.OllamaModelDownloadQueued {
		t.Fatalf("response download=%+v", payload.Download)
	}
	select {
	case <-lifecycle.started:
	case <-time.After(time.Second):
		t.Fatal("background download did not start")
	}
	close(lifecycle.release)
	select {
	case completed := <-store.completed:
		if completed.State != queue.OllamaModelDownloadCompleted || completed.Model != "story-model:latest" {
			t.Fatalf("completed=%+v", completed)
		}
	case <-time.After(time.Second):
		t.Fatal("background download did not complete")
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if len(store.progress) != 2 || store.progress[1].Status != "success" {
		t.Fatalf("progress=%+v", store.progress)
	}
}
