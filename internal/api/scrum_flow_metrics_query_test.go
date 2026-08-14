package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDecodeScrumFlowMetricsQueryUsesOnlyExplicitOmissionDefaults(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/v1/scrum/flow-metrics?project_id=17", nil)
	query, err := decodeScrumFlowMetricsQuery(request)
	if err != nil {
		t.Fatal(err)
	}
	if query != (scrumFlowMetricsQuery{ProjectID: 17, Limit: 25, Offset: 0}) {
		t.Fatalf("query=%+v", query)
	}

	request = httptest.NewRequest(
		http.MethodGet,
		"/v1/scrum/flow-metrics?project_id=17&limit=100&offset=1000000",
		nil,
	)
	query, err = decodeScrumFlowMetricsQuery(request)
	if err != nil {
		t.Fatal(err)
	}
	if query != (scrumFlowMetricsQuery{ProjectID: 17, Limit: 100, Offset: 1_000_000}) {
		t.Fatalf("query=%+v", query)
	}
}

func TestDecodeScrumFlowMetricsQueryRejectsUnregisteredOrInexactTransport(t *testing.T) {
	for _, rawQuery := range []string{
		"",
		"project_id=17&extra=true",
		"project_id=17&project_id=18",
		"project_id=17&limit=25&limit=26",
		"project_id=17&offset=0&offset=1",
		"Project_ID=17",
		"project_id=017",
		"project_id=+17",
		"project_id=%2017",
		"project_id=-1",
		"project_id=17&limit=025",
		"project_id=17&limit=0",
		"project_id=17&limit=101",
		"project_id=17&offset=00",
		"project_id=17&offset=-1",
		"project_id=17&offset=1000001",
		"project_id=17&limit=%FF",
		"project_id=17%00",
		"project_id=17;limit=1",
	} {
		request := httptest.NewRequest(http.MethodGet, "/v1/scrum/flow-metrics?"+rawQuery, nil)
		if _, err := decodeScrumFlowMetricsQuery(request); err == nil {
			t.Fatalf("query %q unexpectedly accepted", rawQuery)
		}
	}
	request := httptest.NewRequest(
		http.MethodGet,
		"/v1/scrum/flow-metrics?project_id=17&extra="+strings.Repeat("x", maxScrumFlowMetricsRawQuerySize),
		nil,
	)
	if _, err := decodeScrumFlowMetricsQuery(request); err == nil {
		t.Fatal("oversized query unexpectedly accepted")
	}
}

func TestScrumFlowMetricsRejectsMalformedQueryBeforeRepositoryAvailability(t *testing.T) {
	server := NewServer(nil, &fakeLLMClient{})
	request := httptest.NewRequest(http.MethodGet, "/v1/scrum/flow-metrics?project_id=17&fallback=true", nil)
	recorder := httptest.NewRecorder()

	server.handleScrumFlowMetrics(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}
