package modelconfig

import "fmt"

var removedEnvironmentKeys = [...]string{
	"OMNI_CODING_REQUIREMENT_ADVISER_MODEL",
	"OLLAMA_MODEL_SPECIALIST_CODING_REQUIREMENT_ADVISER",
	"OMNI_CODING_REQUIREMENT_SPLIT_MODEL",
	"OLLAMA_MODEL_SPECIALIST_CODING_REQUIREMENT_SPLIT",
}

func RemovedEnvironmentKeys() []string {
	keys := make([]string, len(removedEnvironmentKeys))
	copy(keys, removedEnvironmentKeys[:])
	return keys
}

func ValidateEnvironmentValues(values map[string]string) error {
	for _, key := range removedEnvironmentKeys {
		if _, exists := values[key]; exists {
			return fmt.Errorf("model environment setting %s was removed and is unsupported; delete this setting", key)
		}
	}
	return nil
}
