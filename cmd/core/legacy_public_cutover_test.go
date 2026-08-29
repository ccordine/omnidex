package main

import (
	"os"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/db"
)

func TestCoreExposesOnlyExplicitLegacyPublicPreservationCommand(t *testing.T) {
	raw, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	const command = `database:preserve-legacy-public`
	if strings.Count(source, command) != 1 {
		t.Fatalf("legacy preservation command count=%d want 1",
			strings.Count(source, command))
	}
	for _, forbidden := range []string{
		"database:migrate-legacy", "database:copy-public", "database:reset-legacy",
	} {
		if strings.Contains(source, forbidden) {
			t.Fatalf("core contains forbidden legacy command %q", forbidden)
		}
	}
}

func TestLegacyPublicPreservationConfigIgnoresUnrelatedModelRuntime(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://agent:secret@127.0.0.1/omnidex")
	t.Setenv("LLM_PROVIDER", "ollama")
	t.Setenv("OLLAMA_BASE_URL", "")
	config, err := loadCoreDatabaseCommandConfig()
	if err != nil {
		t.Fatal(err)
	}
	if config.databaseURL != "postgres://agent:secret@127.0.0.1/omnidex" ||
		config.runtimeSchema != db.DefaultRuntimeSchema {
		t.Fatalf("preservation config=%+v", config)
	}
}

func TestLegacyPublicPreservationConfigFailsLoudly(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	if _, err := loadCoreDatabaseCommandConfig(); err == nil ||
		!strings.Contains(err.Error(), "DATABASE_URL is required") {
		t.Fatalf("missing DATABASE_URL error=%v", err)
	}

	t.Setenv("DATABASE_URL", "postgres://agent:secret@127.0.0.1/omnidex")
	t.Setenv("DATABASE_SCHEMA", "")
	if _, err := loadCoreDatabaseCommandConfig(); err == nil ||
		!strings.Contains(err.Error(), "DATABASE_SCHEMA is explicitly empty") {
		t.Fatalf("empty DATABASE_SCHEMA error=%v", err)
	}

	t.Setenv("DATABASE_SCHEMA", db.DefaultRuntimeSchema)
	t.Setenv("WRAPPER_ONLY", "true")
	if _, err := loadCoreDatabaseCommandConfig(); err == nil ||
		!strings.Contains(err.Error(), "database-backed core") {
		t.Fatalf("wrapper-only error=%v", err)
	}
}
