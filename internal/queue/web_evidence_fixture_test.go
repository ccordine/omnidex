package queue

import (
	"context"
	"fmt"
	"html"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/evidence"
	"github.com/gryph/omnidex/internal/webresearch"
	"github.com/gryph/omnidex/internal/websearch"
)

type webEvidenceTransport func(*http.Request) (*http.Response, error)

func (transport webEvidenceTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	return transport(request)
}

type webEvidenceResolver struct{}

func (webEvidenceResolver) LookupIPAddr(context.Context, string) ([]net.IPAddr, error) {
	return []net.IPAddr{{IP: net.ParseIP("8.8.8.8")}}, nil
}

type webEvidenceHTTPFixture struct {
	service  *websearch.Service
	query    string
	requests atomic.Int64
}

func newWebEvidenceHTTPFixture(t *testing.T, title, content string) *webEvidenceHTTPFixture {
	t.Helper()
	fixture := &webEvidenceHTTPFixture{query: title}
	const sourceURL = "https://source.example/document"
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		fixture.requests.Add(1)
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		switch request.URL.Path {
		case "/search":
			if request.URL.Query().Get("q") != title {
				t.Errorf("requested query = %q", request.URL.Query().Get("q"))
			}
			fmt.Fprint(writer, `<a href="`+sourceURL+`" class="l1">`+html.EscapeString(title)+`</a>`)
		case "/document":
			fmt.Fprint(writer, `<title>`+html.EscapeString(title)+`</title><p>`+html.EscapeString(content)+`</p>`)
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
	fixture.service, err = websearch.New(websearch.Config{
		Providers: []websearch.ProviderID{websearch.ProviderBrave}, Timeout: 3 * time.Second,
		PerDocumentBytes: 8192, TotalDocumentBytes: 8192, MaxResponseBytes: 16384,
		MaxCandidatesPerProvider: 1, MaxCandidates: 1, MaxDocuments: 1,
		Resolver: webEvidenceResolver{}, HTTPClient: &http.Client{Transport: webEvidenceTransport(func(request *http.Request) (*http.Response, error) {
			// Only test transport redirects the requested public origin to the
			// local fixture server. Production URL/DNS checks still execute.
			cloned := request.Clone(request.Context())
			cloned.URL.Scheme, cloned.URL.Host = local.Scheme, local.Host
			return transport.RoundTrip(cloned)
		})},
	})
	if err != nil {
		t.Fatal(err)
	}
	return fixture
}

func (fixture *webEvidenceHTTPFixture) acquire(t *testing.T) webresearch.AcquiredEvidence {
	t.Helper()
	ctx := context.Background()
	discovery, err := fixture.service.Discover(ctx, websearch.QueryRequest{Query: fixture.query})
	if err != nil {
		t.Fatal(err)
	}
	ids := []websearch.CandidateID{discovery.Candidates[0].ID}
	fetched, err := fixture.service.Fetch(ctx, websearch.FetchRequest{Candidates: discovery.Candidates, CandidateIDs: ids})
	if err != nil {
		t.Fatal(err)
	}
	return webresearch.AcquiredEvidence{Discovery: discovery, Fetch: fetched, CandidateIDs: ids, ProjectionBytes: 8192}
}

func webEvidenceCitation(t *testing.T, stored WebEvidenceRecord, stepID int64) evidence.Record {
	t.Helper()
	sources, projected, err := stored.Acquired.Project()
	if err != nil {
		t.Fatal(err)
	}
	excerpt, truncated, err := webresearch.CitationExcerpt(projected[0])
	if err != nil {
		t.Fatal(err)
	}
	source := sources[0]
	requirement := fmt.Sprintf("objective-%d-1-requirement", stored.JobID)
	return evidence.Record{
		JobID: stored.JobID, StepID: stepID, Kind: evidence.KindObjectiveCitation,
		SourceType: "web_document", SourceRef: source.URL, Excerpt: excerpt,
		Summary: "The fetched source supports this paragraph.", Confidence: 1,
		RequirementAuthorityBindings: []string{requirement + "#paragraph-1"},
		Metadata: map[string]any{
			"capsule_id": string(source.ID), "objective_id": fmt.Sprintf("objective-%d-1", stored.JobID),
			"objective_kind": "external_answer", "requirement_id": requirement,
			"web_evidence_id": strconv.FormatInt(stored.ID, 10), "web_evidence_index": "0",
			"paragraph_indexes": []int{0}, "source_observed_at": source.ObservedAt.Format(time.RFC3339Nano),
			"source_truncated": source.Truncated || projected[0].Truncated || truncated,
		},
	}
}
