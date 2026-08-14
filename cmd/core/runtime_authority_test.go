package main

import (
	"os"
	"strings"
	"testing"
)

func TestCoreValidatesRuntimeAuthorityBeforeServerAndWorkerConstruction(t *testing.T) {
	raw, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	validation := strings.Index(source, "repo.ValidateRuntimeAuthority(ctx)")
	migrations := strings.Index(source, "repo.EnsureSchema(ctx, migrationBundle)")
	server := strings.Index(source, "api.NewServerWithOptions")
	worker := strings.Index(source, "worker.New(")
	if validation < 0 || migrations < 0 || server < 0 || worker < 0 ||
		validation < migrations || validation > server || validation > worker {
		t.Fatalf("runtime authority validation order is invalid: migrations=%d validation=%d server=%d worker=%d", migrations, validation, server, worker)
	}
}
