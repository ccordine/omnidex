package ollama

import "testing"

func TestMatchesOllamaModelUntaggedConfigMatchesLatest(t *testing.T) {
	if !MatchesOllamaModel("nomic-embed-text", "nomic-embed-text:latest") {
		t.Fatal("expected untagged config to match tagged install")
	}
}

func TestMatchesOllamaModelExactTagMatch(t *testing.T) {
	if !MatchesOllamaModel("qwen2.5-coder:7b", "qwen2.5-coder:7b") {
		t.Fatal("expected exact tag match")
	}
}

func TestMatchesOllamaModelDifferentTagsDoNotMatch(t *testing.T) {
	if MatchesOllamaModel("qwen2.5-coder:7b", "qwen2.5-coder:14b") {
		t.Fatal("expected different tags not to match")
	}
}

func TestModelIsAvailable(t *testing.T) {
	installed := []string{"qwen2.5-coder:7b", "nomic-embed-text:latest"}
	if !ModelIsAvailable(installed, "nomic-embed-text") {
		t.Fatal("expected embedding model to be available")
	}
	if ModelIsAvailable(installed, "llama3.2") {
		t.Fatal("expected missing model to be unavailable")
	}
}
