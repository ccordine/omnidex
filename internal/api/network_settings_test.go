package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestNetworkSettingsRequestAcceptsOnlyCanonicalHostAndPort(t *testing.T) {
	request := httptest.NewRequest(http.MethodPut, "/v1/settings/network", strings.NewReader(`{"host":"api.worknet","port":8090}`))
	response := httptest.NewRecorder()
	decoded, err := decodeNetworkSettingsRequest(response, request)
	if err != nil || decoded.Host != "api.worknet" || decoded.Port != 8090 {
		t.Fatalf("decoded=%+v err=%v", decoded, err)
	}
	for _, body := range [][]byte{
		[]byte(`{"host":"one","host":"two","port":8090}`),
		[]byte(`{"host":"one","port":8090,"url":"http://override"}`),
		[]byte(`{"Host":"one","port":8090}`),
		[]byte(`{"host":"one","port":8090} {}`),
		[]byte(`{"host":" http://one ","port":8090}`),
		[]byte(`{"host":"one","port":0}`),
		bytes.Repeat([]byte(" "), int(maxNetworkSettingsBodyBytes+1)),
	} {
		request := httptest.NewRequest(http.MethodPut, "/v1/settings/network", bytes.NewReader(body))
		response := httptest.NewRecorder()
		if _, err := decodeNetworkSettingsRequest(response, request); err == nil {
			t.Fatalf("invalid network settings body accepted: %q", body)
		}
	}
}
