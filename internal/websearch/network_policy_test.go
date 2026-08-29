package websearch

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
)

func TestOutboundPolicyRejectsUnsafeLiteralAndResolvedDestinations(t *testing.T) {
	tests := []struct {
		name      string
		rawURL    string
		addresses []net.IPAddr
	}{
		{name: "ipv4 loopback", rawURL: "http://127.0.0.1/"},
		{name: "ipv6 loopback", rawURL: "http://[::1]/"},
		{name: "rfc1918", rawURL: "http://10.4.3.2/"},
		{name: "metadata", rawURL: "http://169.254.169.254/latest/meta-data/"},
		{name: "unspecified", rawURL: "http://0.0.0.0/"},
		{name: "multicast", rawURL: "http://224.0.0.1/"},
		{name: "ipv6 link local", rawURL: "http://[fe80::1]/"},
		{name: "ipv6 private", rawURL: "http://[fd00::1]/"},
		{name: "carrier grade NAT", rawURL: "http://100.64.0.1/"},
		{name: "benchmark network", rawURL: "http://198.18.0.1/"},
		{name: "documentation IPv4", rawURL: "http://192.0.2.1/"},
		{name: "reserved IPv4", rawURL: "http://240.0.0.1/"},
		{name: "documentation IPv6", rawURL: "http://[2001:db8::1]/"},
		{name: "IPv6 6to4", rawURL: "http://[2002:7f00:1::]/"},
		{name: "IPv6 NAT64", rawURL: "http://[64:ff9b::c0a8:101]/"},
		{name: "resolved private", rawURL: "https://private.example/", addresses: []net.IPAddr{{IP: net.ParseIP("192.168.1.4")}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			resolver := fixtureResolver{addresses: map[string][]net.IPAddr{"private.example": test.addresses}}
			policy := newOutboundPolicy(resolver, validConfig(&fixtureTransport{}, ProviderDuckDuckGo).Timeout)
			if err := policy.validateURL(context.Background(), test.rawURL); !errors.Is(err, ErrUnsafeURL) {
				t.Fatalf("validateURL error=%v want ErrUnsafeURL", err)
			}
		})
	}
}

func TestDialRevalidatesResolutionAfterURLAdmission(t *testing.T) {
	resolver := &sequenceResolver{answers: [][]net.IPAddr{
		{{IP: net.ParseIP("93.184.216.34")}},
		{{IP: net.ParseIP("127.0.0.1")}},
	}}
	policy := newOutboundPolicy(resolver, validConfig(&fixtureTransport{}, ProviderDuckDuckGo).Timeout)
	if err := policy.validateURL(context.Background(), "https://rebind.example/"); err != nil {
		t.Fatalf("initial URL admission error=%v", err)
	}
	if _, err := policy.dialContext(context.Background(), "tcp", "rebind.example:443"); !errors.Is(err, ErrUnsafeURL) {
		t.Fatalf("dial error=%v want rebound private address rejection", err)
	}
}

func TestHTTPClientRejectsPublicToPrivateRedirectBeforeSecondRequest(t *testing.T) {
	transport := &fixtureTransport{responses: map[string]fixtureResponse{
		"https://public.example/start": {status: http.StatusFound, body: "redirect"},
	}}
	transportResponse := policyRedirectTransport{fixtureTransport: transport, location: "http://metadata.example/latest"}
	config := validConfig(transportResponse, ProviderDuckDuckGo)
	config.Resolver = fixtureResolver{addresses: map[string][]net.IPAddr{
		"public.example":   {{IP: net.ParseIP("93.184.216.34")}},
		"metadata.example": {{IP: net.ParseIP("169.254.169.254")}},
	}}
	service, err := New(config)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.get(context.Background(), "https://public.example/start"); !errors.Is(err, ErrUnsafeURL) {
		t.Fatalf("redirect error=%v want ErrUnsafeURL", err)
	}
	if len(transport.requests) != 1 {
		t.Fatalf("requests=%v; private redirect was executed", transport.requests)
	}
}

func TestInjectedTransportStillRequiresPublicResolution(t *testing.T) {
	transport := &fixtureTransport{responses: map[string]fixtureResponse{
		"https://fixture.public/document": {status: http.StatusOK, body: "public evidence"},
	}}
	config := validConfig(transport, ProviderDuckDuckGo)
	config.Resolver = fixtureResolver{addresses: map[string][]net.IPAddr{
		"fixture.public": {{IP: net.ParseIP("93.184.216.34")}},
	}}
	service, err := New(config)
	if err != nil {
		t.Fatal(err)
	}
	body, err := service.get(context.Background(), "https://fixture.public/document")
	if err != nil || body != "public evidence" {
		t.Fatalf("body=%q error=%v", body, err)
	}
}

func TestSearchRedirectIsRejectedBeforeSecondRequest(t *testing.T) {
	transport := &loopingRedirectTransport{}
	config := validConfig(transport, ProviderDuckDuckGo)
	config.Resolver = fixtureResolver{}
	service, err := New(config)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.get(context.Background(), "https://redirect.example/0"); !errors.Is(err, ErrSearchRedirect) {
		t.Fatalf("redirect error=%v want ErrSearchRedirect", err)
	}
	if transport.requests != 1 {
		t.Fatalf("requests=%d want 1", transport.requests)
	}
}

func TestPublicBoundariesRejectNilAndCanceledContexts(t *testing.T) {
	service, err := New(validConfig(&fixtureTransport{}, ProviderDuckDuckGo))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Discover(nil, QueryRequest{Query: "query"}); !errors.Is(err, ErrNilContext) {
		t.Fatalf("Discover nil context error=%v", err)
	}
	if _, err := service.Fetch(nil, FetchRequest{}); !errors.Is(err, ErrNilContext) {
		t.Fatalf("Fetch nil context error=%v", err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := service.Discover(canceled, QueryRequest{Query: "query"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("Discover canceled context error=%v", err)
	}
	if _, err := service.Fetch(canceled, FetchRequest{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("Fetch canceled context error=%v", err)
	}
}

func TestCancellationAfterHTTPReadReturnsNoPartialAuthority(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	transport := cancelAfterReadTransport{cancel: cancel, body: `<a class="result__a" href="https://docs.example/current">Current</a>`}
	service, err := New(validConfig(transport, ProviderDuckDuckGo))
	if err != nil {
		t.Fatal(err)
	}
	report, err := service.Discover(ctx, QueryRequest{Query: "current"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Discover error=%v want cancellation", err)
	}
	if len(report.Candidates) != 0 || len(report.Diagnostics) != 0 {
		t.Fatalf("canceled discovery leaked partial authority: %#v", report)
	}
}

type policyRedirectTransport struct {
	fixtureTransport *fixtureTransport
	location         string
}

type sequenceResolver struct {
	answers [][]net.IPAddr
	calls   int
}

type loopingRedirectTransport struct{ requests int }

type cancelAfterReadTransport struct {
	cancel context.CancelFunc
	body   string
}

type cancelAfterReadBody struct {
	reader *strings.Reader
	cancel context.CancelFunc
	done   bool
}

func (transport cancelAfterReadTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: http.StatusOK,
		Body: &cancelAfterReadBody{
			reader: strings.NewReader(transport.body), cancel: transport.cancel,
		},
		Header: make(http.Header), Request: request,
	}, nil
}

func (body *cancelAfterReadBody) Read(target []byte) (int, error) {
	count, err := body.reader.Read(target)
	if !body.done {
		body.done = true
		body.cancel()
	}
	return count, err
}

func (*cancelAfterReadBody) Close() error { return nil }

var _ io.ReadCloser = (*cancelAfterReadBody)(nil)

func (transport *loopingRedirectTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	transport.requests++
	return &http.Response{
		StatusCode: http.StatusFound,
		Body:       http.NoBody,
		Header:     http.Header{"Location": []string{fmt.Sprintf("https://redirect.example/%d", transport.requests)}},
		Request:    request,
	}, nil
}

func (resolver *sequenceResolver) LookupIPAddr(context.Context, string) ([]net.IPAddr, error) {
	if resolver.calls >= len(resolver.answers) {
		return nil, errors.New("resolver answers exhausted")
	}
	answer := append([]net.IPAddr(nil), resolver.answers[resolver.calls]...)
	resolver.calls++
	return answer, nil
}

func (transport policyRedirectTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	response, err := transport.fixtureTransport.RoundTrip(request)
	if err == nil && response.StatusCode >= 300 && response.StatusCode < 400 {
		response.Header.Set("Location", transport.location)
	}
	return response, err
}
