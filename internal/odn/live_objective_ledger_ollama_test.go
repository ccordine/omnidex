package odn

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLiveOllamaBuildsCalculatorAppWithObjectiveLedger(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live objective-ledger calculator test in short mode")
	}
	skipUnlessLiveOllamaEnabled(t)
	requireNodeAndNPM(t)

	client := testOllamaClient(t)
	client.Client.Timeout = 5 * time.Minute

	appDir := t.TempDir()
	prompt := "Build a test calculator web app in the active directory with recyclrjs, npm, and Tailwind CSS. Create package.json and index.html. Do not install packages or contact npm registry; use npm init or write package.json locally. Use Tailwind from the CDN. Include visible calculator UI and calculator logic with display, operands, and operators. Include recyclrjs in package.json or the source so the requested library is accounted for. Verify the files with shell commands before done."
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	events := []StructuredCommandEvent{}

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	result, err := runStructuredCommandDecisionWithConfig(
		ctx,
		prompt,
		nil,
		client,
		stdout,
		stderr,
		func(evt StructuredCommandEvent) {
			events = append(events, evt)
		},
		nil,
		structuredCommandDecisionRunConfig{
			CurrentWorkingDirectory: appDir,
			PromptInterpreter:       NewOllamaPromptInterpreter(client),
			ContextSummarizer:       NewOllamaContextSummarizer(client),
			ShellSpecialist:         NewOllamaShellCommandSpecialist(client),
		},
	)
	if err != nil {
		if isOllamaRunnerStoppedError(err) || isLiveModelTimeoutError(err) {
			t.Skipf("live objective-ledger calculator model run unavailable: %v", err)
		}
		t.Fatalf("live calculator app build failed: %v\ncommand=%q\nanswer=%q\nstdout=%s\nstderr=%s\nobservations=%#v\nevents=%#v",
			err, result.Command, result.Answer, stdout.String(), stderr.String(), result.Observations, events)
	}

	indexPath := filepath.Join(appDir, "index.html")
	packagePath := filepath.Join(appDir, "package.json")
	indexBytes, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatalf("expected index.html in active dir: %v\ncommand=%q\nstdout=%s\nstderr=%s", err, result.Command, stdout.String(), stderr.String())
	}
	packageBytes, err := os.ReadFile(packagePath)
	if err != nil {
		t.Fatalf("expected package.json in active dir: %v\ncommand=%q\nstdout=%s\nstderr=%s", err, result.Command, stdout.String(), stderr.String())
	}

	evidence := strings.ToLower(strings.Join([]string{
		string(indexBytes),
		string(packageBytes),
		stdout.String(),
		stderr.String(),
		result.Answer,
	}, "\n"))
	for _, want := range []string{"calculator", "tailwind", "recyclr", "package.json", "index.html"} {
		if !strings.Contains(evidence, want) {
			t.Fatalf("live calculator evidence missing %q\ncommand=%q\nanswer=%q\nstdout=%s\nstderr=%s\nindex=%s\npackage=%s",
				want, result.Command, result.Answer, stdout.String(), stderr.String(), string(indexBytes), string(packageBytes))
		}
	}
	if len(result.ObjectiveLedger) == 0 {
		t.Fatalf("live planner did not declare an objective ledger\ncommand=%q\nanswer=%q\nstdout=%s\nstderr=%s\nevents=%#v",
			result.Command, result.Answer, stdout.String(), stderr.String(), events)
	}
	if pending := pendingStructuredObjectives(result.ObjectiveLedger); len(pending) != 0 {
		t.Fatalf("objective ledger still pending after live run: %#v\nledger=%#v\ncommand=%q\nstdout=%s\nstderr=%s",
			pending, result.ObjectiveLedger, result.Command, stdout.String(), stderr.String())
	}
}
