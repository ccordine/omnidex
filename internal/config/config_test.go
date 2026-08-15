package config

import (
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/station"
)

func TestLoadRejectsRemovedPersonaModel(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	t.Setenv("WRAPPER_ONLY", "true")
	t.Setenv("LLM_PROVIDER", "ollama")
	t.Setenv("OLLAMA_MODEL_SPECIALIST_SUBTASK_EXECUTOR", "qwen3-coder:30b")

	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "OLLAMA_MODEL_SPECIALIST_SUBTASK_EXECUTOR") {
		t.Fatalf("Load() error=%v, want removed persona model failure", err)
	}
}

func TestLoadRejectsExplicitlyEmptyDatabaseSchema(t *testing.T) {
	t.Setenv("DATABASE_SCHEMA", "")
	if _, err := loadDatabaseSchema(); err == nil || !strings.Contains(err.Error(), "explicitly empty") {
		t.Fatalf("loadDatabaseSchema() error=%v, want explicit failure", err)
	}
}

func TestLoadDedicatedCodingAssemblyModels(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	t.Setenv("WRAPPER_ONLY", "true")
	t.Setenv("LLM_PROVIDER", "ollama")
	t.Setenv("OMNI_CODING_SURFACE_MODEL", "qwen3:4b-thinking")
	t.Setenv("OMNI_CODING_REQUIREMENTS_MODEL", "qwen2.5-coder:7b-requirements")
	t.Setenv("OMNI_CODING_WORKLOAD_MODEL", "qwen3.5:27b-workload")
	t.Setenv("OMNI_CODING_WORKLOAD_REVIEW_MODEL", "llama3.2:3b-review")
	t.Setenv("OMNI_CODING_ARTIFACT_HANDLING_MODEL", "qwen2.5:3b-artifact")
	t.Setenv("OMNI_CODING_CAPABILITY_RELATION_MODEL", "qwen3:4b-relation")
	t.Setenv("OMNI_CODING_SKILL_SELECTION_MODEL", "qwen3:4b-skill-selection")
	t.Setenv("OMNI_CODING_FRAGMENT_MODEL", "qwen3-coder:30b")
	t.Setenv("OMNI_CODING_FRAGMENT_CORRECTION_MODEL", "qwen2.5-coder:14b-correction")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.StationModels[station.CodingSurface]; got != "qwen3:4b-thinking" {
		t.Fatalf("coding surface model=%q want dedicated override", got)
	}
	if got := cfg.StationModels[station.CodingRequirements]; got != "qwen2.5-coder:7b-requirements" {
		t.Fatalf("coding requirements model=%q want dedicated override", got)
	}
	if got := cfg.StationModels[station.CodingWorkload]; got != "qwen3.5:27b-workload" {
		t.Fatalf("coding workload model=%q want dedicated override", got)
	}
	if got := cfg.StationModels[station.CodingWorkloadReview]; got != "llama3.2:3b-review" {
		t.Fatalf("coding workload review model=%q want independent override", got)
	}
	if got := cfg.StationModels[station.CodingArtifactHandling]; got != "qwen2.5:3b-artifact" {
		t.Fatalf("coding artifact handling model=%q want dedicated override", got)
	}
	if got := cfg.StationModels[station.CodingKnownArtifactTruth]; got != "qwen2.5:3b-artifact" {
		t.Fatalf("coding known artifact truth model=%q want shared narrow artifact override", got)
	}
	if got := cfg.StationModels[station.CodingDeclarationArtifactBoundary]; got != "qwen2.5:3b-artifact" {
		t.Fatalf("coding declaration artifact boundary model=%q want shared narrow artifact override", got)
	}
	if got := cfg.StationModels[station.CodingArtifactCandidateSelection]; got != "qwen2.5:3b-artifact" {
		t.Fatalf("coding artifact candidate selection model=%q want shared narrow artifact override", got)
	}
	if got := cfg.StationModels[station.CodingCapabilityRelation]; got != "qwen3:4b-relation" {
		t.Fatalf("coding capability relation model=%q want dedicated override", got)
	}
	if got := cfg.StationModels[station.CodingSkillSelection]; got != "qwen3:4b-skill-selection" {
		t.Fatalf("coding skill selection model=%q want dedicated override", got)
	}
	if got := cfg.StationModels[station.CodingFragment]; got != "qwen3-coder:30b" {
		t.Fatalf("coding fragment model=%q want dedicated override", got)
	}
	if got := cfg.StationModels[station.CodingFragmentCorrection]; got != "qwen2.5-coder:14b-correction" {
		t.Fatalf("coding fragment correction model=%q want dedicated override", got)
	}
	if cfg.CodingFragmentConcurrency != 1 {
		t.Fatalf("local fragment concurrency=%d want 1", cfg.CodingFragmentConcurrency)
	}
}

func TestLoadExactConversationAndWebStationModels(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	t.Setenv("WRAPPER_ONLY", "true")
	t.Setenv("LLM_PROVIDER", "ollama")
	t.Setenv("OMNI_CONVERSATION_OBJECTIVE_KIND_MODEL", "kind")
	t.Setenv("OMNI_CONVERSATION_RESPONSE_MODEL", "conversation")
	t.Setenv("OMNI_GROUNDED_ANSWER_MODEL", "grounded")
	t.Setenv("OMNI_WEB_SEARCH_TERMS_MODEL", "terms")
	t.Setenv("OMNI_WEB_RELEVANCE_MODEL", "relevance")
	t.Setenv("OMNI_WEB_GROUNDED_SYNTHESIS_MODEL", "synthesis")
	t.Setenv("OMNI_WEB_GROUNDED_SYNTHESIS_CORRECTION_MODEL", "correction")
	t.Setenv("OMNI_WEB_CLAIM_EVIDENCE_REVIEW_MODEL", "review")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	wants := map[station.ID]string{
		station.ConversationObjectiveKind:      "kind",
		station.ConversationResponse:           "conversation",
		station.GroundedAnswer:                 "grounded",
		station.WebSearchTerms:                 "terms",
		station.WebRelevance:                   "relevance",
		station.WebGroundedSynthesis:           "synthesis",
		station.WebGroundedSynthesisCorrection: "correction",
		station.WebClaimEvidenceReview:         "review",
	}
	for id, want := range wants {
		if got := cfg.StationModels[id]; got != want {
			t.Fatalf("station %s model=%q want %q", id, got, want)
		}
	}
}

func TestLoadRejectsRemovedRequirementModelEnvironmentRoutes(t *testing.T) {
	for _, key := range []string{
		"OMNI_CODING_PRODUCT_IDENTITY_MODEL",
		"OMNI_CODING_REQUIREMENT_PARTITION_MODEL",
		"OMNI_CODING_REQUIREMENT_ADVISER_MODEL",
		"OLLAMA_MODEL_SPECIALIST_CODING_REQUIREMENT_ADVISER",
		"OMNI_CODING_REQUIREMENT_SPLIT_MODEL",
		"OLLAMA_MODEL_SPECIALIST_CODING_REQUIREMENT_SPLIT",
		"OMNI_CODING_SKILL_PROCEDURE_MODEL",
	} {
		t.Run(key, func(t *testing.T) {
			t.Setenv("DATABASE_URL", "")
			t.Setenv("WRAPPER_ONLY", "true")
			t.Setenv(key, "removed-model")
			if _, err := Load(); err == nil || !strings.Contains(err.Error(), key) {
				t.Fatalf("Load() error=%v, want explicit rejection of %s", err, key)
			}
		})
	}
}

func TestLoadRejectsUnsafeCodingFragmentConcurrency(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	t.Setenv("WRAPPER_ONLY", "true")
	t.Setenv("LLM_PROVIDER", "ollama")
	t.Setenv("CODING_FRAGMENT_CONCURRENCY", "5")
	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "CODING_FRAGMENT_CONCURRENCY") {
		t.Fatalf("Load() error=%v, want fragment concurrency bound", err)
	}
}

func TestLoadRetainsUnknownProviderForLazyUseRejection(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://agent:agent@localhost:5432/agent?sslmode=disable")
	t.Setenv("LLM_PROVIDER", "something-else")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() eagerly rejected dormant provider: %v", err)
	}
	if cfg.LLMProvider != "something-else" {
		t.Fatalf("LLMProvider=%q", cfg.LLMProvider)
	}
}

func TestLoadWrapperOnlyAllowsMissingDatabaseURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	t.Setenv("WRAPPER_ONLY", "true")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if !cfg.WrapperOnly {
		t.Fatalf("WrapperOnly=%v want true", cfg.WrapperOnly)
	}
}

func TestLoadRealtimeDefaults(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	t.Setenv("WRAPPER_ONLY", "true")
	t.Setenv("REALTIME_MAX_CLIENTS", "")
	t.Setenv("REALTIME_STREAM_MAX_AGE", "")
	t.Setenv("REALTIME_HEARTBEAT", "")
	t.Setenv("REALTIME_WRITE_TIMEOUT", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.RealtimeMaxClients != 512 {
		t.Fatalf("RealtimeMaxClients=%d want 512", cfg.RealtimeMaxClients)
	}
	if cfg.RealtimeStreamMaxAge != 10*time.Minute {
		t.Fatalf("RealtimeStreamMaxAge=%s want 10m", cfg.RealtimeStreamMaxAge)
	}
	if cfg.RealtimeHeartbeat != 25*time.Second {
		t.Fatalf("RealtimeHeartbeat=%s want 25s", cfg.RealtimeHeartbeat)
	}
	if cfg.RealtimeWriteTimeout != 10*time.Second {
		t.Fatalf("RealtimeWriteTimeout=%s want 10s", cfg.RealtimeWriteTimeout)
	}
}

func TestLoadRejectsMalformedTypedEnvironment(t *testing.T) {
	tests := []struct {
		key   string
		value string
	}{
		{key: "WORKER_COUNT", value: "many"},
		{key: "INFERENCE_CONTEXT_TOKENS", value: "wide"},
		{key: "WRAPPER_ONLY", value: "perhaps"},
		{key: "REQUEST_TIMEOUT", value: "soon"},
		{key: "WEB_SEARCH_PROVIDERS", value: ",,,"},
	}

	for _, test := range tests {
		t.Run(test.key, func(t *testing.T) {
			t.Setenv("DATABASE_URL", "")
			t.Setenv("WRAPPER_ONLY", "true")
			t.Setenv(test.key, test.value)

			_, err := Load()
			if err == nil {
				t.Fatalf("Load() error=nil, want %s validation failure", test.key)
			}
			if !strings.Contains(err.Error(), test.key) {
				t.Fatalf("Load() error=%v, want %s context", err, test.key)
			}
		})
	}
}

func TestLoadRejectsOutOfRangeRuntimeSettings(t *testing.T) {
	tests := []struct {
		key   string
		value string
	}{
		{key: "WORKER_COUNT", value: "0"},
		{key: "WORKER_POLL_INTERVAL", value: "0s"},
		{key: "INFERENCE_CONTEXT_TOKENS", value: "4095"},
		{key: "REALTIME_MAX_CLIENTS", value: "0"},
		{key: "REALTIME_STREAM_MAX_AGE", value: "10s"},
		{key: "REALTIME_HEARTBEAT", value: "1s"},
		{key: "REALTIME_WRITE_TIMEOUT", value: "100ms"},
		{key: "UI_SESSION_TTL", value: "30s"},
	}

	for _, test := range tests {
		t.Run(test.key, func(t *testing.T) {
			t.Setenv("DATABASE_URL", "")
			t.Setenv("WRAPPER_ONLY", "true")
			t.Setenv(test.key, test.value)

			_, err := Load()
			if err == nil {
				t.Fatalf("Load() error=nil, want %s validation failure", test.key)
			}
			if !strings.Contains(err.Error(), test.key) {
				t.Fatalf("Load() error=%v, want %s context", err, test.key)
			}
		})
	}
}

func TestLoadRejectsRemovedDecisionEngineSettings(t *testing.T) {
	for _, key := range removedEnvironmentKeys {
		t.Run(key, func(t *testing.T) {
			t.Setenv("DATABASE_URL", "")
			t.Setenv("WRAPPER_ONLY", "true")
			t.Setenv(key, "legacy-value")

			_, err := Load()
			if err == nil || !strings.Contains(err.Error(), key) || !strings.Contains(err.Error(), "was removed") {
				t.Fatalf("Load() error=%v, want explicit %s removal failure", err, key)
			}
		})
	}
}
