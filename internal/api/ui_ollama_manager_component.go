package api

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/ollama"
	"github.com/gryph/omnidex/internal/ollamacatalog"
	"github.com/gryph/omnidex/internal/queue"
)

type uiOllamaManagerQuery struct {
	ModelOffset    int
	CatalogQuery   string
	CatalogPage    int
	DownloadOffset int
}

func parseUIOllamaManagerQuery(r *http.Request) (uiOllamaManagerQuery, error) {
	modelOffset, err := exactChannelQueryInteger(r, "model_offset", 0, 0, 1<<30)
	if err != nil {
		return uiOllamaManagerQuery{}, err
	}
	catalogQuery, err := exactUIQuery(r, "catalog_query", 128)
	if err != nil {
		return uiOllamaManagerQuery{}, err
	}
	catalogPage, err := exactChannelQueryInteger(r, "catalog_page", 1, 1, 100)
	if err != nil {
		return uiOllamaManagerQuery{}, err
	}
	downloadOffset, err := exactChannelQueryInteger(r, "download_offset", 0, 0, 1<<30)
	if err != nil {
		return uiOllamaManagerQuery{}, err
	}
	if catalogQuery == "" && catalogPage != 1 {
		return uiOllamaManagerQuery{}, fmt.Errorf("blank Ollama catalog search cannot select page %d", catalogPage)
	}
	return uiOllamaManagerQuery{
		ModelOffset: modelOffset, CatalogQuery: catalogQuery,
		CatalogPage: catalogPage, DownloadOffset: downloadOffset,
	}, nil
}

func (s *Server) renderUIOllamaManager(
	parent context.Context,
	query uiOllamaManagerQuery,
) (string, error) {
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()
	installed, err := s.listInstalledOllamaModels(ctx, dataSourceUIPageSize, query.ModelOffset)
	if err != nil {
		return "", fmt.Errorf("Ollama models unavailable: %w", err)
	}
	downloads := queue.OllamaModelDownloadPage{Offset: query.DownloadOffset}
	if s.ollamaDownloads != nil {
		downloads, err = s.ollamaDownloads.ListOllamaModelDownloads(
			ctx, 10, query.DownloadOffset,
		)
		if err != nil {
			return "", fmt.Errorf("Ollama download activity unavailable: %w", err)
		}
	}
	var catalogPage *ollamacatalog.Page
	if query.CatalogQuery != "" {
		page, searchErr := s.searchOllamaCatalog(ctx, query.CatalogQuery, query.CatalogPage)
		if searchErr != nil {
			return "", searchErr
		}
		catalogPage = &page
	}
	configured, err := s.ollamaConfiguredUsage(ctx, installed.Models)
	if err != nil {
		return "", err
	}
	return renderUIOllamaManager(
		s.ollamaEndpoint(), installed, configured, downloads, query, catalogPage,
	), nil
}

func (s *Server) ollamaConfiguredUsage(
	ctx context.Context,
	models []ollama.ModelInfo,
) (map[string]string, error) {
	result := make(map[string]string)
	config := s.envModelConfig()
	for _, installed := range models {
		for _, configured := range config.ModelNames() {
			if ollama.MatchesOllamaModel(configured, installed.Name) {
				result[installed.Name] = "Global routing"
			}
		}
		if s.repo == nil {
			continue
		}
		usage, err := s.repo.RoleplayOllamaModelUsage(ctx, installed.Name)
		if err != nil {
			return nil, fmt.Errorf("load character model usage: %w", err)
		}
		if usage.InUse() {
			result[installed.Name] = fmt.Sprintf(
				"Characters · %d narrative · %d voice",
				usage.NarrativeCharacters, usage.VoiceCharacters,
			)
		}
	}
	return result, nil
}

func renderUIOllamaManager(
	endpoint string,
	installed ollama.ModelPage,
	configured map[string]string,
	downloads queue.OllamaModelDownloadPage,
	query uiOllamaManagerQuery,
	catalog *ollamacatalog.Page,
) string {
	return `<div class="space-y-6">` + renderUIOllamaCatalog(query, catalog) +
		renderUIOllamaDownloads(downloads) +
		renderUIInstalledOllamaModels(endpoint, installed, configured) +
		`<p class="text-xs text-zinc-500">Assign an installed narrative model and optional voice model in <a href="/?panel=roleplay" class="text-cyan-200 underline decoration-cyan-300/30">Roleplay → Characters</a>.</p></div>`
}

func renderUIOllamaCatalog(query uiOllamaManagerQuery, page *ollamacatalog.Page) string {
	var body strings.Builder
	body.WriteString(`<div><h4 class="text-sm font-semibold text-zinc-100">Find models</h4><p class="mt-1 text-xs text-zinc-500">Searches the official Ollama catalog. Downloads remain entirely local after selection.</p>`)
	body.WriteString(`<form data-action="submit->admin#searchOllamaCatalog" class="mt-3 flex flex-wrap gap-2"><input data-admin-target="catalogQuery" value="` + uiAttribute(query.CatalogQuery) + `" placeholder="Search roleplay, story, uncensored…" class="min-w-[240px] flex-1 rounded-md border border-white/10 bg-zinc-900 px-3 py-2 text-sm" /><button type="submit" class="rounded-md bg-cyan-300 px-4 py-2 text-sm font-semibold text-zinc-950">Search catalog</button></form>`)
	if page == nil {
		body.WriteString(`</div>`)
		return body.String()
	}
	if len(page.Models) == 0 {
		body.WriteString(`<p class="mt-3 text-sm text-zinc-500">No official catalog results for “` + uiEscape(page.Query) + `”.</p></div>`)
		return body.String()
	}
	body.WriteString(`<div class="mt-3 grid gap-2 md:grid-cols-2">`)
	for _, model := range page.Models {
		body.WriteString(`<article class="rounded-md border border-white/10 bg-zinc-900/50 p-3"><div class="flex items-start justify-between gap-3"><div><a href="` + uiAttribute(model.URL) + `" target="_blank" rel="noopener noreferrer" class="font-mono text-sm text-cyan-100 hover:underline">` + uiEscape(model.Name) + `</a><p class="mt-1 text-xs leading-5 text-zinc-500">` + uiEscape(model.Description) + `</p></div><button type="button" data-action="admin#downloadCatalogModel" data-model-name="` + uiAttribute(model.Name) + `" class="shrink-0 rounded-md border border-cyan-300/30 px-2 py-1 text-xs text-cyan-100">Download</button></div></article>`)
	}
	body.WriteString(`</div><div class="mt-3 flex items-center justify-between">`)
	if page.Page > 1 {
		body.WriteString(catalogPageButton("Previous", page.Page-1))
	} else {
		body.WriteString(`<span></span>`)
	}
	body.WriteString(`<span class="text-xs text-zinc-500">Catalog page ` + strconv.Itoa(page.Page) + `</span>`)
	if page.HasMore {
		body.WriteString(catalogPageButton("Next", page.Page+1))
	} else {
		body.WriteString(`<span></span>`)
	}
	body.WriteString(`</div></div>`)
	return body.String()
}

func catalogPageButton(label string, page int) string {
	return `<button type="button" data-action="admin#loadCatalogPage" data-catalog-page="` + strconv.Itoa(page) + `" class="rounded-md border border-white/10 px-3 py-1.5 text-xs text-zinc-300">` + label + `</button>`
}
