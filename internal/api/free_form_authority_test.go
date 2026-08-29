package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
)

func TestRequireFreeFormAuthorityPreservesExactBytes(t *testing.T) {
	t.Parallel()

	exact := "  retain this instruction\nwith its trailing tab\t "
	got, err := requireFreeFormAuthority(exact, "instruction")
	if err != nil {
		t.Fatal(err)
	}
	if got != exact {
		t.Fatalf("authority changed: got %q want %q", got, exact)
	}
}

func TestRequireFreeFormAuthorityRejectsBlankInput(t *testing.T) {
	t.Parallel()

	for _, blank := range []string{"", " ", "\n\t "} {
		if _, err := requireFreeFormAuthority(blank, "instruction"); err == nil {
			t.Fatalf("blank authority accepted: %q", blank)
		}
	}
}

func TestEnqueueHTTPRejectsBlankInstructionBeforeRepositoryAccess(t *testing.T) {
	t.Parallel()

	request := httptest.NewRequest(
		http.MethodPost,
		"/v1/jobs",
		bytes.NewBufferString(`{"instruction":" \n\t ","pipeline":"chat","metadata":{}}`),
	)
	response := httptest.NewRecorder()
	(&Server{}).enqueueJob(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestGenericJobRouteRejectsFreeFormPipelines(t *testing.T) {
	t.Parallel()
	server := NewServer(nil, nil)
	server.repo = &queue.Repository{}
	server.mux = http.NewServeMux()
	server.routes()
	for _, pipeline := range []string{model.PipelineChat, model.PipelineScrum} {
		request := httptest.NewRequest(
			http.MethodPost,
			"/v1/jobs",
			bytes.NewBufferString(`{"instruction":"exact","pipeline":"`+pipeline+`","metadata":{}}`),
		)
		response := httptest.NewRecorder()
		server.Handler().ServeHTTP(response, request)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("pipeline=%s status=%d body=%s", pipeline, response.Code, response.Body.String())
		}
		if pipeline == model.PipelineChat && !strings.Contains(response.Body.String(), "channel") {
			t.Fatalf("pipeline=%s did not direct free-form input to channels: %s", pipeline, response.Body.String())
		}
	}
	for _, pipeline := range []string{"assistant", "story", "agent"} {
		if err := validateGenericJobPipeline(pipeline); err == nil || !strings.Contains(err.Error(), "unsupported") {
			t.Fatalf("retired pipeline %q error=%v", pipeline, err)
		}
	}
}

func TestGenericJobRouteAcceptsOnlyExactExplicitPipelines(t *testing.T) {
	t.Parallel()
	if err := validateGenericJobPipeline(model.PipelineCoding); err != nil {
		t.Fatalf("explicit coding pipeline rejected: %v", err)
	}
	for _, pipeline := range []string{"", "unknown", " coding", "CODING", model.PipelineScrum} {
		if err := validateGenericJobPipeline(pipeline); err == nil {
			t.Fatalf("noncanonical/unknown pipeline %q accepted", pipeline)
		}
	}
}

func TestExplicitCodingEnqueuePreservesExactInstructionInPostgres(t *testing.T) {
	pool := openIsolatedAPIDatabasePool(t)
	repository := queue.New(pool)
	if err := repository.ResetDatabase(t.Context(), loadAPITestDatabaseSetup(t)); err != nil {
		t.Fatal(err)
	}
	server := &Server{repo: repository}
	exact := "  preserve queued authority\nwith trailing tab\t "
	body, err := json.Marshal(map[string]any{
		"instruction": exact,
		"pipeline":    model.PipelineCoding,
		"metadata": map[string]any{
			"client_cwd":   "/srv/workspaces/exact-authority",
			"host_env_cwd": "/srv/workspaces/exact-authority",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/v1/jobs", bytes.NewReader(body))
	response := httptest.NewRecorder()
	server.enqueueJob(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var payload struct {
		Job model.Job `json:"job"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Job.Instruction != exact {
		t.Fatalf("stored instruction changed: got %q want %q", payload.Job.Instruction, exact)
	}
}
