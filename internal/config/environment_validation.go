package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/llmprovider/catalog"
	"github.com/gryph/omnidex/internal/modelconfig"
)

var integerEnvironmentKeys = []string{
	"WEB_SEARCH_PER_SOURCE_BUDGET",
	"WEB_SEARCH_TOTAL_BUDGET",
	"WORKER_COUNT",
	"CODING_FRAGMENT_CONCURRENCY",
	"REALTIME_MAX_CLIENTS",
	"INFERENCE_CONTEXT_TOKENS",
}

var booleanEnvironmentKeys = []string{
	"WRAPPER_ONLY",
	"UI_REDIS_REQUIRED",
	"MIGRATE_ON_STARTUP",
}

var durationEnvironmentKeys = []string{
	"WEB_SEARCH_TIMEOUT",
	"WORKER_POLL_INTERVAL",
	"REQUEST_TIMEOUT",
	"REALTIME_STREAM_MAX_AGE",
	"REALTIME_HEARTBEAT",
	"REALTIME_WRITE_TIMEOUT",
	"UI_SESSION_TTL",
}

var removedEnvironmentKeys = buildRemovedEnvironmentKeys()

func buildRemovedEnvironmentKeys() []string {
	keys := append([]string{
		"APP_ENV",
		"RETRIEVAL_LIMIT",
		"CONTEXT_CHAR_BUDGET",
		"WORKSPACE_MAX_FILES",
		"WORKSPACE_CONTEXT_BUDGET",
		"MIGRATIONS_DIR",
		"OMNIDEX_V3_ENABLED",
		"STOP_ON_SUFFICIENT_CONTEXT",
		"SUFFICIENT_CONTEXT_CHARS",
		"MEMORY_INFERENCE_ENABLED",
		"MEMORY_INFERENCE_MAX_ITEMS",
		"TOURNAMENT_ENABLED",
		"TOURNAMENT_CHUNK_CHARS",
		"TOURNAMENT_SUMMARY_CHARS",
		"TOURNAMENT_MAX_ROUNDS",
		"TOURNAMENT_VERIFY_RELEVANCE",
		"HALLUCINATION_RETRY_LIMIT",
		"OLLAMA_RESTART_COMMAND",
		"OLLAMA_RESTART_TIMEOUT",
		"COGNITION_MODEL_SHA256",
		"COGNITION_MODEL_QUANTIZATION",
		"COGNITION_BACKEND_VERSION",
		"COGNITION_HARDWARE",
		"COGNITION_CONTEXT_CEILING_BYTES",
		"COGNITION_MAX_OUTPUT_TOKENS",
		"WEB_SEARCH_ENABLED",
		"WORKSPACE_SCAN_ENABLED",
		"ANTHROPIC_VERSION",
		"ANTHROPIC_MAX_TOKENS",
		"OMNI_TICKET_CONTEXT_DEADLINE",
		"OMNI_OBJECTIVE_ADVISORY_MODE",
		"OMNI_CONTEXT_RELEVANCE_PROVIDER",
	}, modelconfig.RemovedEnvironmentKeys()...)
	for _, definition := range catalog.Definitions() {
		if definition.SupportsExactPreparedStations || definition.SupportsEmbeddings {
			continue
		}
		keys = append(keys, definition.APIKeyEnvironmentKeys...)
		keys = append(keys, definition.BaseURLEnvironmentKeys...)
		keys = append(keys, definition.EnvironmentKeys("EMBEDDING_MODEL")...)
	}
	return keys
}

func validateTypedEnvironment() error {
	for _, key := range removedEnvironmentKeys {
		if _, exists := os.LookupEnv(key); exists {
			return fmt.Errorf("%s was removed and is unsupported; delete this setting", key)
		}
	}
	for _, key := range integerEnvironmentKeys {
		value, present := nonEmptyEnvironmentValue(key)
		if !present {
			continue
		}
		if _, err := strconv.Atoi(value); err != nil {
			return fmt.Errorf("%s must be an integer, received %q", key, value)
		}
	}
	for _, key := range booleanEnvironmentKeys {
		value, present := nonEmptyEnvironmentValue(key)
		if !present {
			continue
		}
		if _, err := strconv.ParseBool(value); err != nil {
			return fmt.Errorf("%s must be a boolean, received %q", key, value)
		}
	}
	for _, key := range durationEnvironmentKeys {
		value, present := nonEmptyEnvironmentValue(key)
		if !present {
			continue
		}
		if _, err := time.ParseDuration(value); err != nil {
			return fmt.Errorf("%s must be a Go duration, received %q", key, value)
		}
	}
	if value, present := nonEmptyEnvironmentValue("WEB_SEARCH_PROVIDERS"); present {
		providers := strings.Split(value, ",")
		validCount := 0
		for _, provider := range providers {
			if strings.TrimSpace(provider) != "" {
				validCount++
			}
		}
		if validCount == 0 {
			return fmt.Errorf("WEB_SEARCH_PROVIDERS must contain at least one provider")
		}
	}
	return nil
}

func nonEmptyEnvironmentValue(key string) (string, bool) {
	value, exists := os.LookupEnv(key)
	value = strings.TrimSpace(value)
	return value, exists && value != ""
}
