package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
)

func TestChatChannelOptionsAreEscapedServerComponentsWithoutVisiblePagination(t *testing.T) {
	t.Parallel()
	next := 20
	page, err := renderChatChannelOptionsPage([]model.Channel{{
		ID: "chat-42", Scope: model.ChannelScopeUser, Name: `<script>unsafe</script>`,
		Tags: []string{"user-channel"}, ProjectID: 42, WorkspaceRoot: "/workspace/project",
		Mode:      model.ChannelModeAssistant,
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}}, &next, false, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		`data-recyclr-target="channel-options"`,
		`<option value="" disabled selected>New conversation</option>`,
		`<option value="chat-42" data-channel-mode="assistant">`,
		`&lt;script&gt;unsafe&lt;/script&gt;`, `&lt;/script&gt; · assistant`,
	} {
		if !strings.Contains(page.HTML.Bundle, expected) {
			t.Errorf("channel component lacks %q: %s", expected, page.HTML.Bundle)
		}
	}
	if !page.HasMore {
		t.Fatalf("channel page=%+v", page)
	}
	for _, forbidden := range []string{
		"loadMoreChannels", "channel-options-pagination", `data-next-offset=`,
		"__omnidex_new_conversation__", "+ New conversation",
	} {
		if strings.Contains(page.HTML.Bundle, forbidden) {
			t.Errorf("channel component exposes obsolete pagination control %q: %s", forbidden, page.HTML.Bundle)
		}
	}
}

func TestChatChannelOptionsKeepOneNeutralStateForEmptyListAndOffAppendedPages(t *testing.T) {
	t.Parallel()
	empty, err := renderChatChannelOptionsPage(nil, nil, false, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		`<option value="" disabled selected>New conversation</option>`,
	} {
		if !strings.Contains(empty.HTML.Bundle, expected) {
			t.Errorf("empty channel component lacks %q: %s", expected, empty.HTML.Bundle)
		}
	}
	if strings.Count(empty.HTML.Bundle, `<option value=""`) != 1 {
		t.Fatalf("empty channel component must render one neutral state: %s", empty.HTML.Bundle)
	}

	appended, err := renderChatChannelOptionsPage([]model.Channel{{
		ID: "chat-43", Scope: model.ChannelScopeUser, Name: "Next chat",
		Tags: []string{"user-channel"}, ProjectID: 42, WorkspaceRoot: "/workspace/project",
		Mode: model.ChannelModeAssistant, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}}, nil, true, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{`value=""`, "New conversation"} {
		if strings.Contains(appended.HTML.Bundle, forbidden) {
			t.Errorf("appended channel component duplicated reset-only control %q: %s", forbidden, appended.HTML.Bundle)
		}
	}
	if !strings.Contains(appended.HTML.Bundle, `data-recyclr-location="beforeend"`) {
		t.Fatalf("appended channel component does not append: %s", appended.HTML.Bundle)
	}
}

func TestChatChannelOptionCarriesImmutableServerDataSourceBinding(t *testing.T) {
	t.Parallel()
	page, err := renderChatChannelOptionsPage([]model.Channel{{
		ID: "chat-42", Scope: model.ChannelScopeUser, Name: "Evidence chat",
		Tags: []string{"user-channel"}, ProjectID: 42, WorkspaceRoot: "/workspace/project",
		DataSourceID: "ds.primary-1", Mode: model.ChannelModeAssistant,
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}}, nil, false, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		`data-channel-mode="assistant"`, `data-data-source-id="ds.primary-1"`,
		`Evidence chat · assistant · data connected`,
	} {
		if !strings.Contains(page.HTML.Bundle, expected) {
			t.Errorf("bound channel option lacks %q: %s", expected, page.HTML.Bundle)
		}
	}
}

func TestChatChannelOptionCarriesPersistedRoleplayModeAndOpaqueViewpoint(t *testing.T) {
	t.Parallel()
	page, err := renderChatChannelOptionsPage([]model.Channel{{
		ID: "story-42", Scope: model.ChannelScopeUser, Name: "Harbor story",
		Tags: []string{"user-channel"}, ProjectID: 42, WorkspaceRoot: "/workspace/project",
		Mode:                         model.ChannelModeRoleplay,
		RoleplayViewpointCharacterID: "rpc_0123456789abcdef0123456789abcdef",
		CreatedAt:                    time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}}, nil, false, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		`data-channel-mode="roleplay"`,
		`data-roleplay-viewpoint-character-id="rpc_0123456789abcdef0123456789abcdef"`,
		`Harbor story · roleplay viewpoint rpc_0123456789abcdef0123456789abcdef`,
	} {
		if !strings.Contains(page.HTML.Bundle, expected) {
			t.Errorf("roleplay channel option lacks %q: %s", expected, page.HTML.Bundle)
		}
	}
	if strings.Contains(page.HTML.Bundle, "roleplay_world_name") || strings.Contains(page.HTML.Bundle, "Alice") {
		t.Fatalf("roleplay channel option exposed creation-only names: %s", page.HTML.Bundle)
	}
}

func TestChatChannelOptionsHTTPScopesAssistantThreadsAndRoleplayWorlds(t *testing.T) {
	t.Parallel()
	server, store := newChannelFrontdoorTestServer(t)
	store.channels["story-42"] = model.Channel{
		ID: "story-42", Scope: model.ChannelScopeUser, Name: "Harbor story",
		Tags: []string{"user-channel"}, ProjectID: 42, WorkspaceRoot: "/srv/workspaces/story-42",
		Mode: model.ChannelModeRoleplay, RoleplayViewpointCharacterID: "rpc_0123456789abcdef0123456789abcdef",
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}

	for _, test := range []struct {
		mode    string
		present string
		absent  string
		neutral string
	}{
		{mode: "assistant", present: "authority", absent: "story-42", neutral: "New conversation"},
		{mode: "roleplay", present: "story-42", absent: "authority", neutral: "Select a world"},
	} {
		request := httptest.NewRequest(http.MethodGet, "/v1/ui/chat/channels?limit=20&offset=0&mode="+test.mode, nil)
		response := httptest.NewRecorder()
		server.Handler().ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("mode=%s status=%d body=%s", test.mode, response.Code, response.Body.String())
		}
		body := response.Body.String()
		if !strings.Contains(body, test.present) || !strings.Contains(body, test.neutral) {
			t.Errorf("mode=%s lacks scoped option or neutral label: %s", test.mode, body)
		}
		if strings.Contains(body, test.absent) {
			t.Errorf("mode=%s leaked an out-of-scope channel: %s", test.mode, body)
		}
	}
}

func TestChatDataSourceOptionsExposeOnlyEscapedOpaqueIdentity(t *testing.T) {
	t.Parallel()
	next := 20
	page, err := renderChatDataSourceOptionsPage([]queue.DataSourceRecord{{
		ID: "ds.primary-1", Name: `<script>Customer DB</script>`, Driver: "postgres",
		Host: "private.internal", Port: 5432, DatabaseName: "secret_db", Username: "admin",
		Password: "password-secret", DSN: "postgres://secret-dsn", ReadOnly: true,
	}}, &next, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		`data-recyclr-target="new-channel-data-source-options"`,
		`<option value="" selected>No data</option>`,
		`<option value="ds.primary-1">&lt;script&gt;Customer DB&lt;/script&gt;</option>`,
	} {
		if !strings.Contains(page.HTML.Bundle, expected) {
			t.Errorf("data-source component lacks %q: %s", expected, page.HTML.Bundle)
		}
	}
	for _, forbidden := range []string{
		"private.internal", "secret_db", "admin", "password-secret", "postgres://secret-dsn", "postgres", "5432",
	} {
		if strings.Contains(page.HTML.Bundle, forbidden) {
			t.Errorf("data-source component leaked %q: %s", forbidden, page.HTML.Bundle)
		}
	}
	if !page.HasMore || page.NextOffset == nil || *page.NextOffset != 20 {
		t.Fatalf("data-source page=%+v", page)
	}
	for _, forbidden := range []string{"loadMoreChatDataSources", "new-channel-data-source-pagination", `data-next-offset=`} {
		if strings.Contains(page.HTML.Bundle, forbidden) {
			t.Errorf("data-source component exposes obsolete pagination control %q: %s", forbidden, page.HTML.Bundle)
		}
	}
}

func TestChatDataSourceOptionsRejectMalformedStoredIdentity(t *testing.T) {
	t.Parallel()
	if _, err := renderChatDataSourceOptionsPage([]queue.DataSourceRecord{{
		ID: "NOT CANONICAL", Name: "Customer DB",
	}}, nil, false); err == nil {
		t.Fatal("malformed stored data-source identity was rendered")
	}
}

func TestChatDataSourceComponentRejectsInexactTransportBeforeRepositoryAccess(t *testing.T) {
	t.Parallel()
	server := &Server{}
	for _, test := range []struct {
		request *http.Request
		status  int
	}{
		{httptest.NewRequest(http.MethodPost, "/v1/ui/chat/data-sources?limit=20&offset=0", nil), http.StatusMethodNotAllowed},
		{httptest.NewRequest(http.MethodGet, "/v1/ui/chat/data-sources?limit=20&offset=0&unknown=1", nil), http.StatusBadRequest},
	} {
		response := httptest.NewRecorder()
		server.handleChatDataSourceOptions(response, test.request)
		if response.Code != test.status {
			t.Errorf("method=%s status=%d want=%d body=%s", test.request.Method, response.Code, test.status, response.Body.String())
		}
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
		Models: []queue.TelemetryModelSummary{{Calls: 5, Failures: 1}},
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
		`Context utilization`, `42.5%`, `Failure events`,
	} {
		if !strings.Contains(bundle, expected) {
			t.Errorf("metrics component lacks %q: %s", expected, bundle)
		}
	}
}
