package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gryph/omnidex/internal/codex"
	"github.com/gryph/omnidex/internal/config"
)

func TestAgentCatalogPreservesProviderIdentityForSameModelName(t *testing.T) {
	local := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/tags" {
			t.Errorf("unexpected model operation: %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"models":[{"name":"same-model"}]}`))
	}))
	defer local.Close()
	server := &Server{providerConfig: config.Config{OllamaBaseURL: local.URL}}
	models, err := server.codingAgentModelCatalog(context.Background(), []codex.Model{{ID: "same-model"}})
	if err != nil || len(models) != 2 || models[0].Provider != "codex" || models[1].Provider != "ollama" ||
		models[0].ID != models[1].ID || len(models[1].Efforts) != 0 || len(models[1].ServiceTiers) != 0 {
		t.Fatalf("provider model identities or capabilities were merged: %+v %v", models, err)
	}
}
