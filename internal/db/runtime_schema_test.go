package db

import (
	"context"
	"strings"
	"testing"
)

func TestRuntimeSchemaNameRequiresDedicatedIdentifier(t *testing.T) {
	for _, invalid := range []string{
		"", "public", "information_schema", "pg_catalog", "Omnidex", "omnidex-runtime",
	} {
		if err := ValidateRuntimeSchemaName(invalid); err == nil {
			t.Fatalf("runtime schema %q was accepted", invalid)
		}
	}
	if err := ValidateRuntimeSchemaName(DefaultRuntimeSchema); err != nil {
		t.Fatal(err)
	}
}

func TestRuntimeSearchPathHasOneExactAuthority(t *testing.T) {
	got, err := RuntimeSearchPath(DefaultRuntimeSchema)
	if err != nil {
		t.Fatal(err)
	}
	if got != "omnidex_runtime,public" {
		t.Fatalf("runtime search_path=%q", got)
	}
	if _, err := RuntimeSearchPath("public"); err == nil {
		t.Fatal("public schema produced a runtime search_path")
	}
}

func TestRuntimeConnectionRejectsURLSearchPathAuthority(t *testing.T) {
	for _, databaseURL := range []string{
		"postgres://user:password@127.0.0.1/database?search_path=other",
		"postgres://user:password@127.0.0.1/database?options=-csearch_path%3Dother",
	} {
		if _, err := ConnectRuntime(
			context.Background(), databaseURL, DefaultRuntimeSchema,
		); err == nil {
			t.Fatal("URL-level search_path was accepted beside DATABASE_SCHEMA")
		}
	}
}

func TestRuntimeSchemaBootstrapRejectsLegacyPublicState(t *testing.T) {
	if err := rejectPublicOmnidexState(false); err != nil {
		t.Fatal(err)
	}
	if err := rejectPublicOmnidexState(true); err == nil {
		t.Fatal("legacy public Omnidex state was accepted beside a new runtime schema")
	}
}

func TestConnectRuntimeReadOnlyRejectsInvalidSchemaBeforeOpeningConnection(t *testing.T) {
	_, err := ConnectRuntimeReadOnly(context.Background(), "postgres://127.0.0.1:1/omnidex", "public")
	if err == nil || !strings.Contains(err.Error(), "runtime database schema") {
		t.Fatalf("error=%v, want invalid runtime schema", err)
	}
}
