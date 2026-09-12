package worker

import (
	"context"
	"fmt"
	"html"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/websearch"
)

type researchHTTPFixture struct {
	service     *websearch.Service
	requests    atomic.Int64
	invalidPath atomic.Pointer[string]
}

type researchFixtureTransport func(*http.Request) (*http.Response, error)

func (transport researchFixtureTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	return transport(request)
}

type researchFixtureResolver struct{}

func (researchFixtureResolver) LookupIPAddr(context.Context, string) ([]net.IPAddr, error) {
	return []net.IPAddr{{IP: net.ParseIP("8.8.8.8")}}, nil
}

func newResearchHTTPFixture(t *testing.T, fixture researchWorkflowCase) *researchHTTPFixture {
	t.Helper()
	result := &researchHTTPFixture{}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		result.requests.Add(1)
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		if invalid := result.invalidPath.Load(); invalid != nil && request.URL.Path == *invalid {
			fmt.Fprint(writer, "unusable\x00response")
			return
		}
		switch request.URL.Path {
		case "/search":
			if request.URL.Query().Get("q") != fixture.question {
				t.Errorf("discovery changed the exact question: %q", request.URL.Query().Get("q"))
			}
			fmt.Fprint(writer, `<a href="https://source.example/first" class="l1">First source</a><a href="https://source.example/second" class="l1">Second source</a>`)
		case "/first", "/second":
			start := 0
			if request.URL.Path == "/second" {
				start = 2
			}
			fmt.Fprint(writer, `<title>`+html.EscapeString(fixture.name)+`</title><p>`+html.EscapeString(strings.Join(fixture.paragraphs[start:start+2], " "))+`</p>`)
		default:
			http.Error(writer, "unexpected fixture request", http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)
	local, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	transport := &http.Transport{}
	t.Cleanup(transport.CloseIdleConnections)
	result.service, err = websearch.New(websearch.Config{
		Providers: []websearch.ProviderID{websearch.ProviderBrave}, Timeout: 3 * time.Second,
		PerDocumentBytes: 8192, TotalDocumentBytes: 16384, MaxResponseBytes: 32768,
		MaxCandidatesPerProvider: 2, MaxCandidates: 2, MaxDocuments: 2,
		Resolver: researchFixtureResolver{}, HTTPClient: &http.Client{Transport: researchFixtureTransport(func(request *http.Request) (*http.Response, error) {
			// Production discovery, URL/DNS checks, fetch and parsing still run;
			// only this test transport redirects the public origin to localhost.
			cloned := request.Clone(request.Context())
			cloned.URL.Scheme, cloned.URL.Host = local.Scheme, local.Host
			return transport.RoundTrip(cloned)
		})},
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}
