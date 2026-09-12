package worker

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/queue"
)

func TestVerificationCommandsUseProcessResultsWithoutWorkspaceHashGates(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OMNI_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OMNI_TEST_DATABASE_URL is required for command execution coverage")
	}
	_, repository := freshWorkerEvidenceRepository(t, databaseURL)
	ctx := context.Background()
	root := t.TempDir()
	job, err := repository.EnqueueCodingJob(ctx, "verify command results", root)
	if err != nil {
		t.Fatal(err)
	}
	claim, err := repository.ClaimNextStep(ctx, "verification-worker")
	if err != nil || claim == nil || claim.Job.ID != job.ID {
		t.Fatalf("claim=%#v err=%v", claim, err)
	}
	session := &directCodingSession{
		root:    root,
		runtime: &nativeRuntimeV3{svc: &Service{repo: repository}, ctx: ctx, claim: claim},
	}
	for _, fixture := range []struct {
		name, script, path, content string
	}{
		{"build output", "printf built > application.bin", "application.bin", "built"},
		{"generated report", "mkdir reports; printf passed > reports/result.txt", "reports/result.txt", "passed"},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			_, err := session.runRecordedVerificationCommand(root, queue.VerificationIsolatedFinal,
				directCodingVerificationCommand{Argv: []string{"sh", "-c", fixture.script}, Timeout: time.Second})
			if err != nil {
				t.Fatal(err)
			}
			content, err := os.ReadFile(filepath.Join(root, fixture.path))
			if err != nil || string(content) != fixture.content {
				t.Fatalf("command output=%q err=%v", content, err)
			}
		})
	}
	_, err = session.runRecordedVerificationCommand(root, queue.VerificationIsolatedFinal,
		directCodingVerificationCommand{Argv: []string{"sh", "-c", "exit 7"}, Timeout: time.Second})
	if err == nil || !strings.Contains(err.Error(), "exited 7") {
		t.Fatalf("process failure was hidden: %v", err)
	}
	_, err = session.runRecordedVerificationCommand(root, queue.VerificationIsolatedFinal,
		directCodingVerificationCommand{Argv: []string{"sh", "-c", "printf verified"}, Timeout: time.Second})
	if err != nil {
		t.Fatalf("later command blocked by historical failure: %v", err)
	}
	records, err := repository.ListVerificationCommandEvidenceForJob(ctx, job.ID, 0, 10)
	if err != nil || len(records) != 4 {
		t.Fatalf("command history=%#v err=%v", records, err)
	}
	for i, status := range []queue.VerificationCommandStatus{
		queue.VerificationCommandSucceeded, queue.VerificationCommandSucceeded,
		queue.VerificationCommandExitFailed, queue.VerificationCommandSucceeded,
	} {
		if records[i].Status != status {
			t.Fatalf("command %d status=%s; want %s", i, records[i].Status, status)
		}
	}
}
