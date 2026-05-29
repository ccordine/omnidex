package api

import (
	"net/http/httptest"
	"testing"
)

func TestNetworkSettingsGet(t *testing.T) {
	s := NewServerWithOptions(nil, nil, ServerOptions{
		CoreURL:    "http://192.168.1.102:8090",
		ListenAddr: "0.0.0.0:8090",
	})
	req := httptest.NewRequest("GET", "/v1/settings/network", nil)
	rec := httptest.NewRecorder()
	s.handleNetworkSettings(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}
