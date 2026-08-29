package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
)

func TestChatJobProgressIsTypedEscapedServerMarkup(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC().Truncate(time.Second)
	presentation := chatProgressPresentationFixture(now, []string{
		"event=coding_target_tree_validation_failed diagnostic=<unsafe> tree failure",
		"event=objective_worker_started kind=semantic subject=context_minification model=local attempt=1/3 context=prompt:120B,capabilities:0B,current:0B,correction:0B",
		"event=repository_snapshot_failed <script>alert(1)</script> exact failure",
		"event=coding_phase_changed phase=deploying detail=deploying verified workload",
	})
	bundle, err := renderChatJobStateBundle(presentation)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		`data-recyclr-target="job-progress-events"`,
		`Target tree validation failed: &lt;unsafe&gt; tree failure`,
		`Context minification station started (attempt 1/3)`,
		`Repository snapshot failed: &lt;script&gt;alert(1)&lt;/script&gt; exact failure`,
		`Deploying the verified workload`,
		`Latest 4 authoritative events`,
	} {
		if !strings.Contains(bundle, expected) {
			t.Errorf("progress component lacks %q: %s", expected, bundle)
		}
	}
	if strings.Contains(bundle, "<script>") {
		t.Fatal("progress component emitted an unescaped diagnostic")
	}
}

func TestChatJobProgressRejectsMalformedOrUnknownEventAuthority(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC().Truncate(time.Second)
	for _, raw := range []string{
		"event=coding_file_written path=main.go bytes=12 operation=create",
		"event=invented_agent_thinking secret=hidden",
		"time=not-a-time event=coding_phase_changed phase=deploying detail=deploying",
		"event=objective_worker_started kind=semantic model=local attempt=1/3",
	} {
		presentation := chatProgressPresentationFixture(now, []string{raw})
		if _, err := renderChatJobStateBundle(presentation); err == nil {
			t.Errorf("malformed progress authority %q was rendered", raw)
		}
	}
}

func TestChatJobStateResponseExposesOnlyBoundedAuthorityAndServerMarkup(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC().Truncate(time.Second)
	presentation := chatProgressPresentationFixture(now, []string{
		"event=repository_snapshot_ready snapshot=sha256:abc files=7",
	})
	response, err := newChatJobStateResponse(presentation)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	encoded := string(raw)
	for _, expected := range []string{`"current_generation":1`, `"latest_context_id":1`, `"count":1`, `Repository snapshot ready`} {
		if !strings.Contains(encoded, expected) {
			t.Errorf("job state response lacks %q: %s", expected, encoded)
		}
	}
	for _, forbidden := range []string{`"contexts"`, `"value"`, `"metadata"`, `llm_prompt`, `chain-of-thought`} {
		if strings.Contains(encoded, forbidden) {
			t.Errorf("job state response exposed forbidden raw authority %q: %s", forbidden, encoded)
		}
	}
}

func TestChatJobStateComponentPathIsExact(t *testing.T) {
	t.Parallel()
	for _, testCase := range []struct {
		path string
		want int64
		ok   bool
	}{
		{path: "/v1/ui/chat/jobs/42", want: 42, ok: true},
		{path: "/v1/ui/chat/jobs/42/", ok: false},
		{path: "/v1/ui/chat/jobs/42/progress", ok: false},
		{path: "/v1/ui/chat/jobs/0", ok: false},
		{path: "/v1/ui/chat/jobs/+42", ok: false},
	} {
		got, err := parseChatJobStateComponentID(testCase.path)
		if testCase.ok && (err != nil || got != testCase.want) {
			t.Errorf("parse %q = %d, %v; want %d", testCase.path, got, err, testCase.want)
		}
		if !testCase.ok && err == nil {
			t.Errorf("noncanonical component path %q was accepted", testCase.path)
		}
	}
}

func TestChatJobStateComponentRejectsMalformedRequestsBeforeRepositoryAccess(t *testing.T) {
	t.Parallel()
	server := &Server{}
	for _, testCase := range []struct {
		method string
		path   string
		status int
	}{
		{method: http.MethodPost, path: "/v1/ui/chat/jobs/7", status: http.StatusMethodNotAllowed},
		{method: http.MethodGet, path: "/v1/ui/chat/jobs/7?limit=1", status: http.StatusBadRequest},
		{method: http.MethodGet, path: "/v1/ui/chat/jobs/7/", status: http.StatusBadRequest},
	} {
		request := httptest.NewRequest(testCase.method, testCase.path, nil)
		response := httptest.NewRecorder()
		server.handleChatJobStateComponent(response, request)
		if response.Code != testCase.status {
			t.Errorf("%s %s status=%d want %d", testCase.method, testCase.path, response.Code, testCase.status)
		}
	}
}

func chatProgressPresentationFixture(now time.Time, events []string) queue.JobPresentation {
	job := model.Job{
		ID: 7, Instruction: "Build the exact requested change", Pipeline: model.PipelineChat,
		Status: model.JobStatusRunning, CurrentGeneration: 1, CreatedAt: now, UpdatedAt: now,
	}
	progress := queue.JobProgressPage{JobID: job.ID, Generation: 1}
	for index, event := range events {
		contextID := int64(index + 1)
		if !strings.HasPrefix(event, "time=") {
			event = "time=" + now.Format(time.RFC3339) + " " + event
		}
		progress.Items = append(progress.Items, queue.JobProgressContext{
			Context: model.StepContext{
				ID: contextID, StepID: 11, Key: "event", Value: event, CreatedAt: now,
			},
			Generation: 1, StepAction: "v3_coding",
		})
		progress.LatestContextID = contextID
	}
	return queue.JobPresentation{
		Job: job,
		Steps: []model.Step{{
			ID: 11, JobID: job.ID, Action: "v3_coding", Status: model.StepStatusRunning,
			Generation: 1, SortIndex: 0, CreatedAt: now, UpdatedAt: now,
		}},
		Progress: progress,
	}
}
