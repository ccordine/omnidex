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

func TestContextProjectionSourceHasNoCallerControlledUsageMode(t *testing.T) {
	t.Parallel()
	for _, name := range []string{
		"context_projection_types.go",
		"context_projection_validation.go",
		"context_projection_store.go",
		"context_projection_load.go",
		"context_projection_list.go",
	} {
		raw, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{
			"ContextProjectionMode",
			"authority.Mode",
			"ContextProjectionModeShadow",
			"ContextProjectionModeApplied",
		} {
			if strings.Contains(string(raw), forbidden) {
				t.Fatalf("context projection source %s retains caller-controlled mode through %q", name, forbidden)
			}
		}
	}
	store, err := os.ReadFile("context_projection_store.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(store), "$8,$9,'live',$10") {
		t.Fatal("context projection store does not hard-code the sole durable live usage mode")
	}
	for _, name := range []string{"context_projection_load.go", "context_projection_list.go"} {
		raw, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		for _, required := range []string{
			"var usageMode string",
			"&usageMode",
			"requireLiveContextProjectionUsageMode(usageMode)",
		} {
			if !strings.Contains(string(raw), required) {
				t.Fatalf("context projection source %s omitted strict durable usage check %q", name, required)
			}
		}
	}
}

func TestLiveContextProjectionMigrationAddsOneExplicitMode(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("../../migrations/043_live_context_projections.sql")
	if err != nil {
		t.Fatal(err)
	}
	schema := string(raw)
	for _, required := range []string{
		"LOCK TABLE context_projections IN ACCESS EXCLUSIVE MODE",
		"WHERE usage_mode <> 'shadow'",
		"DROP CONSTRAINT context_projections_usage_mode_check",
		"CHECK (usage_mode IN ('shadow','live'))",
	} {
		if !strings.Contains(schema, required) {
			t.Fatalf("live context projection migration omitted %q", required)
		}
	}
	if strings.Contains(schema, "'applied'") || strings.Contains(schema, "transcript") {
		t.Fatal("live context projection migration introduced a fallback mode")
	}
}

func TestContextProjectionSourceLineageIsNormalizedSealedAndImmutable(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("../../migrations/048_context_projection_source_lineage.sql")
	if err != nil {
		t.Fatal(err)
	}
	schema := string(raw)
	for _, required := range []string{
		"CREATE TABLE context_projection_selected_source_refs",
		"source_ref_count",
		"source_refs_sealed_at",
		"guard_context_projection_selected_source_insert",
		"context projection selected source authority is sealed",
		"context_projection_selected_source_immutable",
		"BEFORE UPDATE OR DELETE",
		"BEFORE TRUNCATE",
	} {
		if !strings.Contains(schema, required) {
			t.Fatalf("context projection source-lineage migration omitted %q", required)
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

func TestWorkerHasNoOutputBlindContextProjectionConsumer(t *testing.T) {
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
		for _, forbidden := range []string{
			"ContextProjectionModeApplied",
			"ContextProjectionModeShadow",
			"prepareRepositoryShadowContext",
			"repositoryShadow",
		} {
			if strings.Contains(string(raw), forbidden) {
				t.Fatalf("worker %s retains an output-blind context projection consumer through %q", path, forbidden)
			}
		}
	}
}
