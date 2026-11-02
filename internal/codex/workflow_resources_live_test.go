package codex

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/workflow"
)

// A functional native-adapter qualification in the authoritative checkout, not
// an autonomy claim or a replacement ledger/database/environment.
func TestInstalledWorkflowResourceLifecycle(t *testing.T) {
	if os.Getenv("OMNI_TEST_WORKFLOW_RESOURCES") != "1" {
		t.Skip("native workflow qualification not selected")
	}
	client, model := installedAgentClient(t)
	root := client.directory
	rel := "bin/planning-roadmap-pilot-20261026/workflow-resource-pilot-20261031"
	directory := filepath.Join(root, rel)
	if _, err := os.Lstat(directory); !os.IsNotExist(err) {
		t.Fatal("pilot artifacts already exist; preserve them", err)
	}
	tags := []string{"omnidex-workflow-pilot-20261031:1", "omnidex-workflow-pilot-20261031:2", "omnidex-workflow-pilot-20261031:3"}
	for _, tag := range tags {
		if exec.Command("docker", "image", "inspect", tag).Run() == nil {
			t.Fatal("pilot tag already exists; preserve it", tag)
		}
	}
	defer func() {
		for _, tag := range tags {
			_ = exec.Command("docker", "image", "rm", tag).Run()
		}
		_ = os.RemoveAll(directory)
	}()
	job := int64(91020261031)
	started := time.Now().UTC().Format(time.RFC3339Nano)
	task := &codingagent.Task{ID: "workflow-resource-pilot", Goal: fmt.Sprintf("In this existing checkout, create a tiny Go command under %s that prints workflow-resource-ready. It must also support --socket-check: open and close a Unix socket in the application temporary directory, remove that socket, and print workflow-resource-ready after success. Verify that option using the workflow environment. Also add a Go test in that same directory that opens and closes a Unix socket inside testing.T.TempDir, and run it using the ordinary go CLI. Build and run three binary outputs named resource-1, resource-2 and resource-3 in that directory using the ordinary go CLI. Build the same tiny program into three local Docker image tags %s, %s and %s using the ordinary docker CLI, and save each image into archive-1.tar, archive-2.tar and archive-3.tar in that directory. Use only that directory for generated files and build context. Reuse the existing Docker engine. Do not start any container, create another environment, change application or framework source, modify configuration, or delete the generated outputs. These generated outputs are the complete fixture and will be inspected and cleaned by the caller. Report observed results.", rel, tags[0], tags[1], tags[2])}
	result, err := client.Run(t.Context(), codingagent.Assignment{JobID: job, Generation: 1, Root: root, Role: codingagent.Execute, Route: codingagent.Route{Provider: "codex", Model: model, Effort: "low"}, Task: task}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.ThreadID == "" || result.TurnID == "" {
		t.Fatal("native execution lacks actual lifecycle receipt")
	}
	for i := 1; i <= 3; i++ {
		output, err := exec.Command(filepath.Join(directory, fmt.Sprintf("resource-%d", i))).Output()
		if err != nil || string(output) != "workflow-resource-ready\n" {
			t.Fatalf("actual generated command %d failed: %s %v", i, output, err)
		}
	}
	managed, err := client.workflowScope(root, job, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	environment, err := managed.Environment(os.Environ(), filepath.Join(root, "bin", "omnidex"))
	if err != nil {
		t.Fatal(err)
	}
	check := exec.Command(filepath.Join(directory, "resource-3"), "--socket-check")
	check.Env = environment
	output, err := check.Output()
	if err != nil || string(output) != "workflow-resource-ready\n" {
		t.Fatalf("application Unix-socket compatibility failed: %s %v", output, err)
	}
	fixture := exec.Command("go", "test", "-count=1", "./"+rel)
	fixture.Dir, fixture.Env = root, environment
	if output, err := fixture.CombinedOutput(); err != nil {
		t.Fatalf("Go test fixture Unix-socket compatibility failed: %s %v", output, err)
	}
	if err := client.CleanupWorkflow(context.Background(), root, job, []string{task.ID}); err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 3; i++ {
		for _, name := range []string{fmt.Sprintf("resource-%d", i), fmt.Sprintf("archive-%d.tar", i)} {
			_, err := os.Stat(filepath.Join(directory, name))
			if i == 1 && !os.IsNotExist(err) {
				t.Fatal("obsolete output remains", name)
			}
			if i > 1 && err != nil {
				t.Fatal("current/rollback output removed", name, err)
			}
		}
		data, err := exec.Command("docker", "image", "inspect", tags[i-1]).Output()
		if i == 1 && err == nil {
			t.Fatal("obsolete image tag remains")
		}
		if i > 1 {
			if err != nil {
				t.Fatal("retained image missing", err)
			}
			var image []struct {
				Config struct{ Labels map[string]string }
			}
			if json.Unmarshal(data, &image) != nil || len(image) != 1 || image[0].Config.Labels["io.omnidex.workflow"] == "" {
				t.Fatal("real image lacks code-owned resource label")
			}
		}
	}
	scope, err := workflow.Open(root, root)
	if err != nil {
		t.Fatal(err)
	}
	scope.Job = job
	if err := scope.Cleanup(context.Background()); err != nil {
		t.Fatal("recovery cleanup is not idempotent", err)
	}
	events, err := exec.Command("docker", "events", "--since", started, "--until", time.Now().UTC().Format(time.RFC3339Nano), "--filter", "type=container", "--filter", fmt.Sprintf("label=io.omnidex.job=%d", job)).Output()
	if err != nil || len(events) != 0 {
		t.Fatal("pilot created containers instead of reusing the existing environment", err)
	}
	t.Logf("Real native thread=%s turn=%s: three builds/exports; obsolete binary, archive and tag removed automatically; current and rollback retained; no new containers", result.ThreadID, result.TurnID)
}
