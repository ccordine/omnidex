package api

import (
	"fmt"
	"html"
	"strings"
)

const chatHostBridgeStatusTarget = "host-bridge-status-output"

func renderChatHostBridgeStatusBundle(status hostBridgeStatusResponse) (string, error) {
	markup, err := renderChatHostBridgeStatus(status)
	if err != nil {
		return "", err
	}
	return renderRecyclrTemplateHTML(chatHostBridgeStatusTarget, markup, "innerHTML"), nil
}

func renderChatHostBridgeStatus(status hostBridgeStatusResponse) (string, error) {
	for name, value := range map[string]string{
		"host bridge URL": status.URL, "host bridge service": status.Service,
		"host bridge error": status.Error, "host bridge message": status.Message,
	} {
		if value != "" {
			if err := validateChatText(value, name, 64*1024); err != nil {
				return "", err
			}
		}
	}
	if len(status.Suggestions) > 16 {
		return "", fmt.Errorf("host bridge suggestions exceed the 16-item presentation bound")
	}
	var suggestions strings.Builder
	if len(status.Suggestions) > 0 {
		suggestions.WriteString(`<div class="rounded border border-amber-300/30 bg-amber-300/10 p-3"><div class="text-xs font-semibold uppercase tracking-[.16em] text-amber-200">How to fix</div><ol class="mt-2 list-decimal space-y-2 pl-5 text-sm leading-6 text-amber-50">`)
		for _, suggestion := range status.Suggestions {
			if err := validateChatText(suggestion, "host bridge suggestion", 4096); err != nil {
				return "", err
			}
			suggestions.WriteString(`<li>` + html.EscapeString(suggestion) + `</li>`)
		}
		suggestions.WriteString(`</ol></div>`)
	}
	messageClass := "text-zinc-300"
	if status.Reachable {
		messageClass = "text-emerald-100"
	}
	errorRow := ""
	if status.Error != "" {
		errorRow = `<div class="text-rose-200"><dt class="inline text-rose-300">error</dt> <dd class="inline">` +
			html.EscapeString(status.Error) + `</dd></div>`
	}
	markup := `<div class="space-y-3" role="status" aria-live="polite"><div class="grid grid-cols-2 gap-2 text-xs">` +
		chatHostMetric("Bridge", chatBooleanState(status.Reachable, "reachable", "down"), status.Reachable) +
		chatHostMetric("Configured", chatBooleanState(status.Configured, "yes", "no"), status.Configured) +
		chatHostMetric("Picker", chatBooleanState(status.PickerReady, "ready", "unavailable"), status.PickerReady) +
		chatHostMetric("Native UI", chatBooleanState(status.NativePicker, "yes", "n/a"), status.NativePicker) +
		`</div><div class="rounded border border-white/10 bg-white/[.03] p-3"><div class="text-xs uppercase tracking-[.16em] text-zinc-500">Host bridge</div>` +
		`<dl class="mt-2 space-y-1 font-mono text-xs text-zinc-300"><div><dt class="inline text-zinc-500">url</dt> <dd class="inline">` +
		html.EscapeString(chatDisplayedValue(status.URL, "not set")) + `</dd></div><div><dt class="inline text-zinc-500">service</dt> <dd class="inline">` +
		html.EscapeString(chatDisplayedValue(status.Service, "n/a")) + `</dd></div>` + errorRow + `</dl>` +
		`<p class="mt-3 text-sm leading-6 ` + messageClass + `">` + html.EscapeString(status.Message) + `</p></div>` +
		suggestions.String() + `</div>`
	return markup, nil
}

func chatHostMetric(label, value string, healthy bool) string {
	className := "text-amber-200"
	if healthy {
		className = "text-emerald-200"
	}
	return `<div class="rounded border border-white/10 bg-white/[.03] p-3"><div class="text-[11px] uppercase tracking-[.16em] text-zinc-500">` +
		html.EscapeString(label) + `</div><div class="mt-1 truncate font-mono text-xs ` + className + `">` +
		html.EscapeString(value) + `</div></div>`
}

func chatBooleanState(value bool, whenTrue, whenFalse string) string {
	if value {
		return whenTrue
	}
	return whenFalse
}

func chatDisplayedValue(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
