package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/client"
	"github.com/gryph/omnidex/internal/model"
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

func runStatus(apiClient *client.Client, args []string) {
	fs := flag.NewFlagSet("status", flag.ExitOnError)
	timeout := fs.Duration("timeout", defaultStatusTimeout, "per-service status timeout")
	coreURLFlag := fs.String("core-url", "", "core URL override")
	queueLimit := fs.Int("queue-limit", defaultQueueStatusSampleLimit, "queue sample size")
	webProbe := fs.Bool("web-probe", true, "probe provider reachability for web status")
	providersFlag := fs.String("providers", "", "override web providers csv")
	_ = fs.Parse(args)

	coreURL := resolveCoreStatusURL(*coreURLFlag)
	if *queueLimit < 1 {
		*queueLimit = defaultQueueStatusSampleLimit
	}
	statusClient := client.New(coreURL, *timeout)

	coreReport := collectCoreStatus(coreURL, *timeout)
	queueReport := collectQueueStatus(statusClient, *queueLimit, *timeout)
	llmProvider := statusLLMProvider()
	ollamaReport := collectOllamaStatus(defaultOllamaBaseURL(), *timeout, llmProvider == "ollama")
	webReport := collectWebStatus(parseStatusProviders(*providersFlag), *webProbe, *timeout)

	printCoreStatusLine(coreReport)
	printQueueStatusLine(queueReport)
	printOllamaStatusLine(ollamaReport)
	printWebStatusLine(webReport, true)

	failures := 0
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

func runCoreStatus(args []string) {
	fs := flag.NewFlagSet("core:status", flag.ExitOnError)
	timeout := fs.Duration("timeout", defaultStatusTimeout, "status timeout")
	coreURLFlag := fs.String("core-url", "", "core URL override")
	_ = fs.Parse(args)

	report := collectCoreStatus(resolveCoreStatusURL(*coreURLFlag), *timeout)
	printCoreStatusLine(report)
	if strings.TrimSpace(report.Error) != "" {
		os.Exit(1)
	}
}

func runQueueStatus(_ *client.Client, args []string) {
	fs := flag.NewFlagSet("queue:status", flag.ExitOnError)
	timeout := fs.Duration("timeout", defaultStatusTimeout, "status timeout")
	limit := fs.Int("limit", defaultQueueStatusSampleLimit, "queue sample size")
	coreURLFlag := fs.String("core-url", "", "core URL override")
	_ = fs.Parse(args)

	if *limit < 1 {
		*limit = defaultQueueStatusSampleLimit
	}
	coreURL := resolveCoreStatusURL(*coreURLFlag)
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
		Enabled:   statusEnvBool("WEB_SEARCH_ENABLED", true),
		Providers: providers,
		Probe:     probe,
	}

	if !report.Enabled || !probe {
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

func resolveCoreStatusURL(raw string) string {
	return normalizeStatusURL(raw, getenv("CORE_URL", "http://localhost:8090"))
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

func queryCoreHealth(ctx context.Context, coreURL string) (string, string, error) {
	endpoint := strings.TrimRight(coreURL, "/") + "/healthz"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", "", fmt.Errorf("status=%d body=%s", resp.StatusCode, trimStatusBody(body))
	}

	var payload struct {
		Status string    `json:"status"`
		Time   time.Time `json:"time"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", "", err
	}

	status := strings.TrimSpace(payload.Status)
	if status == "" {
		status = "unknown"
	}
	ts := ""
	if !payload.Time.IsZero() {
		ts = payload.Time.UTC().Format(time.RFC3339)
	}
	return status, ts, nil
}

func queryOllamaModels(ctx context.Context, baseURL string) ([]string, error) {
	endpoint := strings.TrimRight(baseURL, "/") + "/api/tags"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("status=%d body=%s", resp.StatusCode, trimStatusBody(body))
	}

	var payload struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	names := make([]string, 0, len(payload.Models))
	for _, modelValue := range payload.Models {
		name := strings.TrimSpace(modelValue.Name)
		if name == "" {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}

func probeHTTPReachability(ctx context.Context, endpoint string) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("User-Agent", "omni-status/1.0")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	_, _ = io.CopyN(io.Discard, resp.Body, 256)
	if resp.StatusCode >= 500 {
		return resp.StatusCode, fmt.Errorf("status=%d", resp.StatusCode)
	}
	return resp.StatusCode, nil
}

func parseStatusProviders(override string) []string {
	if strings.TrimSpace(override) != "" {
		values := parseStatusCSV(override)
		if len(values) > 0 {
			return values
		}
	}
	if values := parseStatusCSV(os.Getenv("WEB_SEARCH_PROVIDERS")); len(values) > 0 {
		return values
	}
	return []string{"yahoo", "google", "reddit"}
}

func parseStatusCSV(value string) []string {
	parts := strings.Split(value, ",")
	seen := map[string]struct{}{}
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		item := strings.ToLower(strings.TrimSpace(part))
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}

func statusProviderProbeURL(provider string) string {
	p := strings.ToLower(strings.TrimSpace(provider))
	if p == "" {
		return ""
	}
	if strings.HasPrefix(p, "http://") || strings.HasPrefix(p, "https://") {
		return strings.TrimRight(p, "/")
	}

	switch p {
	case "google":
		return "https://www.google.com"
	case "yahoo":
		return "https://search.yahoo.com"
	case "reddit":
		return "https://www.reddit.com"
	case "duckduckgo":
		return "https://duckduckgo.com"
	case "bing":
		return "https://www.bing.com"
	}

	if strings.Contains(p, ".") {
		return "https://" + strings.TrimRight(p, "/")
	}
	return ""
}

func statusEnvBool(key string, fallback bool) bool {
	value := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	if value == "" {
		return fallback
	}
	switch value {
	case "1", "true", "yes", "y", "on", "enabled":
		return true
	case "0", "false", "no", "n", "off", "disabled":
		return false
	default:
		return fallback
	}
}

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

func statusLLMProvider() string {
	value := strings.ToLower(strings.TrimSpace(os.Getenv("LLM_PROVIDER")))
	if value == "" {
		return "ollama"
	}
	return value
}

func printWebStatusLine(report webStatusReport, summaryOnly bool) {
	if !report.Enabled {
		fmt.Println("web: disabled WEB_SEARCH_ENABLED=false")
		return
	}

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
