package odn

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestReactTypeScriptSmokeAppValidator(t *testing.T) {
	requireNodeAndNPM(t)
	root := t.TempDir()
	port := freeTCPPort(t)
	appDir := filepath.Join(root, "react-ts-smoke")
	pidFile := filepath.Join(root, "react-ts.pid")
	t.Cleanup(func() {
		stopPIDFileProcess(pidFile)
		_ = exec.Command("bash", "-lc", fmt.Sprintf("if command -v fuser >/dev/null 2>&1; then fuser -k %d/tcp >/dev/null 2>&1 || true; fi", port)).Run()
	})

	createMinimalReactTypeScriptProject(t, appDir)
	runInDir(t, appDir, "npm install")
	runInDir(t, appDir, "npm run build")
	cmd := exec.Command("npm", "run", "preview", "--", "--host", "127.0.0.1", "--port", fmt.Sprintf("%d", port))
	cmd.Dir = appDir
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pidFile, []byte(fmt.Sprintf("%d", cmd.Process.Pid)), 0o644); err != nil {
		t.Fatal(err)
	}

	assertReactTypeScriptServedBuild(t, fmt.Sprintf("http://127.0.0.1:%d/", port))
}

func TestLiveOllamaBuildsNPMReactTypeScriptSmokeApp(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live React TypeScript npm smoke build in short mode")
	}
	skipUnlessHeavyLiveEnabled(t)
	requireNodeAndNPM(t)
	client := testOllamaClient(t)
	client.Client.Timeout = 2 * time.Minute

	root := t.TempDir()
	port := freeTCPPort(t)
	appDir := filepath.Join(root, "react-ts-smoke")
	pidFile := filepath.Join(root, "react-ts.pid")
	t.Cleanup(func() {
		stopPIDFileProcess(pidFile)
		_ = exec.Command("bash", "-lc", fmt.Sprintf("if command -v fuser >/dev/null 2>&1; then fuser -k %d/tcp >/dev/null 2>&1 || true; fi", port)).Run()
	})

	prompt := fmt.Sprintf(
		"Build a boilerplate React TypeScript npm project in %s, then install dependencies, run an equivalent TypeScript/build check, build it, start a local preview server on http://127.0.0.1:%d/, write the server PID to %s, and verify it with curl. The app must visibly render ODN React TypeScript Smoke. Use npm and TypeScript. Create a minimal Vite React TypeScript project by writing package.json, tsconfig.json, index.html, src/main.tsx, and src/App.tsx. Do not use create-react-app. Do not use npx create-react-app. Do not ask the user to run commands. Commands run in fresh shells, so use absolute paths or include cd %s in every npm command. Use foreground commands for file creation, npm install, and npm run build. Start only the long-running preview server in the background, redirect its stdout/stderr to %s, capture $! in the PID file, then verify with a curl retry loop.",
		appDir,
		port,
		pidFile,
		appDir,
		filepath.Join(root, "react-preview.log"),
	)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()

	questions := []string{}
	result, err := RunStructuredCommandDecisionWithEventsAndAsk(ctx, prompt, client, stdout, stderr, nil, func(ctx context.Context, question string) (string, error) {
		questions = append(questions, question)
		return "Proceed. You have permission to write files in the requested temp directory and run npm commands for this test.", nil
	})
	if err != nil {
		if isOllamaRunnerStoppedError(err) {
			t.Skipf("Ollama runner stopped during live React TypeScript smoke test: %v", err)
		}
		if isLiveModelTimeoutError(err) {
			if validateReactTypeScriptSmokeArtifacts(appDir, port) == nil {
				return
			}
			t.Skipf("Live model timed out before completing React TypeScript chain: %v", err)
		}
		t.Fatalf("React TypeScript build failed: %v\ncommand=%q\nanswer=%q\nstdout=%s\nstderr=%s\nobservations=%#v",
			err, result.Command, result.Answer, stdout.String(), stderr.String(), result.Observations)
	}
	if !hasSuccessfulCommandObservation(result.Observations) {
		t.Fatalf("expected successful command observation: %#v", result.Observations)
	}

	for _, rel := range []string{"package.json", "tsconfig.json", "src/App.tsx"} {
		if _, err := os.Stat(filepath.Join(appDir, rel)); err != nil {
			t.Fatalf("expected generated %s; command=%q stdout=%s stderr=%s err=%v", rel, result.Command, stdout.String(), stderr.String(), err)
		}
	}
	if _, err := os.Stat(filepath.Join(appDir, "dist", "index.html")); err != nil {
		t.Fatalf("expected npm build output dist/index.html; command=%q stdout=%s stderr=%s err=%v", result.Command, stdout.String(), stderr.String(), err)
	}
	appBytes, err := os.ReadFile(filepath.Join(appDir, "src", "App.tsx"))
	if err != nil {
		t.Fatal(err)
	}
	assertReactTypeScriptSource(t, string(appBytes))

	assertReactTypeScriptServedBuild(t, fmt.Sprintf("http://127.0.0.1:%d/", port))
}

func TestStructuredCommandDecisionBuildsNPMReactTypeScriptProjectFromLLMCommands(t *testing.T) {
	requireNodeAndNPM(t)
	root := t.TempDir()
	port := freeTCPPort(t)
	appDir := filepath.Join(root, "react-ts-smoke")
	pidFile := filepath.Join(root, "react-ts.pid")
	t.Cleanup(func() {
		stopPIDFileProcess(pidFile)
		_ = exec.Command("bash", "-lc", fmt.Sprintf("if command -v fuser >/dev/null 2>&1; then fuser -k %d/tcp >/dev/null 2>&1 || true; fi", port)).Run()
	})

	command := buildReactTypeScriptSmokeCommand(appDir, port, pidFile, filepath.Join(root, "react-preview.log"))
	client := &fakeCommandDecisionClient{responses: []string{
		`{"command":` + quoteJSONForTest(command) + `,"done":false,"answer":""}`,
		`{"command":"","done":true,"answer":"React TypeScript npm smoke app built and served"}`,
	}}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	result, err := RunStructuredCommandDecision(ctx, "Build and serve a React TypeScript npm smoke app.", client, stdout, stderr)
	if err != nil {
		t.Fatalf("structured command React build failed: %v\ncommand=%q\nstdout=%s\nstderr=%s", err, result.Command, stdout.String(), stderr.String())
	}
	if client.calls != 2 {
		t.Fatalf("llm calls = %d, want 2", client.calls)
	}
	for _, rel := range []string{"package.json", "tsconfig.json", "src/App.tsx", "dist/index.html"} {
		if _, err := os.Stat(filepath.Join(appDir, rel)); err != nil {
			t.Fatalf("expected %s: %v\nstdout=%s\nstderr=%s", rel, err, stdout.String(), stderr.String())
		}
	}
	assertReactTypeScriptServedBuild(t, fmt.Sprintf("http://127.0.0.1:%d/", port))
}

func requireNodeAndNPM(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("node"); err != nil {
		t.Skipf("node unavailable: %v", err)
	}
	if _, err := exec.LookPath("npm"); err != nil {
		t.Skipf("npm unavailable: %v", err)
	}
}

func createMinimalReactTypeScriptProject(t *testing.T, appDir string) {
	t.Helper()
	files := map[string]string{
		"package.json": `{"scripts":{"build":"vite build","preview":"vite preview"},"dependencies":{"@vitejs/plugin-react":"latest","vite":"latest","typescript":"latest","react":"latest","react-dom":"latest"},"devDependencies":{}}`,
		"index.html":   `<div id="root"></div><script type="module" src="/src/main.tsx"></script>`,
		"tsconfig.json": `{
  "compilerOptions": {
    "target": "ES2020",
    "useDefineForClassFields": true,
    "lib": ["DOM", "DOM.Iterable", "ES2020"],
    "allowJs": false,
    "skipLibCheck": true,
    "esModuleInterop": true,
    "allowSyntheticDefaultImports": true,
    "strict": true,
    "forceConsistentCasingInFileNames": true,
    "module": "ESNext",
    "moduleResolution": "Node",
    "resolveJsonModule": true,
    "isolatedModules": true,
    "noEmit": true,
    "jsx": "react-jsx"
  },
  "include": ["src"]
}`,
		"src/main.tsx": `import React from 'react';
import { createRoot } from 'react-dom/client';
import App from './App';

createRoot(document.getElementById('root')!).render(<App />);
`,
		"src/App.tsx": `export default function App() {
  return <main><h1>ODN React TypeScript Smoke</h1><p>npm build verified</p></main>;
}
`,
	}
	for rel, content := range files {
		path := filepath.Join(appDir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func buildReactTypeScriptSmokeCommand(appDir string, port int, pidFile, logFile string) string {
	return fmt.Sprintf(`set -e
mkdir -p %[1]s/src
cat > %[1]s/package.json <<'JSON'
{"scripts":{"build":"vite build","preview":"vite preview"},"dependencies":{"@vitejs/plugin-react":"latest","vite":"latest","typescript":"latest","react":"latest","react-dom":"latest"},"devDependencies":{}}
JSON
cat > %[1]s/index.html <<'HTML'
<div id="root"></div><script type="module" src="/src/main.tsx"></script>
HTML
cat > %[1]s/tsconfig.json <<'JSON'
{"compilerOptions":{"target":"ES2020","useDefineForClassFields":true,"lib":["DOM","DOM.Iterable","ES2020"],"allowJs":false,"skipLibCheck":true,"esModuleInterop":true,"allowSyntheticDefaultImports":true,"strict":true,"forceConsistentCasingInFileNames":true,"module":"ESNext","moduleResolution":"Node","resolveJsonModule":true,"isolatedModules":true,"noEmit":true,"jsx":"react-jsx"},"include":["src"]}
JSON
cat > %[1]s/src/main.tsx <<'TS'
import React from 'react';
import { createRoot } from 'react-dom/client';
import App from './App';
createRoot(document.getElementById('root')!).render(<App />);
TS
cat > %[1]s/src/App.tsx <<'TS'
export default function App() {
  return <main><h1>ODN React TypeScript Smoke</h1><p>npm build verified</p></main>;
}
TS
cd %[1]s
npm install --silent
npm run build --silent
nohup npm run preview -- --host 127.0.0.1 --port %[2]d > %[4]s 2>&1 &
server_pid=$!
echo "$server_pid" > %[3]s
for i in 1 2 3 4 5 6 7 8 9 10; do curl -fsS http://127.0.0.1:%[2]d/ >/tmp/odn-react-ts-smoke.html && break; sleep 1; done
grep 'script' /tmp/odn-react-ts-smoke.html`, appDir, port, pidFile, logFile)
}

func runInDir(t *testing.T, dir, command string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "bash", "-lc", command)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s failed in %s: %v\n%s", command, dir, err, string(out))
	}
}

func assertReactTypeScriptSource(t *testing.T, body string) {
	t.Helper()
	lower := strings.ToLower(body)
	for _, want := range []string{"odn react typescript smoke"} {
		if !strings.Contains(lower, want) {
			t.Fatalf("React TypeScript smoke output missing %q\n%s", want, body)
		}
	}
}

func assertReactTypeScriptServedBuild(t *testing.T, baseURL string) {
	t.Helper()
	body := fetchSmokeApp(t, baseURL)
	lower := strings.ToLower(body)
	if !strings.Contains(lower, `<div id="root"`) && !strings.Contains(lower, `<div id=root`) {
		t.Fatalf("served React HTML missing root element\n%s", body)
	}
	assetPath := firstScriptSrc(body)
	if assetPath == "" {
		t.Fatalf("served React HTML missing script asset\n%s", body)
	}
	assetURL := strings.TrimRight(baseURL, "/") + "/" + strings.TrimLeft(assetPath, "/")
	asset := fetchSmokeApp(t, assetURL)
	assertReactTypeScriptSource(t, asset)
}

func firstScriptSrc(html string) string {
	lower := strings.ToLower(html)
	scriptIndex := strings.Index(lower, "<script")
	if scriptIndex < 0 {
		return ""
	}
	srcIndex := strings.Index(lower[scriptIndex:], "src=")
	if srcIndex < 0 {
		return ""
	}
	start := scriptIndex + srcIndex + len("src=")
	if start >= len(html) {
		return ""
	}
	quote := html[start]
	if quote != '"' && quote != '\'' {
		return ""
	}
	end := strings.IndexByte(html[start+1:], quote)
	if end < 0 {
		return ""
	}
	return html[start+1 : start+1+end]
}

func isOllamaRunnerStoppedError(err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "model runner has unexpectedly stopped") ||
		strings.Contains(text, "ollama returned status 500") ||
		strings.Contains(text, "unexpected eof")
}

func isLiveModelTimeoutError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "context deadline exceeded")
}

func validateReactTypeScriptSmokeArtifacts(appDir string, port int) error {
	for _, rel := range []string{"package.json", "tsconfig.json", "src/App.tsx", "dist/index.html"} {
		if _, err := os.Stat(filepath.Join(appDir, rel)); err != nil {
			return err
		}
	}
	client := http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/", port))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("status %d", resp.StatusCode)
	}
	return nil
}

func isOllamaRunnerStoppedSummary(summary string, events []Event) bool {
	if strings.Contains(strings.ToLower(summary), "model runner has unexpectedly stopped") {
		return true
	}
	if strings.Contains(strings.ToLower(summary), "unexpected eof") {
		return true
	}
	for _, event := range events {
		for _, value := range event.Details {
			if isOllamaRunnerStoppedError(fmt.Errorf("%s", value)) {
				return true
			}
		}
	}
	return false
}
