package worker

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/artifacts"
	"github.com/gryph/omnidex/internal/model"
)

func TestRuntimeV3ExternalResearchPrefersSearchQueryMetadata(t *testing.T) {
	job := model.Job{Instruction: "long instruction", Metadata: json.RawMessage(`{"search_query":"focused docs query"}`)}
	if got := externalResearchQuery(job, nil); got != "focused docs query" {
		t.Fatalf("query=%q", got)
	}
}

func TestGenericNonAnswerDetectsClarificationTemplates(t *testing.T) {
	cases := []string{
		"Understood! Please let me know what specific output you need.",
		"Sure, I can do that. What is the output you need?",
		"Could you clarify what you want?",
		"Understood. I will return only the requested output as per your instructions. If you have any questions or need further assistance, feel free to ask!",
		"Sure, please provide me with the details of what you need the output to be.",
		"Sure, please specify what you need me to return.",
		"Sure, please provide the details of what you need me to return.",
	}
	for _, tc := range cases {
		if !genericNonAnswer(tc) {
			t.Fatalf("genericNonAnswer(%q)=false, want true", tc)
		}
	}
	if genericNonAnswer("Tokio's spawn_blocking is intended for blocking operations; CPU-bound work may need a separate executor.") {
		t.Fatalf("genericNonAnswer rejected a substantive response")
	}
}

func TestBestV3FinalFallbackPrefersSubstantiveSubtask(t *testing.T) {
	got := bestV3FinalFallback(
		model.Job{Instruction: "answer from stored memory"},
		map[string]string{
			"analysis":  "Understood! Please let me know what specific output you need.",
			"workspace": strings.Repeat("irrelevant workspace context ", 50),
		},
		[]string{"Tokio uses spawn_blocking for blocking operations and recommends a separate CPU-bound executor for CPU-heavy work."},
		"No relevant summary.",
	)
	if got == "" || genericNonAnswer(got) {
		t.Fatalf("fallback returned non-answer: %q", got)
	}
	if got != "Tokio uses spawn_blocking for blocking operations and recommends a separate CPU-bound executor for CPU-heavy work." {
		t.Fatalf("fallback=%q", got)
	}
}

func TestCollectSubtaskOutputsIncludesMutableRuntimeContexts(t *testing.T) {
	rt := &nativeRuntimeV3{
		contexts: map[string]string{"subtask:t1": "fresh subtask output"},
		claim: &model.ClaimedStep{Contexts: []model.StepContext{
			{Key: "subtask:t0", Value: "stored subtask output"},
			{Key: "analysis", Value: "not a subtask"},
		}},
	}
	got := rt.collectSubtaskOutputs()
	if len(got) != 2 {
		t.Fatalf("subtask outputs=%#v, want 2", got)
	}
}

func TestSourceURLConstrainedFallbackUsesOnlyStoredURLs(t *testing.T) {
	got := sourceURLConstrainedFallback("cite only source_url values exactly for modules and async", []string{
		"source_url=https://developer.mozilla.org/en-US/docs/Web/JavaScript/Reference\nbody",
		"source_url=https://nodejs.org/api/\nbody",
		"https://tc39.es/ecma262/",
	})
	for _, want := range []string{
		"https://developer.mozilla.org/en-US/docs/Web/JavaScript/Reference",
		"https://nodejs.org/api/",
		"CLI author notes:",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("fallback missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "tc39.es") {
		t.Fatalf("fallback included untagged URL:\n%s", got)
	}
}

func TestRetrievalArtifactTextIncludesItemContent(t *testing.T) {
	got := retrievalArtifactText(artifacts.RetrievalArtifact{
		Summary: "summary only",
		Items: []artifacts.RetrievalItem{
			{Content: "source_url=https://nodejs.org/api/\nnode api"},
		},
	})
	if !strings.Contains(got, "summary only") || !strings.Contains(got, "https://nodejs.org/api/") {
		t.Fatalf("retrieval text missing expected content:\n%s", got)
	}
}
