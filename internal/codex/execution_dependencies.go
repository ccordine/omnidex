package codex

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/gryph/omnidex/internal/db"
)

// Each effectful native session owns private, disposable equivalents of the
// installed database/cache. Never pass the runtime ledger database to Codex.
// Containers use bounded tmpfs, no volumes, loopback ports and a two-hour lease.
type executionDependencies struct {
	names       []string
	environment []string
	once        sync.Once
	cleanupErr  error
}

func prepareExecutionDependencies(ctx context.Context) (*executionDependencies, error) {
	dependencies := &executionDependencies{environment: os.Environ()}
	if os.Getenv("DATABASE_URL") == "" && os.Getenv("REDIS_URL") == "" && os.Getenv("OLLAMA_BASE_URL") == "" {
		return dependencies, nil
	}
	startup, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	fail := func(err error) (*executionDependencies, error) {
		return nil, errors.Join(err, dependencies.close())
	}
	if os.Getenv("DATABASE_URL") != "" || os.Getenv("REDIS_URL") != "" {
		if err := ensureExecutionDocker(startup); err != nil {
			return fail(err)
		}
	}
	for _, service := range []struct {
		configured, name, image, port, data string
	}{
		{os.Getenv("DATABASE_URL"), "postgres", "postgres:18-alpine", "5432", "/var/lib/postgresql"},
		{os.Getenv("REDIS_URL"), "valkey", "valkey/valkey:8-alpine", "6379", "/data"},
	} {
		if service.configured == "" {
			continue
		}
		name, err := executionDependencyName(service.name)
		if err != nil {
			return fail(err)
		}
		dependencies.names = append(dependencies.names, name)
		password, err := executionDependencyName("password")
		if err != nil {
			return fail(err)
		}
		args := []string{"run", "--detach", "--rm", "--name", name, "--label", "omnidex.execution-dependency=true",
			"--cpus", "1", "--memory", "512m", "--memory-swap", "512m", "--pids-limit", "128", "--read-only",
			"--log-driver", "none", "--stop-timeout", "3", "--tmpfs", service.data + ":rw,size=256m",
			"--tmpfs", "/tmp:rw,size=16m", "--tmpfs", "/var/run:rw,size=16m", "--publish", "127.0.0.1::" + service.port}
		if service.name == "postgres" {
			args = append(args, "-e", "POSTGRES_PASSWORD="+password)
		}
		// PID 1 retains the ordinary image entrypoint while this shell bounds its lease.
		args = append(args, service.image, "sh", "-c", `(sleep 7200; kill -TERM 1) & exec docker-entrypoint.sh "$@"`, "lease")
		if service.name == "postgres" {
			args = append(args, "postgres")
		} else {
			args = append(args, "valkey-server", "--save", "", "--appendonly", "no", "--requirepass", password)
		}
		if _, err := dependencyCommand(startup, "docker", args...); err != nil {
			return fail(fmt.Errorf("start disposable %s: %w", service.name, err))
		}
		probe := []string{"exec", name, "pg_isready", "-h", "127.0.0.1", "-U", "postgres"}
		if service.name == "valkey" {
			probe = []string{"exec", name, "valkey-cli", "-a", password, "ping"}
		}
		if err := waitExecutionReady(startup, func() error { _, err := dependencyCommand(startup, "docker", probe...); return err }); err != nil {
			return fail(fmt.Errorf("%s readiness: %w", service.name, err))
		}
		port, err := dependencyCommand(startup, "docker", "port", name, service.port+"/tcp")
		if err != nil {
			return fail(err)
		}
		address := strings.TrimSpace(port)
		if !strings.HasPrefix(address, "127.0.0.1:") || strings.Contains(address, "\n") {
			return fail(fmt.Errorf("%s did not publish a single loopback endpoint", service.name))
		}
		if service.name == "postgres" {
			endpoint := "postgres://postgres:" + password + "@" + address + "/postgres?sslmode=disable"
			dependencies.set("DATABASE_URL", endpoint)
			dependencies.set("OMNI_TEST_DATABASE_URL", endpoint)
			dependencies.set("DATABASE_SCHEMA", db.DefaultRuntimeSchema)
		} else {
			dependencies.set("REDIS_URL", "redis://:"+password+"@"+address+"/0")
		}
	}
	if endpoint := os.Getenv("OLLAMA_BASE_URL"); endpoint != "" {
		if err := ensureExecutionOllama(startup, endpoint); err != nil {
			return fail(err)
		}
	}
	return dependencies, nil
}

func (dependencies *executionDependencies) set(key, value string) {
	for index, entry := range dependencies.environment {
		if strings.HasPrefix(entry, key+"=") {
			dependencies.environment[index] = key + "=" + value
			return
		}
	}
	dependencies.environment = append(dependencies.environment, key+"="+value)
}

func (dependencies *executionDependencies) close() error {
	dependencies.once.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		for _, name := range dependencies.names {
			// A cancelled create may have reached Docker; query the exact owned name.
			ids, err := dependencyCommand(ctx, "docker", "ps", "-aq", "--filter", "name=^/"+name+"$")
			if err == nil && strings.TrimSpace(ids) != "" {
				_, err = dependencyCommand(ctx, "docker", "rm", "--force", "--volumes", name)
			}
			if err != nil {
				dependencies.cleanupErr = errors.Join(dependencies.cleanupErr, fmt.Errorf("cleanup %s: %w", name, err))
			}
		}
	})
	return dependencies.cleanupErr
}

func executionDependencyName(service string) (string, error) {
	var bytes [12]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "", err
	}
	return "omnidex-execution-" + service + "-" + hex.EncodeToString(bytes[:]), nil
}

func dependencyCommand(ctx context.Context, command string, args ...string) (string, error) {
	process := exec.CommandContext(ctx, command, args...)
	process.WaitDelay = 3 * time.Second
	configureProcessLifetime(process)
	var output boundedInstallationOutput
	process.Stdout = &output
	// Do not expose runtime environment or Docker credential material in errors.
	if err := process.Run(); err != nil {
		return "", fmt.Errorf("%s dependency command failed: %w", command, err)
	}
	return string(output.bytes), nil
}

func waitExecutionReady(ctx context.Context, probe func() error) error {
	for {
		if err := probe(); err == nil {
			return nil
		}
		timer := time.NewTimer(250 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return fmt.Errorf("dependency did not become ready: %w", ctx.Err())
		case <-timer.C:
		}
	}
}

func ensureExecutionDocker(ctx context.Context) error {
	if _, err := dependencyCommand(ctx, "docker", "info", "--format", "{{.ServerVersion}}"); err == nil {
		return nil
	}
	endpoint, err := dependencyCommand(ctx, "docker", "context", "inspect", "--format", "{{.Endpoints.docker.Host}}")
	if err != nil {
		return err
	}
	args := []string{"start", "docker.service"}
	if strings.HasPrefix(strings.TrimSpace(endpoint), "unix:///run/user/") {
		args = append([]string{"--user"}, args...)
	}
	if _, err := dependencyCommand(ctx, "systemctl", args...); err != nil {
		return fmt.Errorf("start configured Docker daemon: %w", err)
	}
	return waitExecutionReady(ctx, func() error {
		_, err := dependencyCommand(ctx, "docker", "info", "--format", "{{.ServerVersion}}")
		return err
	})
}

func ensureExecutionOllama(ctx context.Context, endpoint string) error {
	parsed, err := url.Parse(endpoint)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return fmt.Errorf("invalid configured Ollama endpoint")
	}
	probe := func() error {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(endpoint, "/")+"/api/tags", nil)
		if err != nil {
			return err
		}
		response, err := (&http.Client{Timeout: 2 * time.Second}).Do(request)
		if err != nil {
			return err
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusOK {
			return fmt.Errorf("Ollama returned HTTP %d", response.StatusCode)
		}
		return nil
	}
	if probe() == nil {
		return nil
	}
	if parsed.Hostname() == "127.0.0.1" || parsed.Hostname() == "localhost" || parsed.Hostname() == "::1" {
		if _, err := dependencyCommand(ctx, "systemctl", "start", "ollama.service"); err != nil {
			return fmt.Errorf("start configured Ollama: %w", err)
		}
	}
	return waitExecutionReady(ctx, probe)
}
