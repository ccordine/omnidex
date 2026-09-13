package main

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestCLIProductionDependenciesExcludeServerExecutionAndTestHelpers(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "go", "list", "-deps", "-f", "{{.ImportPath}}", ".")
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(entry, "CGO_ENABLED=") {
			command.Env = append(command.Env, entry)
		}
	}
	command.Env = append(command.Env, "CGO_ENABLED=0")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("CLI cannot resolve its portable production dependencies: %v\n%s", err, output)
	}
	for _, dependency := range strings.Fields(string(output)) {
		if dependency == "testing" || strings.HasPrefix(dependency, "github.com/tree-sitter/") {
			t.Errorf("CLI links a server compiler or test helper through %s", dependency)
		}
		for _, serverPackage := range []string{"queue", "db", "worker", "assemblyline", "experiment"} {
			if dependency == "github.com/gryph/omnidex/internal/"+serverPackage {
				t.Errorf("CLI depends on server execution package %s", dependency)
			}
		}
	}
}
