package api

import (
	"fmt"
	"html"
	"strconv"
	"strings"

	"github.com/gryph/omnidex/internal/queue"
)

const chatMetricsTarget = "metrics-output"

type chatMetricsSnapshot struct {
	Live       queue.TelemetryDashboardSummary
	Models     []queue.TelemetryModelSummary
	Shrink     queue.ContextShrinkMetricsResponse
	Usage      queue.LLMContextUsageMetricsResponse
	Operations queue.OperationsMetricsResponse
}

func renderChatMetricsBundle(snapshot chatMetricsSnapshot) (string, error) {
	markup, err := renderChatMetrics(snapshot)
	if err != nil {
		return "", err
	}
	return renderRecyclrTemplateHTML(chatMetricsTarget, markup, "innerHTML"), nil
}

func renderChatMetrics(snapshot chatMetricsSnapshot) (string, error) {
	completed, err := exactChatMetricCount(snapshot.Live.StatusCounts, "completed")
	if err != nil {
		return "", err
	}
	failed, err := exactChatMetricCount(snapshot.Live.StatusCounts, "failed")
	if err != nil {
		return "", err
	}
	modelCalls, modelFailures := 0, 0
	for _, item := range snapshot.Models {
		if item.Calls < 0 || item.Failures < 0 {
			return "", fmt.Errorf("model metrics contain negative counts")
		}
		modelCalls += item.Calls
		modelFailures += item.Failures
	}
	failureEvents := 0
	for _, item := range snapshot.Operations.FailureCounts {
		if item.Count < 0 {
			return "", fmt.Errorf("operations metrics contain a negative failure count")
		}
		failureEvents += item.Count
	}
	metrics := []chatMetric{
		{Label: "Live runs", Value: strconv.Itoa(len(snapshot.Live.LiveRuns)), Tone: chatMetricNeutral},
		{Label: "Completed", Value: strconv.Itoa(completed), Tone: chatMetricHealthy},
		{Label: "Failed", Value: strconv.Itoa(failed), Tone: chatMetricDanger},
		{Label: "Model calls", Value: strconv.Itoa(modelCalls), Tone: chatMetricNeutral},
		{Label: "Model failures", Value: strconv.Itoa(modelFailures), Tone: chatMetricDanger},
		{Label: "Context requests", Value: strconv.Itoa(snapshot.Usage.Summary.Requests), Tone: chatMetricNeutral},
		{Label: "Context utilization", Value: fmt.Sprintf("%.1f%%", snapshot.Usage.Summary.AvgUtilization), Tone: chatMetricNeutral},
		{Label: "Context overloads", Value: strconv.Itoa(snapshot.Usage.Summary.OverloadEvents), Tone: chatMetricDanger},
		{Label: "Context saved", Value: fmt.Sprintf("%.1f%%", snapshot.Shrink.Summary.AvgSavedPct), Tone: chatMetricHealthy},
		{Label: "Failure events", Value: strconv.Itoa(failureEvents), Tone: chatMetricDanger},
	}
	var output strings.Builder
	output.WriteString(`<section class="space-y-4" role="status" aria-live="polite"><div><h3 class="text-sm font-semibold uppercase tracking-[.18em] text-zinc-300">Operational metrics</h3><p class="mt-1 text-xs text-zinc-500">Server-computed summaries from durable telemetry.</p></div><dl class="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">`)
	for _, metric := range metrics {
		output.WriteString(renderChatMetric(metric))
	}
	output.WriteString(`</dl></section>`)
	return output.String(), nil
}

type chatMetricTone uint8

const (
	chatMetricNeutral chatMetricTone = iota
	chatMetricHealthy
	chatMetricDanger
)

type chatMetric struct {
	Label string
	Value string
	Tone  chatMetricTone
}

func renderChatMetric(metric chatMetric) string {
	className := "text-cyan-200"
	switch metric.Tone {
	case chatMetricHealthy:
		className = "text-emerald-200"
	case chatMetricDanger:
		className = "text-rose-200"
	}
	return `<div class="rounded border border-white/10 bg-white/[.03] p-3"><dt class="text-[11px] uppercase tracking-[.16em] text-zinc-500">` +
		html.EscapeString(metric.Label) + `</dt><dd class="mt-1 font-mono text-lg ` + className + `">` +
		html.EscapeString(metric.Value) + `</dd></div>`
}

func exactChatMetricCount(counts map[string]int, key string) (int, error) {
	value := counts[key]
	if value < 0 {
		return 0, fmt.Errorf("job status metric %q is negative", key)
	}
	return value, nil
}
