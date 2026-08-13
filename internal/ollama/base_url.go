package ollama

import "github.com/gryph/omnidex/internal/dockerhost"

// NormalizeBaseURL normalizes one explicitly configured Ollama transport URL.
func NormalizeBaseURL(raw string) string {
	return dockerhost.NormalizeBaseURL(raw)
}
