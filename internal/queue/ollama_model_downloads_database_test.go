package queue

import (
	"errors"
	"testing"

	"github.com/gryph/omnidex/internal/ollama"
)

func TestOllamaModelDownloadLifecycleIsDurableAndSingleActive(t *testing.T) {
	ctx := t.Context()
	pool := openIsolatedDatabasePool(t)
	repository := New(pool)
	if err := repository.ResetDatabase(ctx, loadCurrentDatabaseSetup(t)); err != nil {
		t.Fatal(err)
	}
	created, err := repository.CreateOllamaModelDownload(ctx, "dolphin3:8b")
	if err != nil {
		t.Fatal(err)
	}
	if created.State != OllamaModelDownloadQueued || created.Model != "dolphin3:8b" {
		t.Fatalf("created=%+v", created)
	}
	duplicate, err := repository.CreateOllamaModelDownload(ctx, "dolphin3:8b")
	if !errors.Is(err, ErrOllamaModelDownloadActive) || duplicate.ID != created.ID {
		t.Fatalf("duplicate=%+v err=%v", duplicate, err)
	}
	running, err := repository.StartOllamaModelDownload(ctx, created.ID)
	if err != nil || running.State != OllamaModelDownloadRunning || running.StartedAt == nil {
		t.Fatalf("running=%+v err=%v", running, err)
	}
	progress, err := repository.RecordOllamaModelDownloadProgress(ctx, created.ID, ollama.PullProgress{
		Status: "downloading", Digest: "sha256:abc", Total: 100, Completed: 25,
	})
	if err != nil || progress.CompletedBytes != 25 || progress.TotalBytes != 100 {
		t.Fatalf("progress=%+v err=%v", progress, err)
	}
	progress, err = repository.RecordOllamaModelDownloadProgress(ctx, created.ID, ollama.PullProgress{
		Status: "downloading", Digest: "sha256:abc", Total: 100, Completed: 100,
	})
	if err != nil {
		t.Fatal(err)
	}
	terminal, err := repository.RecordOllamaModelDownloadProgress(ctx, created.ID, ollama.PullProgress{
		Status: "success",
	})
	if err != nil || terminal.Digest != "sha256:abc" || terminal.TotalBytes != 100 || terminal.CompletedBytes != 100 {
		t.Fatalf("terminal progress erased accepted byte evidence: terminal=%+v err=%v", terminal, err)
	}
	completed, err := repository.CompleteOllamaModelDownload(ctx, created.ID)
	if err != nil || completed.State != OllamaModelDownloadCompleted || completed.FinishedAt == nil {
		t.Fatalf("completed=%+v err=%v", completed, err)
	}
	if completed.Digest != "sha256:abc" || completed.TotalBytes != 100 || completed.CompletedBytes != 100 {
		t.Fatalf("completion erased accepted byte evidence: %+v", completed)
	}
	if _, err := repository.FailOllamaModelDownload(ctx, created.ID, "late failure"); err == nil {
		t.Fatal("expected terminal download mutation to fail")
	}
	next, err := repository.CreateOllamaModelDownload(ctx, "dolphin3:8b")
	if err != nil || next.ID == created.ID {
		t.Fatalf("next=%+v err=%v", next, err)
	}
	failed, err := repository.FailOllamaModelDownload(ctx, next.ID, "provider unavailable")
	if err != nil || failed.State != OllamaModelDownloadFailed || failed.Error == "" {
		t.Fatalf("failed=%+v err=%v", failed, err)
	}
	page, err := repository.ListOllamaModelDownloads(ctx, 1, 0)
	if err != nil || len(page.Items) != 1 || !page.HasMore {
		t.Fatalf("page=%+v err=%v", page, err)
	}
}
