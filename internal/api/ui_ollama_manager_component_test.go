package api

import (
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/ollama"
	"github.com/gryph/omnidex/internal/ollamacatalog"
	"github.com/gryph/omnidex/internal/queue"
)

func TestOllamaManagerRendersSearchDownloadProgressAndInstalledUsage(t *testing.T) {
	now := time.Now().UTC()
	started := now
	html := renderUIOllamaManager(
		"http://ollama:11434",
		ollama.ModelPage{Models: []ollama.ModelInfo{{
			Name: "dolphin3:latest", Size: 4 * 1024 * 1024 * 1024,
			Details: ollama.ModelDetails{
				Family: "llama", ParameterSize: "8B", QuantizationLevel: "Q4_K_M",
			},
		}}},
		map[string]string{"dolphin3:latest": "Characters · 1 narrative · 1 voice"},
		queue.OllamaModelDownloadPage{Items: []queue.OllamaModelDownload{{
			ID: "omd_0123456789abcdef0123456789abcdef", Model: "dolphin3:latest",
			State: queue.OllamaModelDownloadRunning, Status: "pulling layer",
			TotalBytes: 100, CompletedBytes: 50, CreatedAt: now, UpdatedAt: now, StartedAt: &started,
		}}},
		uiOllamaManagerQuery{CatalogQuery: "dolphin", CatalogPage: 1},
		&ollamacatalog.Page{Query: "dolphin", Page: 1, HasMore: true, Models: []ollamacatalog.Model{{
			Name: "dolphin3", Description: "A local story model.", URL: "https://ollama.com/library/dolphin3",
		}}},
	)
	for _, required := range []string{
		`data-action="submit->admin#searchOllamaCatalog"`,
		`data-admin-target="catalogQuery"`,
		`data-action="admin#downloadCatalogModel"`,
		`data-model-name="dolphin3"`,
		`data-action="admin#loadCatalogPage"`,
		`Download activity`, `chat-typing-dot`, `50.0%`,
		`dolphin3:latest`, `llama · 8B · Q4_K_M`,
		`Characters · 1 narrative · 1 voice`,
		`Roleplay → Characters`,
	} {
		if !strings.Contains(html, required) {
			t.Errorf("Ollama manager is missing %q: %s", required, html)
		}
	}
}
