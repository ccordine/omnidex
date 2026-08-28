package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http/httptest"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/browserinference"
	"github.com/gryph/omnidex/internal/station"
)

const browserContextQualificationReportSchemaV1 = "omnidex.context-relevance-qualification-report.v1"

// TestLiveBrowserContextRelevanceQualification is an opt-in framework-primitive
// qualification. It drives the real embedded UI, WebGPU worker, WebSocket
// provider bridge, production station renderer, and server validator. It is not
// an autonomy benchmark and it does not mutate production routing.
func TestLiveBrowserContextRelevanceQualification(t *testing.T) {
	if os.Getenv("OMNIDEX_TEST_BROWSER_CONTEXT_QUALIFICATION") != "1" {
		t.Skip("set OMNIDEX_TEST_BROWSER_CONTEXT_QUALIFICATION=1 to run live WebGPU qualification")
	}
	model := requireQualificationEnvironment(t, "OMNIDEX_TEST_BROWSER_CONTEXT_MODEL")
	reportPath := requireQualificationEnvironment(t, "OMNIDEX_TEST_BROWSER_CONTEXT_REPORT")
	chromiumPath := requireQualificationEnvironment(t, "OMNIDEX_TEST_CHROMIUM_PATH")
	profileDirectory := requireQualificationEnvironment(t, "OMNIDEX_TEST_BROWSER_CONTEXT_PROFILE")
	medianTarget := requirePositiveQualificationInt64(
		t, "OMNIDEX_TEST_BROWSER_CONTEXT_MAX_MEDIAN_MS",
	)
	if err := os.MkdirAll(profileDirectory, 0o700); err != nil {
		t.Fatalf("create qualification browser profile: %v", err)
	}
	corpus := loadBrowserContextQualificationCorpus(t)
	browserVersion := qualificationBrowserVersion(t, chromiumPath)

	broker := browserinference.NewContextRelevanceBroker()
	server := NewServerWithOptions(nil, nil, ServerOptions{
		BrowserContextRelevance: broker,
		BrowserContextModel:     model,
	})
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()

	runContext, cancelRun := context.WithTimeout(t.Context(), 30*time.Minute)
	defer cancelRun()
	browserContext, stopBrowser := context.WithCancel(runContext)
	var browserErrors bytes.Buffer
	command := exec.CommandContext(browserContext, chromiumPath,
		"--headless=new",
		"--enable-unsafe-webgpu",
		"--enable-features=Vulkan,UseSkiaRenderer",
		"--use-angle=vulkan",
		"--disable-vulkan-surface",
		"--enable-logging=stderr",
		"--disable-background-timer-throttling",
		"--disable-dev-shm-usage",
		"--no-default-browser-check",
		"--no-first-run",
		"--user-data-dir="+profileDirectory,
		httpServer.URL+"/",
	)
	command.Stdout = io.Discard
	command.Stderr = io.MultiWriter(&browserErrors, os.Stderr)
	if err := command.Start(); err != nil {
		t.Fatalf("start qualification browser: %v", err)
	}
	browserDone := make(chan struct{})
	var browserExitErr error
	go func() {
		browserExitErr = command.Wait()
		close(browserDone)
	}()
	if err := waitForQualificationBrowser(runContext, broker, browserDone, &browserExitErr); err != nil {
		stopBrowser()
		<-browserDone
		t.Fatalf("%v\nchromium stderr:\n%s", err, browserErrors.String())
	}

	report := runBrowserContextQualification(t, runContext, broker, model, browserVersion, medianTarget, corpus)
	stopBrowser()
	<-browserDone
	writeBrowserContextQualificationReport(t, reportPath, report)
	if !report.Qualified {
		t.Fatalf(
			"browser context relevance did not qualify: passed=%t cases=%d/%d median_ms=%d target_ms=%d report=%s",
			report.Passed, report.MeasuredQuality.PassedCases, report.MeasuredQuality.CaseCount,
			report.MeasuredLatencyMS.Median, report.MedianLatencyTargetMS, reportPath,
		)
	}
}

func runBrowserContextQualification(
	t *testing.T,
	ctx context.Context,
	broker *browserinference.ContextRelevanceBroker,
	model string,
	browserVersion string,
	medianTarget int64,
	corpus browserContextQualificationCorpus,
) browserContextQualificationReport {
	t.Helper()
	report := browserContextQualificationReport{
		Schema: browserContextQualificationReportSchemaV1, CreatedAt: time.Now().UTC(),
		Station: station.ContextRelevance, Provider: "browser_webgpu", Model: model,
		Browser: browserVersion, CorpusVersion: corpus.Version,
		CorpusSHA256:          qualificationCorpusSHA256(),
		MedianLatencyTargetMS: medianTarget,
		Cases:                 make([]browserContextQualificationCaseResult, 0, len(corpus.Cases)),
	}
	latencies := make([]int64, 0, len(corpus.Cases))
	truePositive, falsePositive, falseNegative := 0, 0, 0
	for index, testCase := range corpus.Cases {
		input := buildBrowserContextQualificationInput(t, testCase)
		started := time.Now()
		caseContext, cancel := context.WithTimeout(ctx, 2*time.Minute)
		selected, err := executeBrowserContextRelevanceSelection(
			caseContext, broker, model, input,
		)
		cancel()
		latency := time.Since(started).Milliseconds()
		result := browserContextQualificationCaseResult{
			Name: testCase.Name, LatencyMS: latency,
			ExpectedCandidateIDs: append([]string{}, testCase.ExpectedCandidateIDs...),
			ActualCandidateIDs:   append([]string{}, selected...),
		}
		if err != nil {
			result.Error = err.Error()
			report.Cases = append(report.Cases, result)
			_, _, missed := selectionCounts(testCase.ExpectedCandidateIDs, nil)
			falseNegative += missed
			t.Logf("qualification case %s failed: %v", qualificationCaseLabel(index, testCase), err)
			break
		}
		latencies = append(latencies, latency)
		result.Passed = sameOpaqueIDSet(testCase.ExpectedCandidateIDs, selected)
		tp, fp, fn := selectionCounts(testCase.ExpectedCandidateIDs, selected)
		truePositive += tp
		falsePositive += fp
		falseNegative += fn
		if result.Passed {
			report.MeasuredQuality.PassedCases++
		}
		report.Cases = append(report.Cases, result)
		t.Logf("qualification case %s passed=%t latency_ms=%d", qualificationCaseLabel(index, testCase), result.Passed, latency)
	}
	for index := len(report.Cases); index < len(corpus.Cases); index++ {
		falseNegative += len(corpus.Cases[index].ExpectedCandidateIDs)
	}
	report.MeasuredQuality.CaseCount = len(corpus.Cases)
	report.MeasuredQuality.ExactMatchRate = ratio(report.MeasuredQuality.PassedCases, len(corpus.Cases))
	report.MeasuredQuality.MicroPrecision = ratio(truePositive, truePositive+falsePositive)
	report.MeasuredQuality.MicroRecall = ratio(truePositive, truePositive+falseNegative)
	report.MeasuredLatencyMS = summarizeQualificationLatencies(latencies)
	report.Passed = len(report.Cases) == len(corpus.Cases) &&
		report.MeasuredQuality.PassedCases == len(corpus.Cases)
	report.Qualified = report.Passed && report.MeasuredLatencyMS.Median <= medianTarget
	return report
}

func executeBrowserContextRelevanceSelection(
	ctx context.Context,
	broker *browserinference.ContextRelevanceBroker,
	model string,
	authority assemblyline.ContextRelevanceInput,
) ([]string, error) {
	selected := make([]string, 0, authority.MaxSelections)
	for len(selected) < authority.MaxSelections {
		decision, err := broker.ExecuteContextRelevance(
			ctx,
			model,
			assemblyline.ContextRelevanceSelectionInput{
				Authority:            authority,
				AcceptedCandidateIDs: append([]string{}, selected...),
			},
		)
		if err != nil {
			return nil, err
		}
		if decision.CandidateID == assemblyline.ContextRelevanceNoCandidate {
			break
		}
		selected = append(selected, decision.CandidateID)
	}
	return selected, nil
}

func waitForQualificationBrowser(
	ctx context.Context,
	broker *browserinference.ContextRelevanceBroker,
	browserDone <-chan struct{},
	browserExitErr *error,
) error {
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		if broker.Ready() {
			return nil
		}
		select {
		case <-browserDone:
			return fmt.Errorf(
				"qualification browser exited before its WebGPU model connected: %v",
				*browserExitErr,
			)
		case <-ticker.C:
		case <-ctx.Done():
			return fmt.Errorf("qualification browser did not become ready: %w", ctx.Err())
		}
	}
}

func writeBrowserContextQualificationReport(
	t *testing.T,
	path string,
	report browserContextQualificationReport,
) {
	t.Helper()
	raw, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatalf("encode browser context qualification report: %v", err)
	}
	raw = append(raw, '\n')
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatalf("write browser context qualification report: %v", err)
	}
}

func qualificationBrowserVersion(t *testing.T, path string) string {
	t.Helper()
	output, err := exec.Command(path, "--version").CombinedOutput()
	if err != nil {
		t.Fatalf("read qualification browser version: %v", err)
	}
	version := strings.TrimSpace(string(output))
	if version == "" {
		t.Fatal("qualification browser returned an empty version")
	}
	return version
}

func requireQualificationEnvironment(t *testing.T, key string) string {
	t.Helper()
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		t.Fatalf("%s must be set when browser context qualification is enabled", key)
	}
	return value
}

func requirePositiveQualificationInt64(t *testing.T, key string) int64 {
	t.Helper()
	value := requireQualificationEnvironment(t, key)
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed < 1 {
		t.Fatalf("%s must be one positive integer, received %q", key, value)
	}
	return parsed
}
