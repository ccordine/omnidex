package webresearch

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/websearch"
)

func TestHTTPAcquisitionFailureReturnsObservedSourcesWithoutSemanticWork(t *testing.T) {
	for _, subject := range []string{"Inspection timetable", "Publication catalogue"} {
		for _, phase := range []string{"discovery", "fetch"} {
			failures := []string{"cancelled", "invalid_text"}
			if phase == "discovery" {
				failures = append(failures, "parse_bounds")
			} else {
				failures = append(failures, "redirect")
			}
			for _, failure := range failures {
				t.Run(subject+"/"+phase+"/"+failure, func(t *testing.T) {
					ctx, cancel := context.WithCancel(context.Background())
					defer cancel()
					var mutex sync.Mutex
					var requests []string
					server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
						mutex.Lock()
						requests = append(requests, request.Host+request.URL.Path)
						mutex.Unlock()
						if phase == "discovery" && request.Host == "duckduckgo.com" || phase == "fetch" && request.URL.Path == "/second" {
							switch failure {
							case "cancelled":
								cancel()
								<-request.Context().Done()
							case "invalid_text":
								fmt.Fprint(writer, "unusable\x00response")
							case "parse_bounds":
								fmt.Fprintf(writer, `<a class="result__a" href="https://reading.example/second">%s</a>`, strings.Repeat("x", 4097))
							case "redirect":
								writer.Header().Set("Location", "https://elsewhere.example/unrequested")
								writer.WriteHeader(http.StatusFound)
							}
							return
						}
						switch request.Host {
						case "search.brave.com", "duckduckgo.com":
							if request.URL.Query().Get("q") != subject {
								t.Errorf("query changed: %s", request.URL.String())
							}
							if request.Host == "search.brave.com" {
								fmt.Fprintf(writer, `<a href="https://reading.example/first" class="l1">%s</a><a href="https://reading.example/second" class="l1">Second source</a>`, subject)
							} else {
								fmt.Fprintf(writer, `<a class="result__a" href="https://reading.example/first">%s</a>`, subject)
							}
						case "reading.example":
							fmt.Fprintf(writer, "<title>%s</title><p>%s is recorded here.</p>", subject, subject)
						default:
							t.Errorf("unrequested HTTP destination: %s", request.Host)
							http.Error(writer, "unexpected destination", http.StatusBadRequest)
						}
					}))
					defer server.Close()
					endpoint, err := url.Parse(server.URL)
					if err != nil {
						t.Fatal(err)
					}
					transport := http.DefaultTransport.(*http.Transport).Clone()
					defer transport.CloseIdleConnections()
					acquisition, err := websearch.New(websearch.Config{
						Providers: []websearch.ProviderID{websearch.ProviderBrave, websearch.ProviderDuckDuckGo}, Timeout: 5 * time.Second,
						PerDocumentBytes: 4096, TotalDocumentBytes: 8192, MaxCandidatesPerProvider: 2,
						MaxCandidates: 2, MaxDocuments: 2, MaxResponseBytes: 8192,
						Resolver: acquisitionHTTPResolver{}, HTTPClient: &http.Client{Transport: acquisitionHTTPTransport{
							endpoint: endpoint, next: transport,
						}},
					})
					if err != nil {
						t.Fatal(err)
					}
					started := time.Now().UTC().Truncate(time.Microsecond)
					result, err := GatherRelevantEvidence(ctx, EvidenceRequest{ID: "http-observation", Question: subject, InitialQuery: subject},
						EvidenceConfig{MaxFetchCandidates: 2, MaxProjectionBytes: 1024, MaxRelevantCandidates: 2, CandidateSummaryBytes: 256}, acquisition,
						webUsageRelevanceFunc(func(RelevanceCall) (RelevanceDecision, error) {
							t.Fatal("failed HTTP acquisition invoked semantic relevance")
							return RelevanceDecision{}, nil
						}))
					wantError := websearch.ErrInvalidFetchedText
					switch failure {
					case "cancelled":
						wantError = context.Canceled
					case "parse_bounds":
						wantError = websearch.ErrBoundExceeded
					case "redirect":
						wantError = websearch.ErrDocumentRedirect
					}
					if !errors.Is(err, wantError) || len(result.Discovery) != 1 || result.SemanticCalls != 0 || len(result.Evidence) != 0 || len(result.Projected) != 0 {
						t.Fatalf("failed HTTP acquisition lost its error or accepted evidence: %+v / %v", result, err)
					}
					discovery := result.Discovery[0]
					if discovery.Query != subject || len(discovery.Candidates) != 2 || len(discovery.Diagnostics) != 2 || discovery.Candidates[0].Title != subject {
						t.Fatalf("actual discovery observations lost: %+v", discovery)
					}
					wantRequests := []string{"search.brave.com/search", "duckduckgo.com/html/"}
					if phase == "discovery" {
						if len(result.Fetches) != 0 || discovery.Diagnostics[1].Outcome != websearch.DiscoveryFailed || discovery.Diagnostics[1].Failure == "" {
							t.Fatalf("failed discovery fetched or omitted its diagnostic: %+v", result)
						}
					} else {
						wantRequests = append(wantRequests, "reading.example/first", "reading.example/second")
						if len(result.Fetches) != 1 || len(result.Fetches[0].Documents) != 1 || len(result.Fetches[0].Diagnostics) != 2 {
							t.Fatalf("actual fetch observations lost: %+v", result.Fetches)
						}
						fetch := result.Fetches[0]
						document := fetch.Documents[0]
						if document.URL != "https://reading.example/first" || !strings.Contains(document.Content, subject+" is recorded here.") ||
							document.ObservedAt.Before(started) || document.ObservedAt.After(time.Now().UTC()) ||
							fetch.Diagnostics[0].Outcome != websearch.FetchSucceeded || fetch.Diagnostics[1].Outcome != websearch.FetchFailed || fetch.Diagnostics[1].Failure == "" {
							t.Fatalf("observed source, time, or diagnostic differs: %+v", fetch)
						}
					}
					mutex.Lock()
					actualRequests := append([]string{}, requests...)
					mutex.Unlock()
					if !reflect.DeepEqual(actualRequests, wantRequests) {
						t.Fatalf("acquisition retried or changed its direct sequence: %v, want %v", actualRequests, wantRequests)
					}
				})
			}
		}
	}
}

type acquisitionHTTPResolver struct{}

func (acquisitionHTTPResolver) LookupIPAddr(context.Context, string) ([]net.IPAddr, error) {
	return []net.IPAddr{{IP: net.ParseIP("8.8.8.8")}}, nil
}

type acquisitionHTTPTransport struct {
	endpoint *url.URL
	next     http.RoundTripper
}

func (transport acquisitionHTTPTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	local := request.Clone(request.Context())
	local.Host = request.URL.Host
	local.URL.Scheme, local.URL.Host = transport.endpoint.Scheme, transport.endpoint.Host
	return transport.next.RoundTrip(local)
}
