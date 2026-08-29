package worker

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
)

func desiredStateProductService(
	t *testing.T,
	repository *queue.Repository,
	provider *desiredStateProductProvider,
	root string,
) *Service {
	t.Helper()
	opts := validWorkerOptions()
	opts.WorkerCount = 1
	opts.PollInterval = 10 * time.Millisecond
	opts.Models.Stations = validStationModels()
	opts.Workspace.Root = root
	opts.Logger = log.New(io.Discard, "", 0)
	service, err := New(repository, provider, desiredStateProductEmbedding{}, nil, opts)
	if err != nil {
		t.Fatal(err)
	}
	return service
}

type desiredStateProductEmbedding struct{}

func (desiredStateProductEmbedding) Embedding(context.Context, string) ([]float64, error) {
	vector := make([]float64, model.MemoryEmbeddingDimensions)
	vector[0] = 1
	return vector, nil
}

func desiredStateProductRepository(t *testing.T, obsolete bool) string {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"go.mod":        "module example.com/desiredstateproduct\n\ngo 1.24\n",
		"value.go":      "package desiredstateproduct\n\nfunc Value() int { return 1 }\n",
		"value_test.go": "package desiredstateproduct\n\nimport \"testing\"\n\nfunc TestValue(t *testing.T) {\n\tif Value() != 1 { t.Fatal(\"unexpected value\") }\n}\n",
	}
	if obsolete {
		files["alpha.go"] = "package desiredstateproduct\n\nfunc Alpha() int { return 8 }\n"
		files["obsolete.go"] = "package desiredstateproduct\n\nfunc Obsolete() int { return 9 }\n"
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, args := range [][]string{
		{"init"}, {"config", "user.email", "product-vertical@example.test"},
		{"config", "user.name", "Product Vertical"}, {"add", "."},
		{"commit", "-m", "exact source"},
	} {
		command := exec.Command("git", append([]string{"-C", root}, args...)...)
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, output)
		}
	}
	return root
}

func assertDesiredStateProductFilesystem(t *testing.T, root string, test desiredStateProductCase) {
	t.Helper()
	target := filepath.Join(root, test.target)
	if !test.present {
		if _, err := os.Stat(target); !os.IsNotExist(err) {
			t.Fatalf("deleted authoritative artifact stat error=%v", err)
		}
		return
	}
	raw, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != "package desiredstateproduct\n\nfunc Added() int {\n\treturn 2\n}\n" {
		t.Fatalf("code-owned created artifact=%q", raw)
	}
}

func assertDesiredStateProductCompletion(
	t *testing.T,
	repository *queue.Repository,
	channel model.Channel,
	job model.Job,
	test desiredStateProductCase,
) string {
	t.Helper()
	details, err := repository.CurrentJobDetails(t.Context(), job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if details.Job.ID != job.ID || details.Job.Status != model.JobStatusCompleted ||
		len(details.Steps) != 1 || details.Steps[0].JobID != job.ID ||
		details.Steps[0].Status != model.StepStatusCompleted {
		t.Fatalf("same-job completion failed: %+v", details)
	}
	page, err := repository.ListChannelMessages(t.Context(), channel.ID, 10, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Messages) != 2 || page.Messages[0].Role != model.ChannelMessageRoleUser ||
		page.Messages[0].Content != test.instruction ||
		page.Messages[1].Role != model.ChannelMessageRoleAssistant {
		t.Fatalf("ordinary channel transcript=%+v", page.Messages)
	}
	return page.Messages[1].Content
}

func assertDesiredStateProductSummary(t *testing.T, summary string, test desiredStateProductCase) {
	t.Helper()
	created, deleted, delta := "", test.target, -1
	if test.present {
		created, deleted, delta = test.target, "", 1
	}
	want := []string{
		"Verified desired repository state:", "deterministic_operations=8",
		fmt.Sprintf("model_calls_total=%d", len(test.wantKinds)),
		fmt.Sprintf("semantic_gap_calls=%d", len(test.wantKinds)-test.wantGenerationCalls),
		fmt.Sprintf("declaration_generation_calls=%d", test.wantGenerationCalls),
		"declaration_correction_calls=0",
		"model_selected_mutation_operations=0", "model_visible_target_paths=0",
		"created_files=[" + created + "]", "deleted_files=[" + deleted + "]",
		"verification_commands=[go test -json -count=1 -run ^$ .;go test -json -count=1 ./...]",
		"verification_command_executions=6",
		fmt.Sprintf("inventory_delta=%d", delta),
	}
	for _, fragment := range want {
		if !strings.Contains(summary, fragment) {
			t.Fatalf("assistant completion omitted %q: %s", fragment, summary)
		}
	}
}
