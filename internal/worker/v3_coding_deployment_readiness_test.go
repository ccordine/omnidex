package worker

import (
	"bytes"
	"context"
	"net/http"
	"strings"
	"testing"
)

func TestDirectCodingDeploymentReadinessRequiresExactEmpty204(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		status   int
		body     []byte
		location string
		want     string
	}{
		{name: "ready", status: http.StatusNoContent},
		{name: "wrong status", status: http.StatusOK, want: "HTTP status 200"},
		{name: "redirect", status: http.StatusTemporaryRedirect, location: "http://elsewhere.invalid/", want: "redirects are forbidden"},
		{name: "oversize", status: http.StatusOK, body: bytes.Repeat([]byte{'x'}, directCodingReadinessBodyLimit+1), want: "byte limit"},
		{name: "body on 204", status: http.StatusNoContent, body: []byte("unexpected"), want: "contains a body"},
	}
	for _, testCase := range cases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			transport := directCodingRoundTripFunc(func(request *http.Request) (*http.Response, error) {
				response := directCodingHTTPResponse(request, testCase.status, "text/plain", testCase.body)
				if testCase.location != "" {
					response.Header.Set("Location", testCase.location)
				}
				return response, nil
			})
			err := probeDirectCodingDeploymentReadinessWithClient(
				context.Background(), newDirectCodingReadinessClient(transport),
				"http://127.0.0.1:41000"+directCodingDeploymentReadinessPath,
			)
			if testCase.want == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), testCase.want) {
				t.Fatalf("error=%v, want substring %q", err, testCase.want)
			}
		})
	}
}

func TestDirectCodingDeploymentReadinessRejectsUnregisteredAuthorityBeforeIO(t *testing.T) {
	t.Parallel()
	for _, testCase := range []struct {
		host string
		port uint16
		path string
	}{
		{host: "http://127.0.0.1", port: 41000, path: directCodingDeploymentReadinessPath},
		{host: "127.0.0.1", port: 0, path: directCodingDeploymentReadinessPath},
		{host: "127.0.0.1", port: 41000, path: "/other"},
	} {
		if err := probeDirectCodingDeploymentReadiness(
			context.Background(), testCase.host, testCase.port, testCase.path,
		); err == nil {
			t.Fatalf("accepted unregistered readiness authority: %+v", testCase)
		}
	}
}
