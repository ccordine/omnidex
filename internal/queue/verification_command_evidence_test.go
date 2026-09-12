package queue

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/model"
)

func TestNormalizeVerificationCommandEvidenceClassifiesTerminalReceipts(t *testing.T) {
	t.Parallel()
	zero, seven := 0, 7
	for _, fixture := range []struct {
		name        string
		exitCode    *int
		launchError string
		status      VerificationCommandStatus
	}{
		{name: "success", exitCode: &zero, status: VerificationCommandSucceeded},
		{name: "exit failure", exitCode: &seven, status: VerificationCommandExitFailed},
		{name: "launch failure", launchError: "executable was not found", status: VerificationCommandLaunchFailed},
	} {
		fixture := fixture
		t.Run(fixture.name, func(t *testing.T) {
			t.Parallel()
			record := verificationCommandFixture()
			record.ExitCode = fixture.exitCode
			record.LaunchError = fixture.launchError
			normalized, err := normalizeVerificationCommandEvidence(record)
			if err != nil {
				t.Fatal(err)
			}
			if normalized.Status != fixture.status || normalized.DurationNanos != 125000000 {
				t.Fatalf("normalized receipt=%#v", normalized)
			}
		})
	}
}

func TestNormalizeVerificationCommandEvidencePreservesNilAndEmptyStdin(t *testing.T) {
	t.Parallel()
	zero := 0
	without := verificationCommandFixture()
	without.ExitCode = &zero
	normalized, err := normalizeVerificationCommandEvidence(without)
	if err != nil {
		t.Fatal(err)
	}
	if normalized.StdinPresent || normalized.Stdin != nil {
		t.Fatalf("nil stdin gained evidence: %#v", normalized)
	}
	withEmpty := verificationCommandFixture()
	withEmpty.ExitCode = &zero
	withEmpty.Stdin = []byte{}
	normalized, err = normalizeVerificationCommandEvidence(withEmpty)
	if err != nil {
		t.Fatal(err)
	}
	if !normalized.StdinPresent || normalized.Stdin == nil || len(normalized.Stdin) != 0 {
		t.Fatalf("empty stdin identity was lost: %#v", normalized)
	}
}

func TestNormalizeVerificationCommandEvidenceMarksBoundedOverflowIncomplete(t *testing.T) {
	t.Parallel()
	record := verificationCommandFixture()
	record.ExitCode = nil
	record.LaunchError = "verification command output exceeded the immutable stream bound"
	record.StdoutComplete = false
	normalized, err := normalizeVerificationCommandEvidence(record)
	if err != nil {
		t.Fatal(err)
	}
	if normalized.Status != VerificationCommandLaunchFailed ||
		normalized.StdoutComplete || !normalized.StderrComplete {
		t.Fatalf("overflow receipt=%#v", normalized)
	}
}

func TestNormalizeVerificationCommandEvidenceClassifiesHostObservationFailures(t *testing.T) {
	t.Parallel()
	zero := 0
	host := verificationCommandFixture()
	host.Phase = VerificationHostFinal
	host.ExitCode = &zero
	normalized, err := normalizeVerificationCommandEvidence(host)
	if err != nil {
		t.Fatal(err)
	}
	if normalized.Status != VerificationCommandSucceeded {
		t.Fatalf("host verification requires more than its execution result: %#v", normalized)
	}

	observation := verificationCommandFixture()
	observation.Phase = VerificationHostFinal
	observation.ExitCode = &zero
	observation.ObservationError = "host working directory was removed"
	normalized, err = normalizeVerificationCommandEvidence(observation)
	if err != nil {
		t.Fatal(err)
	}
	if normalized.Status != VerificationCommandObservationFailed {
		t.Fatalf("observation failure receipt=%#v", normalized)
	}
}

func TestNormalizeVerificationCommandEvidenceRejectsInexactAuthority(t *testing.T) {
	t.Parallel()
	zero := 0
	base := verificationCommandFixture()
	base.ExitCode = &zero
	for name, mutate := range map[string]func(*VerificationCommandEvidence){
		"relative cwd": func(record *VerificationCommandEvidence) { record.WorkingDirectory = "project" },
		"unsorted environment": func(record *VerificationCommandEvidence) {
			record.Environment = []string{"Z=1", "A=2"}
		},
		"shell string": func(record *VerificationCommandEvidence) { record.Argv = []string{""} },
		"dual result":  func(record *VerificationCommandEvidence) { record.LaunchError = "failed" },
		"oversized stdout": func(record *VerificationCommandEvidence) {
			record.Stdout = []byte(strings.Repeat("x", maxVerificationCommandStreamBytes+1))
		},
	} {
		name, mutate := name, mutate
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			record := base
			mutate(&record)
			if _, err := normalizeVerificationCommandEvidence(record); err == nil {
				t.Fatalf("accepted invalid verification receipt: %#v", record)
			}
		})
	}
}

func TestVerificationCommandEvidenceContainsResultsWithoutHashReceipts(t *testing.T) {
	record := verificationCommandFixture()
	zero := 0
	record.ExitCode = &zero
	normalized, err := normalizeVerificationCommandEvidence(record)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(normalized)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "sha256") || strings.Contains(string(raw), "digest") {
		t.Fatalf("command result carries redundant hash receipts: %s", raw)
	}
}

func verificationCommandFixture() VerificationCommandEvidence {
	started := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)
	return VerificationCommandEvidence{
		Authority: model.StepAttemptAuthority{
			JobID: 1, Generation: 1, StepID: 2, Attempt: 1, WorkerID: "fixture-worker",
		},
		Phase: VerificationIsolatedTask, Ordinal: 1,
		Argv: []string{"go", "test", "./..."}, Environment: []string{"GOCACHE=/tmp/cache"},
		WorkingDirectory: "/tmp/project", StartedAt: started,
		FinishedAt: started.Add(125 * time.Millisecond),
		Stdout:     []byte("ok\n"), StdoutComplete: true,
		Stderr: []byte{}, StderrComplete: true,
	}
}
