package odn

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestLiveOllamaIdentifiesOperatingSystemFromCommandEvidence(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live OS detection test in short mode")
	}
	skipUnlessLiveOllamaEnabled(t)
	client := testOllamaClient(t)
	client.Client.Timeout = 2 * time.Minute
	baseline := readLiveOSBaseline(t)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	result, err := RunStructuredCommandDecisionWithEventsAndAsk(ctx, "Identify this machine's operating system, distro/version, kernel, architecture, and package manager from command evidence.", client, stdout, stderr, nil, func(ctx context.Context, question string) (string, error) {
		return "Inspect only. Do not install or modify packages.", nil
	})
	if err != nil {
		if isOllamaRunnerStoppedError(err) {
			t.Skipf("Ollama runner stopped during live OS test: %v", err)
		}
		t.Fatal(err)
	}
	if !hasRealCommandObservation(result.Observations) {
		t.Fatalf("expected real command observation: %#v", result.Observations)
	}

	evidence := strings.Join([]string{stdout.String(), stderr.String(), result.Answer}, "\n")
	for _, want := range baseline {
		if !strings.Contains(strings.ToLower(evidence), strings.ToLower(want)) {
			t.Fatalf("OS evidence missing %q\ncommand=%q\nanswer=%s\nstdout=%s\nstderr=%s", want, result.Command, result.Answer, stdout.String(), stderr.String())
		}
	}
}

func readLiveOSBaseline(t *testing.T) []string {
	t.Helper()

	osRelease, err := os.ReadFile("/etc/os-release")
	if err != nil {
		t.Skipf("os-release unavailable: %v", err)
	}
	unameOut, err := exec.Command("uname", "-m").Output()
	if err != nil {
		t.Fatalf("uname -m failed: %v", err)
	}

	wants := []string{}
	if value := osReleaseValue(string(osRelease), "ID"); value != "" {
		wants = append(wants, value)
	}
	if value := osReleaseValue(string(osRelease), "NAME"); value != "" {
		wants = append(wants, value)
	}
	if arch := strings.TrimSpace(string(unameOut)); arch != "" {
		wants = append(wants, arch)
	}
	if path, err := exec.LookPath("pacman"); err == nil && strings.TrimSpace(path) != "" {
		wants = append(wants, "pacman")
	} else if path, err := exec.LookPath("apt"); err == nil && strings.TrimSpace(path) != "" {
		wants = append(wants, "apt")
	} else if path, err := exec.LookPath("dnf"); err == nil && strings.TrimSpace(path) != "" {
		wants = append(wants, "dnf")
	}
	if len(wants) == 0 {
		t.Skip("no OS baseline fields available")
	}
	return wants
}

func osReleaseValue(content, key string) string {
	prefix := key + "="
	for _, line := range strings.Split(content, "\n") {
		if strings.HasPrefix(line, prefix) {
			return strings.Trim(strings.TrimSpace(line[len(prefix):]), `"`)
		}
	}
	return ""
}
