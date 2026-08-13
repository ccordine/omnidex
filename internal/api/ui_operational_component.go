package api

import (
	"html"
	"net/http"
	"strconv"
	"strings"
)

type uiOperationalComponent struct {
	HTML chatComponentHTML `json:"html"`
}

func writeUIOperationalComponent(w http.ResponseWriter, target, body string) {
	writeChatComponentJSON(w, uiOperationalComponent{
		HTML: chatComponentHTML{Bundle: renderRecyclrTemplateHTML(target, body, "innerHTML")},
	})
}

func uiEscape(value string) string {
	return html.EscapeString(value)
}

func uiAttribute(value string) string {
	return html.EscapeString(strings.TrimSpace(value))
}

func uiInt(value int64) string {
	return strconv.FormatInt(value, 10)
}

func uiError(message string) string {
	return `<p role="alert" class="rounded-md border border-rose-400/30 bg-rose-400/10 px-3 py-2 text-sm text-rose-200">` +
		uiEscape(message) + `</p>`
}

func renderUIDataPagination(action, kind string, offset, count int, hasMore bool) string {
	if offset == 0 && !hasMore {
		return ""
	}
	var body strings.Builder
	body.WriteString(`<nav class="mt-3 flex items-center justify-between gap-2" aria-label="` + uiAttribute(kind) + ` pages">`)
	if offset > 0 {
		previous := offset - dataSourceUIPageSize
		if previous < 0 {
			previous = 0
		}
		body.WriteString(`<button type="button" data-action="` + uiAttribute(action) + `" data-page-kind="` + uiAttribute(kind) + `" data-page-offset="` + strconv.Itoa(previous) + `" class="rounded-md border border-white/10 px-3 py-1.5 text-xs">Previous</button>`)
	}
	if hasMore {
		body.WriteString(`<button type="button" data-action="` + uiAttribute(action) + `" data-page-kind="` + uiAttribute(kind) + `" data-page-offset="` + strconv.Itoa(offset+count) + `" class="ml-auto rounded-md border border-white/10 px-3 py-1.5 text-xs">Next</button>`)
	}
	body.WriteString(`</nav>`)
	return body.String()
}

func uiLoading(message string) string {
	return `<div class="flex min-h-[4rem] items-center gap-2 rounded-md border border-white/10 bg-zinc-950/40 px-3 py-3 text-sm text-zinc-400" role="status" aria-live="polite">` +
		`<span class="inline-block h-4 w-4 shrink-0 animate-spin rounded-full border-2 border-cyan-300/25 border-t-cyan-200" aria-hidden="true"></span>` +
		`<span>` + uiEscape(message) + `</span></div>`
}

func exactUIQuery(r *http.Request, name string, maxBytes int) (string, error) {
	return exactChatComponentString(r, name, maxBytes)
}
