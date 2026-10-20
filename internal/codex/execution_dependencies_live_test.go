package codex

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/manifest"
	"github.com/jackc/pgx/v5"
)

// A transport/dependency qualification fixture, not workload autonomy evidence.
func TestInstalledExecutionDependencyRoles(t *testing.T) {
	if os.Getenv("OMNI_TEST_EXECUTION_DEPENDENCIES") != "1" {
		t.Skip("installed dependency qualification not selected")
	}
	client, _ := installedAgentClient(t)
	routes := manifest.Defaults().Coding()
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	task := &codingagent.Task{ID: "dependency-qualification", Goal: fmt.Sprintf("Verify the supplied disposable runtime services by running exactly %s -test.run '^TestExecutionDependencyOperations$' -test.v. This command checks PostgreSQL transactions, Valkey writes and reads, and reachability using inherited environment. Do not print connection URLs or credentials. Report the observed result. This is the entire frozen task; no implementation edits are needed.", binary)}
	var threads []string
	for _, role := range []codingagent.Role{codingagent.Execute, codingagent.Review} {
		t.Run(string(role), func(t *testing.T) {
			before := dependencyContainers(t)
			if before != "" {
				t.Fatalf("fixture must begin with disposable dependencies absent: %s", before)
			}
			route := routes.Executor
			if role == codingagent.Review {
				route = routes.Reviewer
			}
			root := t.TempDir()
			call := codingagent.Assignment{JobID: 1, Generation: 1, Root: root, Role: role, Route: route, Task: task}
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
			defer cancel()
			if os.Getenv("OMNI_TEST_ISOLATED_SERVICE_RECOVERY") == "1" {
				if _, err := dependencyCommand(ctx, "systemctl", "stop", "docker.service", "ollama.service"); err != nil {
					t.Fatal(err)
				}
				if _, err := dependencyCommand(ctx, "docker", "info"); err == nil {
					t.Fatal("isolated Docker must be stopped before native role")
				}
				response, err := (&http.Client{Timeout: time.Second}).Get(strings.TrimRight(os.Getenv("OLLAMA_BASE_URL"), "/") + "/api/tags")
				if err == nil {
					response.Body.Close()
					t.Fatal("isolated Ollama must be stopped before native role")
				}
				t.Log("real isolated Docker and Ollama stopped before native role")
			}
			commandSeen, proofSeen := false, false
			result, err := client.Run(ctx, call, func(event codingagent.NativeEvent) error {
				var frame struct {
					Method string `json:"method"`
					Params struct {
						Item struct {
							Type string `json:"type"`
						} `json:"item"`
					} `json:"params"`
				}
				_ = json.Unmarshal(event.Frame, &frame)
				if frame.Params.Item.Type == "commandExecution" && strings.Contains(string(event.Frame), "TestExecutionDependencyOperations") {
					commandSeen = true
				}
				if (frame.Params.Item.Type == "commandExecution" || frame.Method == "item/commandExecution/outputDelta") && strings.Contains(string(event.Frame), "DEPENDENCY_OPERATIONS_PASSED") {
					proofSeen = true
				}
				if file := os.Getenv("OMNI_TEST_DEPENDENCY_EVIDENCE"); file != "" {
					// Raw native frames may contain secrets; retain only method/type and proof flags.

					output, err := os.OpenFile(file, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
					if err != nil {
						return err
					}
					err = json.NewEncoder(output).Encode(map[string]any{"role": role, "method": frame.Method, "item_type": frame.Params.Item.Type, "command_seen": commandSeen, "proof_seen": proofSeen})
					closeErr := output.Close()
					if err != nil {
						return err
					}
					return closeErr
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			if !commandSeen || !proofSeen {
				t.Fatalf("native operation proof missing: command=%v proof=%v; response=%s", commandSeen, proofSeen, result.Content)
			}
			if dependencyContainers(t) != "" {
				t.Fatal("native role leaked disposable dependencies")
			}
			threads = append(threads, result.ThreadID)
			t.Logf("%s installed Codex thread=%s turn=%s: real dependent operations passed; owned containers removed", role, result.ThreadID, result.TurnID)
		})
	}
	if len(threads) == 2 && threads[0] == threads[1] {
		t.Fatal("review reused execution thread")
	}
}

func TestExecutionDependencyOperations(t *testing.T) {
	if os.Getenv("OMNI_TEST_EXECUTION_DEPENDENCIES") != "1" {
		t.Skip("dependency operation fixture not selected")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	databaseURL := os.Getenv("OMNI_TEST_DATABASE_URL")
	if databaseURL == "" || databaseURL != os.Getenv("DATABASE_URL") {
		t.Fatal("child database endpoints missing or disagree")
	}
	database, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		t.Fatal("child PostgreSQL not reachable")
	}
	defer database.Close(ctx)
	var value int
	if err := database.QueryRow(ctx, "SELECT 42").Scan(&value); err != nil || value != 42 {
		t.Fatal("PostgreSQL query failed")
	}
	if _, err := database.Exec(ctx, "CREATE TEMP TABLE dependency_probe (value integer); INSERT INTO dependency_probe VALUES (73)"); err != nil {
		t.Fatal("PostgreSQL write failed")
	}
	if err := database.QueryRow(ctx, "SELECT value FROM dependency_probe").Scan(&value); err != nil || value != 73 {
		t.Fatal("PostgreSQL write/read failed")
	}
	endpoint, err := url.Parse(os.Getenv("REDIS_URL"))
	if err != nil || endpoint.Host == "" || endpoint.User == nil {
		t.Fatal("child cache endpoint missing")
	}
	cache, err := net.DialTimeout("tcp", endpoint.Host, 2*time.Second)
	if err != nil {
		t.Fatal("child Valkey not reachable")
	}
	defer cache.Close()
	_ = cache.SetDeadline(time.Now().Add(5 * time.Second))
	password, _ := endpoint.User.Password()
	reader := bufio.NewReader(cache)
	for _, command := range []string{"AUTH " + password + "\r\n", "SET dependency_probe 73\r\n", "GET dependency_probe\r\n"} {
		if _, err := fmt.Fprint(cache, command); err != nil {
			t.Fatal("Valkey command failed")
		}
		line, err := reader.ReadString('\n')
		if err != nil || strings.HasPrefix(line, "-") {
			t.Fatal("Valkey rejected command")
		}
		if strings.HasPrefix(command, "GET") {
			value, err := reader.ReadString('\n')
			if err != nil || value != "73\r\n" {
				t.Fatal("Valkey write/read failed")
			}
		} else if line != "+OK\r\n" {
			t.Fatal("Valkey write/authentication failed")
		}
	}
	if err := ensureExecutionOllama(ctx, os.Getenv("OLLAMA_BASE_URL")); err != nil {
		t.Fatal("configured Ollama not reachable")
	}
	if _, err := dependencyCommand(ctx, "docker", "info", "--format", "{{.ServerVersion}}"); err != nil {
		t.Fatal("configured Docker not reachable")
	}
	verifyInstalledDependencyRuntime(t, ctx)
	t.Log("DEPENDENCY_OPERATIONS_PASSED")
}

func dependencyContainers(t *testing.T) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	result, err := dependencyCommand(ctx, "docker", "ps", "-aq", "--filter", "label=omnidex.execution-dependency=true")
	if err != nil {
		t.Fatal(err)
	}
	baseline := strings.Fields(os.Getenv("OMNI_TEST_DEPENDENCY_BASELINE"))
	var owned []string
	for _, id := range strings.Fields(result) {
		existing := false
		for _, preserved := range baseline {
			if id == preserved {
				existing = true
				break
			}
		}
		if !existing {
			owned = append(owned, id)
		}
	}
	return strings.Join(owned, "\n")
}

func TestRealExecutionDependencyReadinessFailureCleanup(t *testing.T) {
	if os.Getenv("OMNI_TEST_EXECUTION_DEPENDENCIES") != "1" {
		t.Skip("real Docker failure qualification not selected")
	}
	if dependencyContainers(t) != "" {
		t.Fatal("fixture must start without disposable dependencies")
	}
	root := t.TempDir()
	script := `#!/bin/sh
set -eu
if [ "$1" = run ]; then
 id=$(/usr/local/bin/docker-real "$@")
 /usr/local/bin/docker-real pause "$id" >/dev/null
 printf '%s\n' "$id"
else
 exec /usr/local/bin/docker-real "$@"
fi
`
	if err := os.WriteFile(filepath.Join(root, "docker"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", root+":"+os.Getenv("PATH"))
	client, _ := installedAgentClient(t)
	for _, role := range []codingagent.Role{codingagent.Execute, codingagent.Review} {
		t.Run(string(role), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 8*time.Second)
			defer cancel()
			result, err := client.Run(ctx, codingagent.Assignment{JobID: 1, Generation: 1, Root: t.TempDir(), Role: role, Route: manifest.Defaults().Coding().Executor, Task: &codingagent.Task{ID: "readiness-failure", Goal: "Verify dependencies."}}, nil)
			if err == nil || !strings.Contains(err.Error(), "readiness") || result.ThreadID != "" {
				t.Fatalf("native role did not fail before inference on paused service: %v", err)
			}
			if dependencyContainers(t) != "" {
				t.Fatal("readiness failure leaked real disposable container")
			}
			t.Log("native role rejected paused PostgreSQL readiness; real container and private data removed")
		})
	}
}

func verifyInstalledDependencyRuntime(t *testing.T, ctx context.Context) {
	t.Helper()
	binary := os.Getenv("OMNI_TEST_CORE_COMMAND")
	if binary == "" {
		t.Fatal("installed core qualification requires OMNI_TEST_CORE_COMMAND")
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	_ = listener.Close()
	runtimeCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	command := exec.CommandContext(runtimeCtx, binary)
	command.Env = append(os.Environ(), "LISTEN_ADDR="+address)
	configureProcessLifetime(command)
	command.WaitDelay = 3 * time.Second
	// Core diagnostics can contain configuration; never copy them to native frames.
	if err := command.Start(); err != nil {
		t.Fatal("installed core launch failed")
	}
	done := make(chan error, 1)
	go func() { done <- command.Wait() }()
	defer func() { cancel(); <-done }()
	ready := false
	for deadline := time.Now().Add(8 * time.Second); time.Now().Before(deadline); {
		response, err := (&http.Client{Timeout: time.Second}).Get("http://" + address + "/readyz")
		if err == nil {
			ready = response.StatusCode == http.StatusOK
			response.Body.Close()
		}
		if ready {
			break
		}
		select {
		case err := <-done:
			done <- err
			t.Fatalf("installed core exited before readiness: %v", err)
		case <-time.After(100 * time.Millisecond):
		}
	}
	if !ready {
		t.Fatal("installed core failed readiness using inherited dependency environment")
	}
	t.Log("installed core ready using inherited disposable database/schema/cache")
}
