package api

import (
	"net/http/httptest"
	"testing"
)

func TestDecodeJobItemRouteRegistersExactCodingPlanEndpoints(t *testing.T) {
	t.Parallel()
	tests := []struct {
		path   string
		action jobItemAction
	}{
		{path: "/v1/jobs/42/plan", action: jobItemPlanRead},
		{path: "/v1/jobs/42/plan/decisions", action: jobItemPlanDecisions},
		{path: "/v1/jobs/42/plan/freeze", action: jobItemPlanFreeze},
	}
	for _, test := range tests {
		test := test
		t.Run(test.path, func(t *testing.T) {
			t.Parallel()
			request := httptest.NewRequest("GET", test.path, nil)
			jobID, action, err := decodeJobItemRoute(request)
			if err != nil {
				t.Fatal(err)
			}
			if jobID != 42 || action != test.action {
				t.Fatalf("decoded route = job %d action %q", jobID, action)
			}
		})
	}
}

func TestDecodeJobItemRouteRejectsNoncanonicalCodingPlanPaths(t *testing.T) {
	t.Parallel()
	for _, path := range []string{
		"/v1/jobs/42/plan/",
		"/v1/jobs/042/plan",
		"/v1/jobs/42/plan/decisions/extra",
		"/v1/jobs/42/plan/freeze/extra",
		"/v1/jobs/42/plan/%64ecisions",
	} {
		path := path
		t.Run(path, func(t *testing.T) {
			t.Parallel()
			request := httptest.NewRequest("GET", path, nil)
			if _, _, err := decodeJobItemRoute(request); err == nil {
				t.Fatal("noncanonical coding plan route was accepted")
			}
		})
	}
}
