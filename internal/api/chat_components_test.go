package api

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
)

func TestChatChannelOptionsAreEscapedPaginatedServerComponents(t *testing.T) {
	t.Parallel()
	next := 20
	page, err := renderChatChannelOptionsPage([]model.Channel{{
		ID: "chat-42", Scope: model.ChannelScopeUser, Name: `<script>unsafe</script>`,
		Tags: []string{"user-channel"}, ProjectID: 42, WorkspaceRoot: "/workspace/project",
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}}, &next, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		`data-recyclr-target="channel-options"`, `<option value="chat-42">`,
		`&lt;script&gt;unsafe&lt;/script&gt;`, `data-action="chat#loadMoreChannels"`, `data-next-offset="20"`,
	} {
		if !strings.Contains(page.HTML.Bundle, expected) {
			t.Errorf("channel component lacks %q: %s", expected, page.HTML.Bundle)
		}
	}
	if page.DefaultChannelID == nil || *page.DefaultChannelID != "chat-42" || !page.HasMore {
		t.Fatalf("channel page=%+v", page)
	}
}

func TestChatJobsAreEscapedServerComponentsWithExactControls(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	job := model.Job{
		ID: 7, Instruction: `<img src=x onerror=alert(1)>`, Pipeline: model.PipelineChat,
		Status: model.JobStatusRunning, CreatedAt: now, UpdatedAt: now,
	}
	page, err := renderChatJobsPage([]model.Job{job}, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(page.HTML.Bundle, "<img") || !strings.Contains(page.HTML.Bundle, "&lt;img") ||
		!strings.Contains(page.HTML.Bundle, `data-action="chat#selectJob"`) {
		t.Fatalf("unsafe or incomplete job component: %s", page.HTML.Bundle)
	}
	job.CurrentGeneration = 1
	bundle, err := renderChatJobStateBundle(queue.JobPresentation{
		Job: job,
		Steps: []model.Step{{
			ID: 11, JobID: job.ID, Action: "v3_coding", Status: model.StepStatusRunning,
			Generation: 1, SortIndex: 0, CreatedAt: now, UpdatedAt: now,
		}},
		Progress: queue.JobProgressPage{JobID: job.ID, Generation: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		`data-recyclr-target="progress"`, `data-recyclr-target="job-details"`,
		`data-recyclr-target="job-progress-events"`, `Latest activity`,
		`data-action="chat#interruptJob"`, `data-action="chat#replanJob"`, `data-action="chat#cancelJob"`,
	} {
		if !strings.Contains(bundle, expected) {
			t.Errorf("job state component lacks %q: %s", expected, bundle)
		}
	}
}

func TestChatMemoryAndCandidatesAreEscapedPaginatedComponents(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	next := 20
	page, err := renderChatMemoryPage("all", queue.MemoryChunkPage{
		Items: []queue.MemoryChunkSummary{{
			ID: 1, Source: "server", Kind: model.MemoryKindReference,
			Content: `<script>memory</script>`, Tags: []string{"exact"}, CreatedAt: now,
		}}, NextOffset: &next, HasMore: true,
	}, queue.MemoryCandidatePage{Items: []model.MemoryCandidate{{
		ID: 2, CandidateKind: model.MemoryKindReference, Content: `<img src=x>`,
		Status: model.MemoryCandidateStatusCandidate, CreatedAt: now,
	}}}, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		`data-recyclr-target="memory-list"`, `&lt;script&gt;memory&lt;/script&gt;`,
		`data-recyclr-target="memory-candidates"`, `&lt;img src=x&gt;`,
		`data-authority="global"`, `data-action="chat#promoteMemory"`, `data-next-offset="20"`,
	} {
		if !strings.Contains(page.HTML.Bundle, expected) {
			t.Errorf("memory component lacks %q: %s", expected, page.HTML.Bundle)
		}
	}
}

func TestChatTimelineIsServerRenderedAndPaginated(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	next := 40
	page, err := renderChatTimelinePage([]model.Job{{
		ID: 9, Instruction: "Grounded job", Pipeline: model.PipelineChat,
		Status: model.JobStatusCompleted, CreatedAt: now, UpdatedAt: now,
	}}, &next, true, 40)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		`data-recyclr-target="timeline"`, `data-recyclr-location="beforeend"`,
		`data-action="chat#openTimelineJob"`, `data-next-offset="40"`,
		`data-recyclr-target="event-count"`, `40 jobs`,
	} {
		if !strings.Contains(page.HTML.Bundle, expected) {
			t.Errorf("timeline component lacks %q: %s", expected, page.HTML.Bundle)
		}
	}
}

func TestChatComponentsRejectInvalidStoredPresentation(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	if _, err := renderChatJobsPage([]model.Job{{
		ID: 1, Instruction: "job", Pipeline: model.PipelineChat,
		Status: "invented", CreatedAt: now, UpdatedAt: now,
	}}, nil, false); err == nil {
		t.Fatal("unsupported job status was rendered")
	}
	if _, err := renderChatMemoryPage("memory", queue.MemoryChunkPage{Items: []queue.MemoryChunkSummary{{
		ID: 1, Source: "source", Kind: model.MemoryKindReference,
		Content: "memory", Tags: []string{"bad\x00tag"}, CreatedAt: now,
	}}}, queue.MemoryCandidatePage{}, false); err == nil {
		t.Fatal("invalid stored memory tag was rendered")
	}
}

func TestChatComponentQueriesRejectDuplicateAndNoncanonicalValues(t *testing.T) {
	t.Parallel()
	for _, query := range []string{"status=running&status=failed", "status=%20running", "kind=unknown"} {
		request := httptest.NewRequest("GET", "/v1/ui/chat/jobs?"+query, nil)
		key := "status"
		if strings.HasPrefix(query, "kind=") {
			key = "kind"
		}
		value, err := exactChatComponentString(request, key, 64)
		if err == nil && key == "kind" {
			err = validateChatMemoryKindFilter(value)
		}
		if err == nil {
			t.Errorf("query %q was accepted", query)
		}
	}
}

func TestChatHostBridgeStatusIsEscapedServerMarkup(t *testing.T) {
	t.Parallel()
	bundle, err := renderChatHostBridgeStatusBundle(hostBridgeStatusResponse{
		Configured: true, URL: "http://host.test/<unsafe>", Error: `<script>bridge</script>`,
		Message: "Unavailable", Suggestions: []string{`Run <command>`},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		`data-recyclr-target="host-bridge-status-output"`, `http://host.test/&lt;unsafe&gt;`,
		`&lt;script&gt;bridge&lt;/script&gt;`, `Run &lt;command&gt;`, `role="status"`,
	} {
		if !strings.Contains(bundle, expected) {
			t.Errorf("host status component lacks %q: %s", expected, bundle)
		}
	}
	if strings.Contains(bundle, "<script>") {
		t.Fatal("host status component emitted unescaped script markup")
	}
}

func TestChatMetricsAreOneServerComputedSummaryWithoutClientLists(t *testing.T) {
	t.Parallel()
	bundle, err := renderChatMetricsBundle(chatMetricsSnapshot{
		Live: queue.TelemetryDashboardSummary{
			LiveRuns:     []queue.TelemetryRunSummary{{ID: "run-1"}},
			StatusCounts: map[string]int{"completed": 3, "failed": 1},
		},
		Models:     []queue.TelemetryModelSummary{{Calls: 5, Failures: 1}},
		Playbooks:  []queue.TelemetryPlaybookSummary{{Uses: 2}},
		Benchmarks: []queue.TelemetryBenchmarkSummary{{Runs: 4}},
		Usage: queue.LLMContextUsageMetricsResponse{Summary: queue.LLMContextUsageSummary{
			Requests: 7, AvgUtilization: 42.5, OverloadEvents: 0,
		}},
		Shrink:     queue.ContextShrinkMetricsResponse{Summary: queue.ContextShrinkMetricSummary{AvgSavedPct: 33.3}},
		Operations: queue.OperationsMetricsResponse{FailureCounts: []queue.TelemetryCountSummary{{Count: 2}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		`data-recyclr-target="metrics-output"`, `Server-computed summaries`, `Live runs`, `>1</dd>`,
		`Context utilization`, `42.5%`, `Failure events`, `Playbook uses`,
	} {
		if !strings.Contains(bundle, expected) {
			t.Errorf("metrics component lacks %q: %s", expected, bundle)
		}
	}
}
