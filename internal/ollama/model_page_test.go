package ollama

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestListModelPageStreamsExactBoundedWindow(t *testing.T) {
	client := New("http://ollama.invalid", "", "", 0, 0)
	client.httpClient.Transport = roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Path != "/api/tags" {
			t.Fatalf("path=%q", request.URL.Path)
		}
		return jsonHTTPResponse(http.StatusOK, `{"models":[{"name":"model-0","size":0},{"name":"model-1","size":1},{"name":"model-2","size":2},{"name":"model-3","size":3},{"name":"model-4","size":4}]}`), nil
	})

	page, err := client.ListModelPage(context.Background(), 2, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Models) != 2 || !page.HasMore || page.Offset != 2 ||
		page.Models[0].Name != "model-2" || page.Models[1].Name != "model-3" {
		t.Fatalf("page=%+v", page)
	}
}

func TestListModelPageRejectsInvalidBoundsAndMalformedCatalog(t *testing.T) {
	client := New("http://127.0.0.1:1", "", "", 0, 0)
	if _, err := client.ListModelPage(context.Background(), 0, 0); err == nil {
		t.Fatal("expected zero limit to fail")
	}
	if _, err := client.ListModelPage(context.Background(), 1, -1); err == nil {
		t.Fatal("expected negative offset to fail")
	}
	malformed := New("http://ollama.invalid", "", "", 0, 0)
	malformed.httpClient.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		return jsonHTTPResponse(http.StatusOK, `{"unexpected":[]}`), nil
	})
	if _, err := malformed.ListModelPage(context.Background(), 1, 0); err == nil {
		t.Fatal("expected missing models field to fail")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return fn(request) }

func jsonHTTPResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)),
	}
}
