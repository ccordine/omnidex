package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/gryph/omnidex/internal/llmprovider/catalog"
	"github.com/gryph/omnidex/internal/model"
)

func printCoreStatusLine(report coreStatusReport) {
	if strings.TrimSpace(report.Error) != "" {
		fmt.Printf("core: down url=%s error=%s\n", report.CoreURL, report.Error)
		return
	}
	line := fmt.Sprintf("core: ok url=%s status=%s", report.CoreURL, safeValue(report.Status, "unknown"))
	if strings.TrimSpace(report.Time) != "" {
		line += " time=" + report.Time
	}
	fmt.Println(line)
}

func printQueueStatusLine(report queueStatusReport) {
	if strings.TrimSpace(report.Error) != "" {
		fmt.Printf("queue: down error=%s\n", report.Error)
		return
	}

	parts := []string{
		fmt.Sprintf("queue: ok sampled=%d limit=%d", report.Sampled, report.SampleLimit),
		fmt.Sprintf("pending=%d", report.Counts[model.JobStatusPending]),
		fmt.Sprintf("running=%d", report.Counts[model.JobStatusRunning]),
		fmt.Sprintf("waiting_input=%d", report.Counts[model.JobStatusWaiting]),
		fmt.Sprintf("completed=%d", report.Counts[model.JobStatusCompleted]),
		fmt.Sprintf("failed=%d", report.Counts[model.JobStatusFailed]),
		fmt.Sprintf("canceled=%d", report.Counts[model.JobStatusCanceled]),
	}
	if len(report.ActiveIDs) > 0 {
		parts = append(parts, "active_job_ids="+joinInt64s(report.ActiveIDs))
	}
	fmt.Println(strings.Join(parts, " "))
}

func printOllamaStatusLine(report ollamaStatusReport) {
	if report.Skipped {
		fmt.Printf("ollama: skipped reason=%s\n", safeValue(report.SkipReason, "disabled"))
		return
	}
	if strings.TrimSpace(report.Error) != "" {
		fmt.Printf("ollama: down base_url=%s error=%s\n", report.BaseURL, report.Error)
		return
	}

	parts := []string{
		fmt.Sprintf("ollama: ok base_url=%s models=%d", report.BaseURL, report.ModelCount),
	}
	if len(report.Models) > 0 {
		parts = append(parts, "sample_models="+compactStatusList(report.Models, 4))
	}
	fmt.Println(strings.Join(parts, " "))
}

func statusLLMProvider() (string, error) {
	value := strings.ToLower(strings.TrimSpace(os.Getenv("LLM_PROVIDER")))
	if value == "" {
		return "", nil
	}
	return catalog.CanonicalID(value)
}

func printLLMStatusLine(provider string, err error) {
	if err != nil {
		fmt.Printf("llm: invalid error=%s\n", err)
	} else if provider == "" {
		fmt.Println("llm: unconfigured")
	} else {
		fmt.Printf("llm: configured provider=%s\n", provider)
	}
}

func printWebStatusLine(report webStatusReport, summaryOnly bool) {
	reachable := 0
	failed := 0
	for _, probe := range report.Probes {
		if strings.TrimSpace(probe.Error) != "" {
			failed++
		} else if probe.StatusCode > 0 {
			reachable++
		}
	}

	fmt.Printf(
		"web: enabled providers=%d probe=%t reachable=%d failed=%d\n",
		len(report.Providers),
		report.Probe,
		reachable,
		failed,
	)

	if summaryOnly {
		return
	}

	for _, probe := range report.Probes {
		if strings.TrimSpace(probe.Error) != "" {
			fmt.Printf("  - %s down target=%s error=%s\n", probe.Provider, safeValue(probe.TargetURL, "n/a"), probe.Error)
			continue
		}
		fmt.Printf("  - %s ok target=%s status=%d\n", probe.Provider, safeValue(probe.TargetURL, "n/a"), probe.StatusCode)
	}
}

func webStatusHasFailures(report webStatusReport) bool {
	if !report.Enabled || !report.Probe {
		return false
	}
	for _, probe := range report.Probes {
		if strings.TrimSpace(probe.Error) != "" {
			return true
		}
	}
	return false
}

func isActiveJobStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case model.JobStatusPending, model.JobStatusRunning, model.JobStatusWaiting:
		return true
	default:
		return false
	}
}

func compactStatusList(values []string, limit int) string {
	if len(values) == 0 {
		return ""
	}
	if limit < 1 || len(values) <= limit {
		return strings.Join(values, "|")
	}
	return strings.Join(values[:limit], "|") + "|..."
}

func trimStatusBody(body []byte) string {
	text := strings.TrimSpace(string(body))
	if len(text) <= 200 {
		return text
	}
	return text[:200] + "...[truncated]"
}

func joinInt64s(values []int64) string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, fmt.Sprintf("%d", value))
	}
	return strings.Join(parts, ",")
}
