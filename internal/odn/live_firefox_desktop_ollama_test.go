package odn

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestStructuredCommandDecisionOpensFirefoxTabFromLLMSelectedDesktopCommand(t *testing.T) {
	root := t.TempDir()
	binDir := filepath.Join(root, "bin")
	logPath := filepath.Join(root, "desktop.log")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeExecutable(t, filepath.Join(binDir, "firefox"), fmt.Sprintf(`#!/usr/bin/env bash
if [ "${1:-}" = "--new-tab" ]; then
  printf 'firefox %%s\n' "$*" >> %q
  exit 0
fi
while :; do sleep 1; done
`, logPath))
	writeExecutable(t, filepath.Join(binDir, "xdotool"), fmt.Sprintf(`#!/usr/bin/env bash
printf 'xdotool %%s\n' "$*" >> %q
`, logPath))

	firefoxProcess := exec.Command(filepath.Join(binDir, "firefox"))
	firefoxProcess.Env = append(os.Environ(), "PATH="+binDir+":"+os.Getenv("PATH"))
	if err := firefoxProcess.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = firefoxProcess.Process.Kill()
		_, _ = firefoxProcess.Process.Wait()
	})

	url := "https://example.com/odn-firefox-test"
	command := fmt.Sprintf(`set -e
export PATH=%[1]q:"$PATH"
pid=$(ps -eo pid=,args= | awk -v target=%[2]q '$0 ~ target && $0 !~ /awk/ {print $1; exit}')
test -n "$pid"
xdotool search --pid "$pid" windowactivate --sync
firefox --new-tab %[3]q
printf 'OPENED pid=%%s url=%%s\n' "$pid" %[3]q`, binDir, filepath.Join(binDir, "firefox"), url)
	client := &fakeCommandDecisionClient{responses: []string{
		`{"command":` + quoteJSONForTest(command) + `,"done":false,"answer":""}`,
		`{"command":"","done":true,"answer":"Opened the existing Firefox process to ` + url + `."}`,
	}}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	result, err := RunStructuredCommandDecision(context.Background(), "Open my existing browser to the requested website by using its running process.", client, stdout, stderr)
	if err != nil {
		t.Fatalf("desktop browser command failed: %v\nstdout=%s\nstderr=%s", err, stdout.String(), stderr.String())
	}
	if client.calls != 2 {
		t.Fatalf("llm calls = %d, want 2", client.calls)
	}
	if !strings.Contains(stdout.String(), "OPENED pid=") || !strings.Contains(stdout.String(), url) {
		t.Fatalf("stdout missing open evidence: %q", stdout.String())
	}
	logBytes, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	log := string(logBytes)
	for _, want := range []string{"xdotool search --pid", "windowactivate --sync", "firefox --new-tab " + url} {
		if !strings.Contains(log, want) {
			t.Fatalf("desktop log missing %q\nlog=%s\nresult=%#v", want, log, result)
		}
	}
}

func TestLiveOllamaOpensFirefoxTabByPID(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live Firefox desktop test in short mode")
	}
	if strings.TrimSpace(os.Getenv("ODN_RUN_DESKTOP_LIVE")) == "" {
		t.Skip("set ODN_RUN_DESKTOP_LIVE=1 to allow this test to focus Firefox and open a real browser tab")
	}
	if strings.TrimSpace(os.Getenv("DISPLAY")) == "" && strings.TrimSpace(os.Getenv("WAYLAND_DISPLAY")) == "" {
		t.Skip("no GUI display session detected")
	}
	if _, err := exec.LookPath("firefox"); err != nil {
		t.Skipf("firefox command not found: %v", err)
	}
	if err := exec.Command("bash", "-lc", "pgrep -f firefox >/dev/null").Run(); err != nil {
		t.Skip("no running firefox process found")
	}
	client := testOllamaClient(t)
	client.Client.Timeout = 2 * time.Minute
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()

	url := "https://example.com/odn-firefox-live-test"
	prompt := "Open my running Firefox browser to " + url + ". Check running processes, identify the Firefox PID, bring that browser window to the front if a desktop window tool is available, then open the URL in a new tab. Use command evidence and do not install packages."
	result, err := RunStructuredCommandDecision(ctx, prompt, client, stdout, stderr)
	if err != nil {
		if isOllamaRunnerStoppedError(err) || isLiveModelTimeoutError(err) {
			t.Skipf("live Firefox desktop model run unavailable: %v", err)
		}
		t.Fatalf("live Firefox desktop command failed: %v\ncommand=%q\nanswer=%q\nstdout=%s\nstderr=%s\nobservations=%#v",
			err, result.Command, result.Answer, stdout.String(), stderr.String(), result.Observations)
	}
	combined := strings.ToLower(stdout.String() + "\n" + stderr.String() + "\n" + result.Answer)
	if !strings.Contains(combined, "firefox") || !strings.Contains(combined, strings.ToLower(url)) {
		t.Fatalf("live Firefox result missing browser/url evidence\ncommand=%q\nanswer=%q\nstdout=%s\nstderr=%s\nobservations=%#v",
			result.Command, result.Answer, stdout.String(), stderr.String(), result.Observations)
	}
}

func writeExecutable(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
}
