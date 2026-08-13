package api

import (
	"html"
	"strconv"
	"strings"

	"github.com/gryph/omnidex/internal/model"
)

const (
	chatTimelineTarget           = "timeline"
	chatTimelinePaginationTarget = "timeline-pagination"
	chatTimelineCountTarget      = "event-count"
)

func renderChatTimelinePage(jobs []model.Job, nextOffset *int, appendItems bool, loaded int) (chatComponentPage, error) {
	var output strings.Builder
	for _, job := range jobs {
		if err := validateChatJob(job); err != nil {
			return chatComponentPage{}, err
		}
		pill, err := chatStatusPillClass(job.Status)
		if err != nil {
			return chatComponentPage{}, err
		}
		output.WriteString(`<button type="button" data-action="chat#openTimelineJob" data-job-id="` +
			strconv.FormatInt(job.ID, 10) + `" class="timeline-card block w-full text-left transition hover:border-cyan-300/40 hover:bg-cyan-300/10">` +
			`<span class="flex items-start justify-between gap-3"><span><span class="text-sm font-semibold text-zinc-100">Job #` +
			strconv.FormatInt(job.ID, 10) + `</span><span class="mt-1 block line-clamp-2 text-xs text-zinc-400">` +
			html.EscapeString(job.Instruction) + `</span></span><span class="` + pill + `">` + html.EscapeString(job.Status) +
			`</span></span><time datetime="` + html.EscapeString(job.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z")) +
			`" class="mt-2 block font-mono text-[11px] text-zinc-500">` +
			html.EscapeString(job.UpdatedAt.UTC().Format("15:04 UTC")) + `</time></button>`)
	}
	if len(jobs) == 0 && !appendItems {
		output.WriteString(chatEmptyState("No server job activity yet."))
	}
	location := "innerHTML"
	if appendItems {
		location = "beforeend"
	}
	bundle := renderRecyclrTemplateHTML(chatTimelineTarget, output.String(), location) +
		renderRecyclrTemplateHTML(chatTimelinePaginationTarget, chatPaginationButton(
			"loadMoreTimeline", chatTimelineTarget, "timeline", nextOffset, "Load older activity",
		), "innerHTML") +
		renderRecyclrTemplateHTML(chatTimelineCountTarget, strconv.Itoa(loaded)+" jobs", "innerHTML")
	return chatComponentPage{
		NextOffset: nextOffset, HasMore: nextOffset != nil, HTML: chatComponentHTML{Bundle: bundle},
	}, nil
}
