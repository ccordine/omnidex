package main

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"
)

type sealedMigrationFailingWriter struct{}

func (sealedMigrationFailingWriter) Write([]byte) (int, error) {
	return 0, errors.New("receipt sink unavailable")
}

func TestCoreExposesOneSealedDatabaseMigrationCommand(t *testing.T) {
	mainSource, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	const command = `database:migrate-sealed`
	if strings.Count(string(mainSource), command) != 1 {
		t.Fatalf("sealed database migration command count=%d want 1",
			strings.Count(string(mainSource), command))
	}
	source, err := os.ReadFile("sealed_database_migration.go")
	if err != nil {
		t.Fatal(err)
	}
	raw := string(source)
	migrate := strings.Index(raw, "repository.EnsureSchema(ctx, bundle)")
	validate := strings.Index(raw, "repository.ValidateRuntimeAuthority(ctx)")
	printReceipt := strings.Index(raw, "writeSealedDatabaseMigrationReceipt(output, bundle.ManifestSHA256())")
	if migrate < 0 || validate <= migrate || printReceipt <= validate {
		t.Fatalf(
			"sealed migration order is invalid: migrate=%d validate=%d receipt=%d",
			migrate, validate, printReceipt,
		)
	}
	for _, forbidden := range []string{"worker.New(", "api.NewServerWithOptions", "MIGRATE_ON_STARTUP"} {
		if strings.Contains(raw, forbidden) {
			t.Fatalf("sealed migration command starts unrelated runtime %q", forbidden)
		}
	}
}

func TestSealedDatabaseMigrationReceiptIsExactAndFailLoud(t *testing.T) {
	const manifest = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	var output bytes.Buffer
	if err := writeSealedDatabaseMigrationReceipt(&output, manifest); err != nil {
		t.Fatal(err)
	}
	want := `{"schema":"omnidex.sealed-database-migration-receipt.v1","manifest_sha256":"` +
		manifest + `","runtime_authority_valid":true}` + "\n"
	if output.String() != want {
		t.Fatalf("sealed migration receipt=%q want %q", output.String(), want)
	}
	if err := writeSealedDatabaseMigrationReceipt(sealedMigrationFailingWriter{}, manifest); err == nil ||
		!strings.Contains(err.Error(), "write sealed migration receipt") {
		t.Fatalf("receipt write error=%v", err)
	}
	if err := writeSealedDatabaseMigrationReceipt(nil, manifest); err == nil ||
		!strings.Contains(err.Error(), "receipt output is required") {
		t.Fatalf("nil receipt output error=%v", err)
	}
}
