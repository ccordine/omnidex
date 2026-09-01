package api

import (
	"fmt"
	"html"
	"strconv"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
)

const (
	chatJobsTarget           = "jobs-list"
	chatJobsPaginationTarget = "jobs-pagination"
	chatProgressTarget       = "progress"
	chatProgressStateTarget  = "progress-state"
	chatJobDetailsTarget     = "job-details"
)

func renderChatJobsPage(jobs []model.Job, nextOffset *int, appendJobs bool) (chatComponentPage, error) {
	var output strings.Builder
	for _, job := range jobs {
		item, err := renderChatJobListItem(job)
		if err != nil {
			return chatComponentPage{}, err
		}
		output.WriteString(item)
	}
	if len(jobs) == 0 && !appendJobs {
		output.WriteString(chatEmptyState("No jobs matched this filter."))
	}
	location := "innerHTML"
	if appendJobs {
		location = "beforeend"
	}
	bundle := renderRecyclrTemplateHTML(chatJobsTarget, output.String(), location) +
		renderRecyclrTemplateHTML(chatJobsPaginationTarget, chatPaginationButton(
			"loadMoreJobs", chatJobsTarget, "jobs", nextOffset, "Load more jobs",
		), "innerHTML")
	return chatComponentPage{
		NextOffset: nextOffset, HasMore: nextOffset != nil, HTML: chatComponentHTML{Bundle: bundle},
	}, nil
}

func renderChatJobListItem(job model.Job) (string, error) {
	if err := validateChatJob(job); err != nil {
		return "", err
	}
	pill, err := chatStatusPillClass(job.Status)
	if err != nil {
		return "", err
	}
	return `<button type="button" data-action="chat#selectJob" data-job-id="` + strconv.FormatInt(job.ID, 10) +
		`" class="w-full rounded-lg border border-white/10 bg-zinc-950/50 p-3 text-left transition hover:border-cyan-300/40 hover:bg-cyan-300/10">` +
		`<span class="flex items-start justify-between gap-3"><span><span class="font-mono text-xs text-cyan-200">#` +
		strconv.FormatInt(job.ID, 10) + `</span><span class="mt-1 line-clamp-2 block text-sm font-medium text-zinc-100">` +
		html.EscapeString(job.Instruction) + `</span></span><span class="` + pill + `">` + html.EscapeString(job.Status) +
		`</span></span><span class="mt-2 block text-xs text-zinc-500">` + html.EscapeString(job.Pipeline) + ` · ` +
		html.EscapeString(job.UpdatedAt.UTC().Format("2006-01-02 15:04 UTC")) + `</span></button>`, nil
}

func renderChatJobStateBundle(presentation queue.JobPresentation) (string, error) {
	if err := validateChatJobPresentation(presentation); err != nil {
		return "", err
	}
	progress, err := renderChatProgress(presentation)
	if err != nil {
		return "", err
	}
	jobDetails, err := renderChatJobDetails(presentation)
	if err != nil {
		return "", err
	}
	return renderRecyclrTemplateHTML(chatProgressTarget, progress, "innerHTML") +
		renderRecyclrTemplateHTML(chatProgressStateTarget, html.EscapeString(presentation.Job.Status), "innerHTML") +
		renderRecyclrTemplateHTML(chatJobDetailsTarget, jobDetails, "innerHTML"), nil
}

func renderChatProgress(presentation queue.JobPresentation) (string, error) {
	pill, err := chatStatusPillClass(presentation.Job.Status)
	if err != nil {
		return "", err
	}
	current := "Waiting for deterministic work."
	for index := len(presentation.Steps) - 1; index >= 0; index-- {
		step := presentation.Steps[index]
		if step.Status == "running" || step.Status == "waiting_input" || step.Status == "pending" {
			current = step.Action + " · " + step.Status
			break
		}
	}
	return `<div class="space-y-3"><div class="flex items-center justify-between gap-3"><span class="font-mono text-xs text-cyan-200">#` +
		strconv.FormatInt(presentation.Job.ID, 10) + `</span><span class="` + pill + `">` + html.EscapeString(presentation.Job.Status) +
		`</span></div><div class="grid grid-cols-2 gap-2 text-center text-xs">` +
		chatMetricTile(strconv.Itoa(len(presentation.Steps)), "steps") +
		chatMetricTile(presentation.Job.UpdatedAt.UTC().Format("15:04"), "updated") + `</div>` +
		`<div class="rounded border border-white/10 bg-white/[.03] p-3"><div class="text-xs uppercase tracking-[.16em] text-zinc-500">Current step</div>` +
		`<div class="mt-1 text-sm text-zinc-200">` + html.EscapeString(current) + `</div></div>` +
		`<p data-chat-target="progressLoading" class="hidden text-sm text-cyan-100" role="status"></p></div>`, nil
}

func renderChatJobDetails(presentation queue.JobPresentation) (string, error) {
	details := presentation
	pill, err := chatStatusPillClass(details.Job.Status)
	if err != nil {
		return "", err
	}
	interrupt, err := chatJobControl("interruptJob", details.Job.ID, "Interrupt", "amber")
	if err != nil {
		return "", err
	}
	replan, err := chatJobControl("replanJob", details.Job.ID, "Replan", "cyan")
	if err != nil {
		return "", err
	}
	cancel, err := chatJobControl("cancelJob", details.Job.ID, "Cancel", "rose")
	if err != nil {
		return "", err
	}
	controls := `<div class="mt-4 flex flex-wrap gap-2">` + interrupt + replan + cancel + `</div>`
	body := `<div class="flex flex-wrap items-start justify-between gap-3"><div><div class="font-mono text-xs text-cyan-200">#` +
		strconv.FormatInt(details.Job.ID, 10) + `</div><h3 class="mt-1 text-lg font-semibold text-zinc-100">` +
		html.EscapeString(details.Job.Instruction) + `</h3><p class="mt-1 text-xs text-zinc-500">` +
		html.EscapeString(details.Job.Pipeline) + ` · ` + html.EscapeString(details.Job.CreatedAt.UTC().Format(time.RFC3339)) +
		`</p></div><span class="` + pill + `">` + html.EscapeString(details.Job.Status) + `</span></div>` + controls
	if details.Job.Result != "" {
		if err := validateChatText(details.Job.Result, "job result", 1024*1024); err != nil {
			return "", err
		}
		body += chatJobResultSection(details.Job.Result)
	}
	if details.Job.Error != "" {
		if err := validateChatText(details.Job.Error, "job error", 1024*1024); err != nil {
			return "", err
		}
		body += chatJobErrorSection(details.Job.Error)
	}
	body += `<dl class="mt-5 grid grid-cols-1 gap-3 text-sm"><div><dt class="text-zinc-500">Steps</dt><dd class="font-mono text-zinc-200">` +
		strconv.Itoa(len(details.Steps)) + `</dd></div></dl>`
	return body, nil
}

func validateChatJob(job model.Job) error {
	if job.ID < 1 || job.CreatedAt.IsZero() || job.UpdatedAt.IsZero() {
		return fmt.Errorf("chat job presentation requires positive identity and timestamps")
	}
	if err := validateChatText(job.Instruction, "job instruction", 64*1024); err != nil {
		return err
	}
	if err := validateChatText(job.Pipeline, "job pipeline", 64); err != nil {
		return err
	}
	_, err := chatStatusPillClass(job.Status)
	return err
}

func chatMetricTile(value, label string) string {
	return `<div class="rounded border border-white/10 bg-white/[.03] p-2"><div class="font-mono text-zinc-100">` +
		html.EscapeString(value) + `</div><div class="mt-1 text-zinc-500">` + html.EscapeString(label) + `</div></div>`
}

func chatJobControl(action string, jobID int64, label, color string) (string, error) {
	classes := map[string]string{
		"amber": "border-amber-300/30 bg-amber-300/10 text-amber-100",
		"cyan":  "border-cyan-300/30 bg-cyan-300/10 text-cyan-100",
		"rose":  "border-rose-300/30 bg-rose-300/10 text-rose-100",
	}
	className, exists := classes[color]
	if !exists {
		return "", fmt.Errorf("unsupported chat job control color %q", color)
	}
	return `<button type="button" data-action="chat#` + action + `" data-job-id="` + strconv.FormatInt(jobID, 10) +
		`" class="rounded-md border px-3 py-2 text-xs font-semibold ` + className + `">` + label + `</button>`, nil
}

func chatJobResultSection(value string) string {
	return `<section class="mt-5"><h4 class="text-xs font-semibold uppercase tracking-[.18em] text-zinc-300">Result</h4>` +
		`<pre class="mt-2 max-h-64 overflow-auto whitespace-pre-wrap rounded-md bg-zinc-400/10 p-3 text-sm text-zinc-100">` +
		html.EscapeString(value) + `</pre></section>`
}

func chatJobErrorSection(value string) string {
	return `<section class="mt-5"><h4 class="text-xs font-semibold uppercase tracking-[.18em] text-rose-300">Error</h4>` +
		`<pre class="mt-2 max-h-64 overflow-auto whitespace-pre-wrap rounded-md bg-rose-400/10 p-3 text-sm text-rose-100">` +
		html.EscapeString(value) + `</pre></section>`
}
