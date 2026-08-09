package queue

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestContextProjectionMigrationDefinesImmutableExactAuthority(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("../../migrations/031_context_projections.sql")
	if err != nil {
		t.Fatal(err)
	}
	schema := string(raw)
	for _, required := range []string{
		"CREATE TABLE context_projections",
		"CREATE TABLE context_projection_selected_refs",
		"CREATE TABLE context_projection_omitted_refs",
		"working_set_version",
		"source_freshness",
		"authority",
		"usage_mode TEXT NOT NULL CHECK (usage_mode = 'shadow')",
		"omission_reason",
		"rendered_sha256 = encode(digest(rendered_context, 'sha256'), 'hex')",
		"context_projections_job_step_generation_fkey",
		"context_projections_working_set_fkey",
		"prevent_context_projection_mutation",
		"BEFORE UPDATE OR DELETE",
		"BEFORE TRUNCATE",
		"context_projection_id",
		"llm_call_evidence_job_step_generation_fkey",
		"llm_call_evidence_context_projection_fkey",
		"DROP TRIGGER llm_call_evidence_immutable",
		"CREATE TRIGGER llm_call_evidence_immutable",
	} {
		if !strings.Contains(schema, required) {
			t.Fatalf("context projection migration omitted %q", required)
		}
	}
	if strings.Contains(schema, "'applied'") {
		t.Fatal("context projection migration retains a write-only applied mode")
	}
}

func TestContextProjectionSourceRegistersOnlyShadowMode(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"context_projection_types.go", "context_projection_validation.go"} {
		raw, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{"ContextProjectionModeApplied", `"applied"`} {
			if strings.Contains(string(raw), forbidden) {
				t.Fatalf("context projection source %s retains unsupported mode %q", name, forbidden)
			}
		}
	}
}

func TestLLMEvidenceHasNoSynthesizedProjectionFallback(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("llm_evidence.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	for _, forbidden := range []string{
		"contextbuilder.Build(",
		"StoreContextProjection(",
		"fallbackProjection",
		"defaultProjection",
	} {
		if strings.Contains(source, forbidden) {
			t.Fatalf("LLM evidence synthesized a shadow projection through %q", forbidden)
		}
	}
}

func TestContextProjectionWorkerConsumerIsStrictlyShadowOnly(t *testing.T) {
	t.Parallel()
	paths, err := filepath.Glob("../worker/*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{"ContextProjectionModeApplied"} {
			if strings.Contains(string(raw), forbidden) {
				t.Fatalf("worker %s performed an applied context projection cutover through %q", path, forbidden)
			}
		}
	}
	raw, err := os.ReadFile("../worker/v3_repository_context_shadow.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		"ContextProjectionModeShadow", "StoreContextProjection(",
	} {
		if !strings.Contains(string(raw), required) {
			t.Fatalf("repository shadow consumer omitted %q", required)
		}
	}
}
