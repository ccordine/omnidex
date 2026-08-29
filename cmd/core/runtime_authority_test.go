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
	reset := strings.Index(source, "repo.ResetDatabase(ctx, databaseSetup)")
	server := strings.Index(source, "api.NewServerWithOptions")
	worker := strings.Index(source, "worker.New(")
	if validation < 0 || reset < 0 || server < 0 || worker < 0 ||
		validation < reset || validation > server || validation > worker {
		t.Fatalf("runtime authority validation order is invalid: reset=%d validation=%d server=%d worker=%d", reset, validation, server, worker)
	}
}
