package odn

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestStimulusTailwindSmokeAppValidator(t *testing.T) {
	root := t.TempDir()
	port := freeTCPPort(t)
	appDir := filepath.Join(root, "stimulus-tailwind-smoke")
	pidFile := filepath.Join(root, "odn-webapp.pid")
	t.Cleanup(func() {
		stopPIDFileProcess(pidFile)
		_ = exec.Command("bash", "-lc", fmt.Sprintf("if command -v fuser >/dev/null 2>&1; then fuser -k %d/tcp >/dev/null 2>&1 || true; fi", port)).Run()
	})

	if err := os.MkdirAll(appDir, 0o755); err != nil {
		t.Fatal(err)
	}
	index := `<!doctype html>
<html>
<head>
  <script src="https://cdn.tailwindcss.com"></script>
  <script type="module">
    import { Application, Controller } from "https://unpkg.com/@hotwired/stimulus/dist/stimulus.js"
    window.Stimulus = Application.start()
    Stimulus.register("demo", class extends Controller { ping() { this.element.dataset.smoke = "ok" } })
  </script>
</head>
<body class="bg-slate-950 text-white">
  <main data-controller="demo">
    <h1 class="text-3xl font-bold">ODN Stimulus Tailwind Smoke</h1>
    <button class="rounded bg-emerald-500 px-3 py-2" data-action="click->demo#ping">Ping</button>
  </main>
</body>
</html>`
	if err := os.WriteFile(filepath.Join(appDir, "index.html"), []byte(index), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("python3", "-m", "http.server", fmt.Sprintf("%d", port), "--bind", "127.0.0.1", "--directory", appDir)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pidFile, []byte(fmt.Sprintf("%d", cmd.Process.Pid)), 0o644); err != nil {
		t.Fatal(err)
	}

	body := fetchSmokeApp(t, fmt.Sprintf("http://127.0.0.1:%d/", port))
	assertStimulusTailwindSmokePage(t, body)
}

func TestLiveOllamaBuildsAndServesStimulusTailwindSmokeApp(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live web app smoke build in short mode")
	}
	skipUnlessHeavyLiveEnabled(t)
	client := testOllamaClient(t)
	client.Client.Timeout = 2 * time.Minute

	root := t.TempDir()
	port := freeTCPPort(t)
	appDir := filepath.Join(root, "stimulus-tailwind-smoke")
	pidFile := filepath.Join(root, "odn-webapp.pid")
	t.Cleanup(func() {
		stopPIDFileProcess(pidFile)
		_ = exec.Command("bash", "-lc", fmt.Sprintf("if command -v fuser >/dev/null 2>&1; then fuser -k %d/tcp >/dev/null 2>&1 || true; fi", port)).Run()
	})

	prompt := fmt.Sprintf(
		"Build a smoke test demo web app in %s and serve it at http://127.0.0.1:%d/. Use one single first shell command for the whole job: create the directory, write static index.html, start the server, write the PID file, then curl the served page. Use Tailwind CSS from a CDN and Stimulus JS from a CDN. The page must include visible text ODN Stimulus Tailwind Smoke, Tailwind utility classes, a Stimulus controller with data-controller, and a button wired with data-action. Do not use npm or install packages. Create files in the foreground. The command must start the server before curl. Do not use '&& nohup ... &'. Use this server shape after file creation with semicolons: nohup python3 -m http.server %d --bind 127.0.0.1 --directory %s > %s 2>&1 & server_pid=$!; echo \"$server_pid\" > %s; Then verify with a retry loop like: for i in 1 2 3 4 5; do curl -fsS http://127.0.0.1:%d/ | grep 'ODN Stimulus Tailwind Smoke' && break; sleep 1; done.",
		appDir,
		port,
		port,
		appDir,
		filepath.Join(root, "server.log"),
		pidFile,
		port,
	)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()

	result, err := RunStructuredCommandDecisionWithEventsAndAsk(ctx, prompt, client, stdout, stderr, nil, func(ctx context.Context, question string) (string, error) {
		t.Fatalf("web app smoke test should not require user input; question=%q", question)
		return "", nil
	})
	if err != nil {
		if isOllamaRunnerStoppedError(err) {
			t.Skipf("Ollama runner stopped during live web app smoke test: %v", err)
		}
		if isLiveModelTimeoutError(err) && validateStimulusTailwindSmokeArtifacts(t, appDir, port) {
			return
		}
		t.Fatalf("web app build failed: %v\ncommand=%q\nanswer=%q\nstdout=%s\nstderr=%s\nobservations=%#v",
			err, result.Command, result.Answer, stdout.String(), stderr.String(), result.Observations)
	}
	if !hasSuccessfulCommandObservation(result.Observations) {
		t.Fatalf("expected successful command observation: %#v", result.Observations)
	}

	indexPath := filepath.Join(appDir, "index.html")
	indexBytes, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatalf("expected generated index.html at %s; command=%q stdout=%s stderr=%s err=%v", indexPath, result.Command, stdout.String(), stderr.String(), err)
	}
	assertStimulusTailwindSmokePage(t, string(indexBytes))

	assertNoFalseCapabilityLimitation(t, client, result, stdout.String(), stderr.String())
	body := fetchSmokeApp(t, fmt.Sprintf("http://127.0.0.1:%d/", port))
	assertStimulusTailwindSmokePage(t, body)
}

func validateStimulusTailwindSmokeArtifacts(t *testing.T, appDir string, port int) bool {
	t.Helper()
	indexBytes, err := os.ReadFile(filepath.Join(appDir, "index.html"))
	if err != nil {
		return false
	}
	assertStimulusTailwindSmokePage(t, string(indexBytes))
	body := fetchSmokeApp(t, fmt.Sprintf("http://127.0.0.1:%d/", port))
	assertStimulusTailwindSmokePage(t, body)
	return true
}

func freeTCPPort(t *testing.T) int {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("allocate free tcp port: %v", err)
	}
	defer listener.Close()

	addr, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatalf("unexpected listener addr %T", listener.Addr())
	}
	return addr.Port
}

func fetchSmokeApp(t *testing.T, url string) string {
	t.Helper()

	client := http.Client{Timeout: 2 * time.Second}
	var lastErr error
	for deadline := time.Now().Add(20 * time.Second); time.Now().Before(deadline); time.Sleep(250 * time.Millisecond) {
		resp, err := client.Get(url)
		if err != nil {
			lastErr = err
			continue
		}
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		_ = resp.Body.Close()
		if readErr != nil {
			lastErr = readErr
			continue
		}
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return string(body)
		}
		lastErr = fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}
	t.Fatalf("smoke app did not respond at %s: %v", url, lastErr)
	return ""
}

func stopPIDFileProcess(pidFile string) {
	pidBytes, err := os.ReadFile(pidFile)
	if err != nil {
		return
	}
	pid := strings.TrimSpace(string(pidBytes))
	if pid == "" || strings.ContainsAny(pid, " \t\r\n/") {
		return
	}
	_ = exec.Command("kill", pid).Run()
}

func assertStimulusTailwindSmokePage(t *testing.T, body string) {
	t.Helper()

	for _, want := range []string{
		"ODN Stimulus Tailwind Smoke",
		"tailwind",
		"data-controller",
		"data-action",
	} {
		if !strings.Contains(strings.ToLower(body), strings.ToLower(want)) {
			t.Fatalf("smoke page missing %q\n%s", want, body)
		}
	}
	if !strings.Contains(strings.ToLower(body), "stimulus") && !strings.Contains(strings.ToLower(body), "@hotwired") {
		t.Fatalf("smoke page missing Stimulus reference\n%s", body)
	}
}
