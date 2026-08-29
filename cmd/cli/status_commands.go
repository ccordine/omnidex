package main

import (
	"context"
	"flag"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/client"
)

const defaultStatusTimeout = 5 * time.Second
const defaultQueueStatusSampleLimit = 300

type coreStatusReport struct {
	CoreURL string
	Status  string
	Time    string
	Error   string
}

type queueStatusReport struct {
	SampleLimit int
	Sampled     int
	Counts      map[string]int
	ActiveIDs   []int64
	Error       string
}

type ollamaStatusReport struct {
	Skipped    bool
	SkipReason string
	BaseURL    string
	ModelCount int
	Models     []string
	Error      string
}

type webProbeReport struct {
	Provider   string
	TargetURL  string
	StatusCode int
	Error      string
}

type webStatusReport struct {
	Enabled   bool
	Providers []string
	Probe     bool
	Probes    []webProbeReport
}

func runStatus(apiClient *client.Client, args []string, configuredCoreURL string) {
	fs := flag.NewFlagSet("status", flag.ExitOnError)
	timeout := fs.Duration("timeout", defaultStatusTimeout, "per-service status timeout")
	coreURLFlag := fs.String("core-url", "", "core URL override")
	queueLimit := fs.Int("queue-limit", defaultQueueStatusSampleLimit, "queue sample size")
	webProbe := fs.Bool("web-probe", true, "probe provider reachability for web status")
	providersFlag := fs.String("providers", "", "override web providers csv")
	_ = fs.Parse(args)

	coreURL := resolveCoreStatusURL(*coreURLFlag, configuredCoreURL)
	if *queueLimit < 1 {
		*queueLimit = defaultQueueStatusSampleLimit
	}
	statusClient := client.New(coreURL, *timeout)

	coreReport := collectCoreStatus(coreURL, *timeout)
	queueReport := collectQueueStatus(statusClient, *queueLimit, *timeout)
	llmProvider, providerErr := statusLLMProvider()
	ollamaReport := collectOllamaStatus(defaultOllamaBaseURL(), *timeout, providerErr == nil && llmProvider == "ollama")
	webReport := collectWebStatus(parseStatusProviders(*providersFlag), *webProbe, *timeout)

	printCoreStatusLine(coreReport)
	printQueueStatusLine(queueReport)
	printLLMStatusLine(llmProvider, providerErr)
	printOllamaStatusLine(ollamaReport)
	printWebStatusLine(webReport, true)

	failures := 0
	if providerErr != nil {
		failures++
	}
	if strings.TrimSpace(coreReport.Error) != "" {
		failures++
	}
	if strings.TrimSpace(queueReport.Error) != "" {
		failures++
	}
	if !ollamaReport.Skipped && strings.TrimSpace(ollamaReport.Error) != "" {
		failures++
	}
	if webStatusHasFailures(webReport) {
		failures++
	}
	if failures > 0 {
		os.Exit(1)
	}
}

func runCoreStatus(args []string, configuredCoreURL string) {
	fs := flag.NewFlagSet("core:status", flag.ExitOnError)
	timeout := fs.Duration("timeout", defaultStatusTimeout, "status timeout")
	coreURLFlag := fs.String("core-url", "", "core URL override")
	_ = fs.Parse(args)

	report := collectCoreStatus(resolveCoreStatusURL(*coreURLFlag, configuredCoreURL), *timeout)
	printCoreStatusLine(report)
	if strings.TrimSpace(report.Error) != "" {
		os.Exit(1)
	}
}

func runQueueStatus(_ *client.Client, args []string, configuredCoreURL string) {
	fs := flag.NewFlagSet("queue:status", flag.ExitOnError)
	timeout := fs.Duration("timeout", defaultStatusTimeout, "status timeout")
	limit := fs.Int("limit", defaultQueueStatusSampleLimit, "queue sample size")
	coreURLFlag := fs.String("core-url", "", "core URL override")
	_ = fs.Parse(args)

	if *limit < 1 {
		*limit = defaultQueueStatusSampleLimit
	}
	coreURL := resolveCoreStatusURL(*coreURLFlag, configuredCoreURL)
	statusClient := client.New(coreURL, *timeout)

	report := collectQueueStatus(statusClient, *limit, *timeout)
	printQueueStatusLine(report)
	if strings.TrimSpace(report.Error) != "" {
		os.Exit(1)
	}
}

func runOllamaStatus(args []string) {
	fs := flag.NewFlagSet("ollama:status", flag.ExitOnError)
	timeout := fs.Duration("timeout", defaultStatusTimeout, "status timeout")
	baseURL := fs.String("base-url", defaultOllamaBaseURL(), "ollama base URL")
	_ = fs.Parse(args)

	report := collectOllamaStatus(strings.TrimSpace(*baseURL), *timeout, true)
	printOllamaStatusLine(report)
	if strings.TrimSpace(report.Error) != "" {
		os.Exit(1)
	}
}

func runWebStatus(args []string) {
	fs := flag.NewFlagSet("web:status", flag.ExitOnError)
	timeout := fs.Duration("timeout", defaultStatusTimeout, "per-provider probe timeout")
	probe := fs.Bool("probe", true, "probe provider reachability")
	providersFlag := fs.String("providers", "", "override web providers csv")
	_ = fs.Parse(args)

	report := collectWebStatus(parseStatusProviders(*providersFlag), *probe, *timeout)
	printWebStatusLine(report, false)
	if webStatusHasFailures(report) {
		os.Exit(1)
	}
}

func collectCoreStatus(coreURL string, timeout time.Duration) coreStatusReport {
	report := coreStatusReport{
		CoreURL: coreURL,
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	status, ts, err := queryCoreHealth(ctx, coreURL)
	if err != nil {
		report.Error = err.Error()
		return report
	}
	report.Status = status
	report.Time = ts
	return report
}

func collectQueueStatus(c *client.Client, limit int, timeout time.Duration) queueStatusReport {
	report := queueStatusReport{
		SampleLimit: limit,
		Counts:      make(map[string]int, 8),
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	jobs, err := c.List(ctx, "", limit, 0)
	if err != nil {
		report.Error = err.Error()
		return report
	}
	report.Sampled = len(jobs)
	for _, job := range jobs {
		status := strings.TrimSpace(job.Status)
		if status == "" {
			status = "unknown"
		}
		report.Counts[status]++
		if isActiveJobStatus(status) && len(report.ActiveIDs) < 5 {
			report.ActiveIDs = append(report.ActiveIDs, job.ID)
		}
	}
	return report
}

func collectOllamaStatus(baseURL string, timeout time.Duration, enabled bool) ollamaStatusReport {
	report := ollamaStatusReport{
		BaseURL: normalizeStatusURL(baseURL, defaultOllamaBaseURL()),
	}
	if !enabled {
		report.Skipped = true
		report.SkipReason = "LLM_PROVIDER is not ollama"
		return report
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	models, err := queryOllamaModels(ctx, report.BaseURL)
	if err != nil {
		report.Error = err.Error()
		return report
	}
	report.Models = models
	report.ModelCount = len(models)
	return report
}

func collectWebStatus(providers []string, probe bool, timeout time.Duration) webStatusReport {
	report := webStatusReport{
		Enabled:   true,
		Providers: providers,
		Probe:     probe,
	}

	if !probe {
		return report
	}

	probes := make([]webProbeReport, 0, len(providers))
	for _, provider := range providers {
		target := statusProviderProbeURL(provider)
		item := webProbeReport{
			Provider:  provider,
			TargetURL: target,
		}
		if target == "" {
			item.Error = "no probe URL mapping"
			probes = append(probes, item)
			continue
		}

		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		statusCode, err := probeHTTPReachability(ctx, target)
		cancel()
		if err != nil {
			item.Error = err.Error()
		} else {
			item.StatusCode = statusCode
		}
		probes = append(probes, item)
	}

	report.Probes = probes
	return report
}

func resolveCoreStatusURL(raw string, configuredCoreURL string) string {
	return normalizeStatusURL(raw, configuredCoreURL)
}

func normalizeStatusURL(raw string, fallback string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		value = strings.TrimSpace(fallback)
	}
	if value == "" {
		return ""
	}
	if !strings.Contains(value, "://") {
		value = "http://" + value
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return strings.TrimRight(value, "/")
	}
	if parsed.Scheme == "" {
		parsed.Scheme = "http"
	}
	return strings.TrimRight(parsed.String(), "/")
}
