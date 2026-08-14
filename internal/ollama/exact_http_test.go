package ollama

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httptrace"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/llm"
)

func TestExactProviderRequestHonorsConfiguredDelayedHeaderTimeout(t *testing.T) {
	const responseDelay = 220 * time.Millisecond
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(responseDelay)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	request := func(t *testing.T, timeout time.Duration) (*http.Response, llm.ProviderRequestDisposition, error) {
		t.Helper()
		client := New(server.URL, "", "", timeout, llm.DefaultInferenceContextTokens)
		req, err := http.NewRequest(http.MethodPost, server.URL+"/api/generate", strings.NewReader("{}"))
		if err != nil {
			t.Fatal(err)
		}
		return client.doExactProviderRequest(req)
	}

	if response, disposition, err := request(t, 750*time.Millisecond); err != nil {
		t.Fatalf("larger configured timeout rejected delayed headers: %v", err)
	} else {
		defer response.Body.Close()
		if disposition != llm.ProviderRequestDispatched || response.StatusCode != http.StatusNoContent {
			t.Fatalf("response status=%d disposition=%q", response.StatusCode, disposition)
		}
	}

	response, disposition, err := request(t, 50*time.Millisecond)
	if err == nil || response != nil {
		t.Fatalf("short explicit timeout returned response=%v error=%v", response, err)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("short explicit timeout error=%v want context deadline exceeded", err)
	}
	if disposition != llm.ProviderRequestDispatched {
		t.Fatalf("short timeout disposition=%q want dispatched", disposition)
	}
}

func TestExactProviderRequestRecordsTruthfulWriteDisposition(t *testing.T) {
	t.Parallel()
	transportFailure := errors.New("transport failed")
	writeFailure := errors.New("request body write failed")
	for _, testCase := range []struct {
		name      string
		transport http.RoundTripper
		want      llm.ProviderRequestDisposition
	}{
		{
			name: "failure before any write",
			transport: exactRoundTripFunc(func(*http.Request) (*http.Response, error) {
				return nil, transportFailure
			}),
			want: llm.ProviderRequestNotDispatched,
		},
		{
			name: "full write before response loss",
			transport: exactRoundTripFunc(func(request *http.Request) (*http.Response, error) {
				notifyExactRequestWrite(request, nil)
				return nil, transportFailure
			}),
			want: llm.ProviderRequestDispatched,
		},
		{
			name: "partial write",
			transport: exactRoundTripFunc(func(request *http.Request) (*http.Response, error) {
				notifyExactRequestWrite(request, writeFailure)
				return nil, transportFailure
			}),
			want: llm.ProviderRequestWriteIndeterminate,
		},
		{
			name: "response proves dispatch",
			transport: exactRoundTripFunc(func(request *http.Request) (*http.Response, error) {
				notifyExactRequestWrite(request, writeFailure)
				return &http.Response{
					StatusCode: http.StatusBadRequest,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader(`{"error":"bad request"}`)),
					Request:    request,
				}, nil
			}),
			want: llm.ProviderRequestDispatched,
		},
	} {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			client := &Client{httpClient: &http.Client{Transport: testCase.transport}}
			request, err := http.NewRequest(http.MethodPost, "http://provider.invalid/api/generate", strings.NewReader("{}"))
			if err != nil {
				t.Fatal(err)
			}
			response, disposition, requestErr := client.doExactProviderRequest(request)
			if disposition != testCase.want {
				t.Fatalf("request disposition=%q want %q (response=%v error=%v)", disposition, testCase.want, response, requestErr)
			}
			if testCase.name == "response proves dispatch" {
				if requestErr != nil || response == nil {
					t.Fatalf("response path returned response=%v error=%v", response, requestErr)
				}
				response.Body.Close()
			} else if !errors.Is(requestErr, transportFailure) || response != nil {
				t.Fatalf("failure path returned response=%v error=%v", response, requestErr)
			}
		})
	}
}

func TestExactProviderRequestUsesRealTransportWriteEvidence(t *testing.T) {
	t.Run("dial failure is not dispatched", func(t *testing.T) {
		dialFailure := errors.New("dial failed before write")
		transport := &http.Transport{DialContext: func(
			context.Context, string, string,
		) (net.Conn, error) {
			return nil, dialFailure
		}}
		client := &Client{httpClient: &http.Client{Transport: transport}}
		request, err := http.NewRequest(http.MethodPost, "http://provider.invalid/generate", strings.NewReader("{}"))
		if err != nil {
			t.Fatal(err)
		}
		_, disposition, err := client.doExactProviderRequest(request)
		if !errors.Is(err, dialFailure) || disposition != llm.ProviderRequestNotDispatched {
			t.Fatalf("dial error=%v disposition=%q", err, disposition)
		}
	})

	t.Run("request body failure is write indeterminate", func(t *testing.T) {
		client, endpoint := exactScriptedTransport(t, func(request *http.Request) {
			_, _ = io.Copy(io.Discard, request.Body)
		})
		bodyFailure := errors.New("body source failed after prefix")
		request, err := http.NewRequest(
			http.MethodPost, endpoint, &exactFailingBody{prefix: []byte("partial"), err: bodyFailure},
		)
		if err != nil {
			t.Fatal(err)
		}
		request.ContentLength = 1024
		_, disposition, err := client.doExactProviderRequest(request)
		if err == nil || disposition != llm.ProviderRequestWriteIndeterminate {
			t.Fatalf("body error=%v disposition=%q", err, disposition)
		}
	})

	t.Run("complete write followed by response loss is dispatched", func(t *testing.T) {
		client, endpoint := exactScriptedTransport(t, func(request *http.Request) {
			_, _ = io.Copy(io.Discard, request.Body)
		})
		request, err := http.NewRequest(http.MethodPost, endpoint, strings.NewReader("complete-body"))
		if err != nil {
			t.Fatal(err)
		}
		_, disposition, err := client.doExactProviderRequest(request)
		if err == nil || disposition != llm.ProviderRequestDispatched {
			t.Fatalf("response loss error=%v disposition=%q", err, disposition)
		}
	})
}

type exactFailingBody struct {
	prefix []byte
	err    error
}

func (body *exactFailingBody) Read(destination []byte) (int, error) {
	if len(body.prefix) == 0 {
		return 0, body.err
	}
	count := copy(destination, body.prefix)
	body.prefix = body.prefix[count:]
	return count, nil
}

func (*exactFailingBody) Close() error { return nil }

func exactScriptedTransport(
	t *testing.T,
	serve func(*http.Request),
) (*Client, string) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	done := make(chan struct{})
	go func() {
		defer close(done)
		connection, acceptErr := listener.Accept()
		if acceptErr != nil {
			return
		}
		defer connection.Close()
		_ = connection.SetDeadline(time.Now().Add(5 * time.Second))
		request, readErr := http.ReadRequest(bufio.NewReader(connection))
		if readErr == nil {
			serve(request)
			_ = request.Body.Close()
		}
	}()
	t.Cleanup(func() {
		_ = listener.Close()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("scripted exact transport did not stop")
		}
	})
	transport := &http.Transport{
		DisableKeepAlives:      true,
		MaxResponseHeaderBytes: maxExactProviderResponseHeaderBytes,
	}
	return &Client{httpClient: &http.Client{Transport: transport}},
		"http://" + listener.Addr().String() + "/generate"
}

func notifyExactRequestWrite(request *http.Request, err error) {
	trace := httptrace.ContextClientTrace(request.Context())
	if trace == nil || trace.WroteRequest == nil {
		panic("exact request is missing its write trace")
	}
	trace.WroteRequest(httptrace.WroteRequestInfo{Err: err})
}

type exactRoundTripFunc func(*http.Request) (*http.Response, error)

func (function exactRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}
