package api

import (
	"fmt"
	"html"
	"strings"
)

func renderScrumFlowSummaryHTML(summary ScrumFlowProjectSummary) string {
	if summary.TotalCards == 0 {
		return ""
	}
	chips := []string{}
	if summary.LikelyIncomplete > 0 {
		chips = append(chips, fmt.Sprintf(`<span class="rounded border border-amber-300/30 bg-amber-300/10 px-2 py-1 text-[11px] text-amber-100">%d likely incomplete</span>`, summary.LikelyIncomplete))
	}
	if summary.LongConversations > 0 {
		chips = append(chips, fmt.Sprintf(`<span class="rounded border border-violet-300/25 bg-violet-300/10 px-2 py-1 text-[11px] text-violet-100">%d long conversations</span>`, summary.LongConversations))
	}
	if summary.AssignedReturnsTotal > 0 {
		chips = append(chips, fmt.Sprintf(`<span class="rounded border border-rose-300/25 bg-rose-300/10 px-2 py-1 text-[11px] text-rose-100">%d assigned returns</span>`, summary.AssignedReturnsTotal))
	}
	if summary.LikelyComplete > 0 {
		chips = append(chips, fmt.Sprintf(`<span class="rounded border border-emerald-300/25 bg-emerald-300/10 px-2 py-1 text-[11px] text-emerald-100">%d likely complete</span>`, summary.LikelyComplete))
	}
	if len(chips) == 0 {
		return ""
	}
	return `<div class="flex flex-wrap items-center gap-2">` + strings.Join(chips, "") + `</div>`
}

func renderScrumFlowBadgeHTML(card ScrumCard) (string, error) {
	metrics, err := parseScrumFlowMetrics(card.FlowMetrics)
	if err != nil {
		return "", err
	}
	if metrics.CompletionStatus == "" || metrics.CompletionStatus == "likely_complete" {
		return "", nil
	}
	tone := "border-zinc-400/30 bg-zinc-400/10 text-zinc-300"
	label := "Uncertain"
	if metrics.CompletionStatus == "likely_incomplete" {
		tone = "border-amber-300/35 bg-amber-300/10 text-amber-100"
		label = "Incomplete"
		if metrics.AssignedReturns > 0 {
			label = fmt.Sprintf("Incomplete - %d returns", metrics.AssignedReturns)
		}
	}
	title := strings.Join(firstScrumStrings(metrics.Signals, 3), " - ")
	return fmt.Sprintf(`<span class="rounded border px-1.5 py-0.5 text-[10px] font-medium normal-case tracking-normal %s" title="%s">%s</span>`, tone, html.EscapeString(title), html.EscapeString(label)), nil
}

func firstScrumStrings(values []string, limit int) []string {
	if limit <= 0 || len(values) <= limit {
		return values
	}
	return values[:limit]
}
