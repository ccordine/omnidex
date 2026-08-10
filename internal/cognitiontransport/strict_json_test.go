package cognitiontransport

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestServerRejectsDuplicateAndInexactWireFields(t *testing.T) {
	world := newTransportWorld(t)
	handler, err := NewHandler(world.environment, world.environment, mustAuthenticator(t, "secret-token"))
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string]string{
		"duplicate top-level": `{"protocol":"` + ProtocolVersionV1 + `","protocol":"` + ProtocolVersionV1 + `","scenario":` + mustJSON(t, world.scenario) + `}`,
		"inexact top-level":   `{"Protocol":"` + ProtocolVersionV1 + `","scenario":` + mustJSON(t, world.scenario) + `}`,
		"inexact nested":      `{"protocol":"` + ProtocolVersionV1 + `","scenario":{"ID":"` + string(world.scenario.ID) + `","sha256":"` + world.scenario.SHA256 + `"}}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, startPath, strings.NewReader(body))
			request.Header.Set("Authorization", "Bearer secret-token")
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, request)
			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
			}
		})
	}
	if world.environment.startCalls != 0 {
		t.Fatalf("invalid requests reached environment %d times", world.environment.startCalls)
	}
}

func TestClientRejectsDuplicateAndInexactWireFields(t *testing.T) {
	world := newTransportWorld(t)
	cases := map[string]string{
		"duplicate result": `{"protocol":"` + ProtocolVersionV1 + `","transition":` + mustJSON(t, world.start) + `,"transition":` + mustJSON(t, world.start) + `}`,
		"inexact result":   `{"Protocol":"` + ProtocolVersionV1 + `","transition":` + mustJSON(t, world.start) + `}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			client, err := NewClient("http://environment.invalid", "secret-token", &http.Client{
				Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       io.NopCloser(strings.NewReader(body)),
						Header:     make(http.Header),
					}, nil
				}),
			})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := client.Start(context.Background(), world.scenario); !errors.Is(err, ErrInvalidWire) {
				t.Fatalf("error=%v, want ErrInvalidWire", err)
			}
		})
	}
}
