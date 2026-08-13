package api

import (
	"fmt"
	"html"
	"strconv"
	"strings"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
)

const (
	chatMemoryListTarget                = "memory-list"
	chatMemoryPaginationTarget          = "memory-pagination"
	chatMemoryCandidatesTarget          = "memory-candidates"
	chatMemoryCandidatePaginationTarget = "memory-candidates-pagination"
)

type chatMemorySectionPage struct {
	NextOffset *int `json:"next_offset,omitempty"`
	HasMore    bool `json:"has_more"`
}

type chatMemoryPage struct {
	Memory     *chatMemorySectionPage `json:"memory,omitempty"`
	Candidates *chatMemorySectionPage `json:"candidates,omitempty"`
	HTML       chatComponentHTML      `json:"html"`
}

func renderChatMemoryPage(
	section string,
	memory queue.MemoryChunkPage,
	candidates queue.MemoryCandidatePage,
	appendItems bool,
) (chatMemoryPage, error) {
	response := chatMemoryPage{}
	location := "innerHTML"
	if appendItems {
		location = "beforeend"
	}
	var bundle strings.Builder
	if section == "all" || section == "memory" {
		markup, err := renderChatMemoryItems(memory.Items, appendItems)
		if err != nil {
			return chatMemoryPage{}, err
		}
		bundle.WriteString(renderRecyclrTemplateHTML(chatMemoryListTarget, markup, location))
		bundle.WriteString(renderRecyclrTemplateHTML(chatMemoryPaginationTarget, chatPaginationButton(
			"loadMoreMemory", chatMemoryListTarget, "memory", memory.NextOffset, "Load more memory",
		), "innerHTML"))
		response.Memory = &chatMemorySectionPage{NextOffset: memory.NextOffset, HasMore: memory.HasMore}
	}
	if section == "all" || section == "candidates" {
		markup, err := renderChatMemoryCandidates(candidates.Items, appendItems)
		if err != nil {
			return chatMemoryPage{}, err
		}
		bundle.WriteString(renderRecyclrTemplateHTML(chatMemoryCandidatesTarget, markup, location))
		bundle.WriteString(renderRecyclrTemplateHTML(chatMemoryCandidatePaginationTarget, chatPaginationButton(
			"loadMoreMemory", chatMemoryCandidatesTarget, "candidates", candidates.NextOffset, "Load more candidates",
		), "innerHTML"))
		response.Candidates = &chatMemorySectionPage{
			NextOffset: candidates.NextOffset, HasMore: candidates.HasMore,
		}
	}
	response.HTML.Bundle = bundle.String()
	if response.HTML.Bundle == "" {
		return chatMemoryPage{}, fmt.Errorf("chat memory section %q is unsupported", section)
	}
	return response, nil
}

func renderChatMemoryItems(items []queue.MemoryChunkSummary, appendItems bool) (string, error) {
	if len(items) == 0 && !appendItems {
		return chatEmptyState("No durable memory chunks found."), nil
	}
	var output strings.Builder
	for _, item := range items {
		if item.ID < 1 || item.CreatedAt.IsZero() {
			return "", fmt.Errorf("memory presentation requires positive identity and timestamp")
		}
		if err := validateChatText(string(item.Kind), "memory kind", 64); err != nil {
			return "", err
		}
		if err := validateChatText(item.Source, "memory source", 1024); err != nil {
			return "", err
		}
		if err := validateChatText(item.Content, "memory content", 1024*1024); err != nil {
			return "", err
		}
		tags, err := renderChatMemoryTags(item.Tags)
		if err != nil {
			return "", err
		}
		output.WriteString(`<article class="rounded-lg border border-white/10 bg-zinc-950/50 p-4">` +
			`<div class="flex flex-wrap items-center justify-between gap-3"><span class="font-mono text-xs text-cyan-200">memory #` +
			strconv.FormatInt(item.ID, 10) + `</span><span class="rounded-full border border-white/10 px-2 py-1 text-[11px] text-zinc-300">` +
			html.EscapeString(string(item.Kind)) + `</span></div><div class="mt-2 text-xs text-zinc-500">` +
			html.EscapeString(item.Source) + ` · ` + html.EscapeString(item.CreatedAt.UTC().Format("2006-01-02 15:04 UTC")) +
			`</div><p class="mt-2 whitespace-pre-wrap break-words text-sm leading-6 text-zinc-200">` +
			html.EscapeString(item.Content) + `</p>` + tags +
			`<div class="mt-4"><button type="button" data-action="chat#deleteMemory" data-memory-id="` +
			strconv.FormatInt(item.ID, 10) + `" class="rounded-md border border-rose-300/30 px-3 py-1.5 text-xs font-semibold text-rose-200 hover:bg-rose-400/10">Remove</button></div></article>`)
	}
	return output.String(), nil
}

func renderChatMemoryCandidates(items []model.MemoryCandidate, appendItems bool) (string, error) {
	if len(items) == 0 && !appendItems {
		return chatEmptyState("No memory candidates found."), nil
	}
	var output strings.Builder
	for _, item := range items {
		if item.ID < 1 || item.CreatedAt.IsZero() {
			return "", fmt.Errorf("memory candidate presentation requires positive identity and timestamp")
		}
		pill, err := chatStatusPillClass(item.Status)
		if err != nil {
			return "", err
		}
		if err := validateChatText(string(item.CandidateKind), "candidate kind", 128); err != nil {
			return "", err
		}
		if err := validateChatText(item.Content, "candidate content", 1024*1024); err != nil {
			return "", err
		}
		id := strconv.FormatInt(item.ID, 10)
		authority := "historical_generation"
		if item.JobID == 0 {
			authority = "global"
		}
		approve, err := chatCandidateButton("promoteMemory", id, "approved", authority, "Approve", "cyan")
		if err != nil {
			return "", err
		}
		durable, err := chatCandidateButton("promoteMemory", id, "durable", authority, "Durable", "emerald")
		if err != nil {
			return "", err
		}
		reject, err := chatCandidateButton("rejectMemory", id, "", authority, "Reject", "rose")
		if err != nil {
			return "", err
		}
		remove, err := chatCandidateButton("deleteMemoryCandidate", id, "", authority, "Delete", "zinc")
		if err != nil {
			return "", err
		}
		output.WriteString(`<article class="rounded-lg border border-white/10 bg-zinc-950/50 p-4"><div class="flex flex-wrap items-center justify-between gap-3">` +
			`<span class="font-mono text-xs text-cyan-200">candidate #` + id + `</span><span class="` + pill + `">` +
			html.EscapeString(item.Status) + `</span></div><div class="mt-2 text-xs uppercase tracking-[.16em] text-zinc-500">` +
			html.EscapeString(string(item.CandidateKind)) + `</div><p class="mt-2 whitespace-pre-wrap break-words text-sm leading-6 text-zinc-200">` +
			html.EscapeString(item.Content) + `</p><div class="mt-4 flex flex-wrap gap-2">` +
			approve + durable + reject + remove + `</div></article>`)
	}
	return output.String(), nil
}

func renderChatMemoryTags(tags []string) (string, error) {
	if len(tags) == 0 {
		return "", nil
	}
	if len(tags) > 64 {
		return "", fmt.Errorf("memory tags exceed the 64-item presentation bound")
	}
	var output strings.Builder
	output.WriteString(`<div class="mt-3 flex flex-wrap gap-1">`)
	for _, tag := range tags {
		if err := validateChatText(tag, "memory tag", 256); err != nil {
			return "", err
		}
		output.WriteString(`<span class="rounded bg-white/[.06] px-2 py-1 font-mono text-[11px] text-zinc-400">` +
			html.EscapeString(tag) + `</span>`)
	}
	output.WriteString(`</div>`)
	return output.String(), nil
}

func chatCandidateButton(action, id, tier, authority, label, color string) (string, error) {
	tierAttribute := ""
	if tier != "" {
		tierAttribute = ` data-tier="` + html.EscapeString(tier) + `"`
	}
	classes := map[string]string{
		"cyan":    "border-cyan-300/30 text-cyan-100",
		"emerald": "border-emerald-300/30 text-emerald-100",
		"rose":    "border-rose-300/30 text-rose-100",
		"zinc":    "border-zinc-300/30 text-zinc-100",
	}
	className, exists := classes[color]
	if !exists {
		return "", fmt.Errorf("unsupported memory action color %q", color)
	}
	return `<button type="button" data-action="chat#` + action + `" data-candidate-id="` + id +
		`" data-authority="` + html.EscapeString(authority) + `"` +
		tierAttribute + ` class="rounded-md border px-3 py-2 text-xs font-semibold ` + className + `">` + label + `</button>`, nil
}
