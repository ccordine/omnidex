package api

import (
	"fmt"
	"html"
	"strconv"
	"strings"
)

func renderChatJobProgressEvents(events []chatProgressEvent) (string, error) {
	if len(events) > 24 {
		return "", fmt.Errorf("chat progress component exceeds its 24-event presentation bound")
	}
	if len(events) == 0 {
		return `<section aria-label="Latest job activity"><div class="flex items-center justify-between gap-3">` +
			`<h4 class="text-xs font-semibold uppercase tracking-[.16em] text-zinc-400">Latest activity</h4>` +
			`<span class="text-[11px] text-zinc-600">No events yet</span></div>` +
			`<p class="mt-2 text-xs text-zinc-500">Waiting for authoritative progress events.</p></section>`, nil
	}
	var items strings.Builder
	var priorContextID int64
	for _, event := range events {
		if event.ContextID <= priorContextID || event.StepID <= 0 || event.Generation <= 0 ||
			event.OccurredAt.IsZero() || strings.TrimSpace(event.Summary) == "" {
			return "", fmt.Errorf("chat progress component received malformed ordered event authority")
		}
		priorContextID = event.ContextID
		tone, err := chatProgressKindClass(event.Kind)
		if err != nil {
			return "", err
		}
		items.WriteString(`<li data-progress-context-id="`)
		items.WriteString(strconv.FormatInt(event.ContextID, 10))
		items.WriteString(`" class="rounded border border-white/10 bg-white/[.03] p-2.5"><div class="flex items-start gap-2">`)
		items.WriteString(`<span aria-hidden="true" class="mt-1.5 h-1.5 w-1.5 shrink-0 rounded-full `)
		items.WriteString(tone)
		items.WriteString(`"></span><div class="min-w-0"><p class="text-xs leading-5 text-zinc-200">`)
		items.WriteString(html.EscapeString(event.Summary))
		items.WriteString(`</p><p class="mt-0.5 font-mono text-[10px] text-zinc-600">step `)
		items.WriteString(strconv.FormatInt(event.StepID, 10))
		items.WriteString(` · `)
		items.WriteString(html.EscapeString(event.OccurredAt.UTC().Format("15:04:05")))
		items.WriteString(`</p></div></div></li>`)
	}
	return `<section aria-label="Latest job activity"><div class="flex items-center justify-between gap-3">` +
		`<h4 class="text-xs font-semibold uppercase tracking-[.16em] text-zinc-400">Latest activity</h4>` +
		`<span class="text-[11px] text-zinc-600">Latest ` + strconv.Itoa(len(events)) +
		` authoritative events</span></div><ol class="mt-2 max-h-72 space-y-2 overflow-y-auto" aria-live="polite">` +
		items.String() + `</ol></section>`, nil
}

func chatProgressKindClass(kind chatProgressKind) (string, error) {
	classes := map[chatProgressKind]string{
		chatProgressActivity:     "bg-zinc-400",
		chatProgressStation:      "bg-violet-300",
		chatProgressRetrieval:    "bg-sky-300",
		chatProgressPreparation:  "bg-cyan-300",
		chatProgressFile:         "bg-emerald-300",
		chatProgressVerification: "bg-blue-300",
		chatProgressDiagnostic:   "bg-amber-300",
	}
	className, exists := classes[kind]
	if !exists {
		return "", fmt.Errorf("chat progress kind %q is not registered", kind)
	}
	return className, nil
}
