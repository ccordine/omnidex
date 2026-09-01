package main

import (
	"os"
	"strings"
)

func defaultOllamaBaseURL() string {
	if value := strings.TrimSpace(os.Getenv("OLLAMA_BASE_URL")); value != "" {
		return value
	}
	return "http://localhost:11434"
}
