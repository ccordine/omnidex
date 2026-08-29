package api

import (
	"fmt"
	"html"
	"strconv"
	"strings"
	"unicode/utf8"
)

type chatComponentHTML struct {
	Bundle string `json:"bundle"`
}

type chatComponentPage struct {
	NextOffset *int              `json:"next_offset,omitempty"`
	HasMore    bool              `json:"has_more"`
	HTML       chatComponentHTML `json:"html"`
}

func boundedChatComponentPage[T any](items []T, limit, offset int) ([]T, *int, bool) {
	hasMore := len(items) > limit
	if !hasMore {
		return items, nil, false
	}
	items = items[:limit]
	next := offset + limit
	return items, &next, true
}

func chatPaginationButton(action, controls, section string, nextOffset *int, label string) string {
	if nextOffset == nil {
		return ""
	}
	return `<button type="button" data-action="chat#` + html.EscapeString(action) +
		`" data-next-offset="` + strconv.Itoa(*nextOffset) + `" data-page-section="` +
		html.EscapeString(section) + `" aria-controls="` + html.EscapeString(controls) +
		`" class="rounded-md border border-white/10 px-3 py-1.5 text-xs font-semibold text-zinc-300 transition hover:border-cyan-300/40 hover:bg-cyan-300/10 disabled:cursor-wait disabled:opacity-60">` +
		html.EscapeString(label) + `</button>`
}

func chatStatusPillClass(status string) (string, error) {
	base := "rounded-full border px-2 py-1 text-[11px] font-medium "
	switch status {
	case "completed", "approved", "durable":
		return base + "border-emerald-300/30 bg-emerald-300/10 text-emerald-100", nil
	case "running":
		return base + "border-cyan-300/30 bg-cyan-300/10 text-cyan-100", nil
	case "pending", "waiting_input", "candidate":
		return base + "border-amber-300/30 bg-amber-300/10 text-amber-100", nil
	case "failed", "canceled", "rejected":
		return base + "border-rose-300/30 bg-rose-300/10 text-rose-100", nil
	default:
		return "", fmt.Errorf("unsupported chat presentation status %q", status)
	}
}

func chatEmptyState(message string) string {
	return `<p role="status" class="rounded-md border border-dashed border-white/10 px-4 py-6 text-center text-sm text-zinc-500">` +
		html.EscapeString(message) + `</p>`
}

func validateChatText(value, name string, maxBytes int) error {
	if len(value) == 0 || len(value) > maxBytes || !utf8.ValidString(value) || strings.IndexByte(value, 0) >= 0 {
		return fmt.Errorf("%s must contain 1..%d valid UTF-8 bytes without NUL", name, maxBytes)
	}
	return nil
}
