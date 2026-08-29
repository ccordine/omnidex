package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestUIShellIsServerLocalizedFromExplicitLocale(t *testing.T) {
	server := NewServer(nil, &fakeLLMClient{})
	req := httptest.NewRequest(http.MethodGet, "/chat?locale=es-MX", nil)
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{`<html lang="es" dir="ltr"`, "Nuevo hilo", "Proyecto en vista", `<option value="es" selected>Español</option>`} {
		if !strings.Contains(body, want) {
			t.Errorf("Spanish shell missing %q", want)
		}
	}
	if strings.Contains(body, ">New thread<") {
		t.Fatal("server returned an English fallback for a translated shell message")
	}
	if got := rec.Header().Get("Cache-Control"); got != "private, no-store" {
		t.Fatalf("Cache-Control=%q", got)
	}
}

func TestUIShellNegotiatesJapaneseFromAcceptLanguage(t *testing.T) {
	server := NewServer(nil, &fakeLLMClient{})
	req := httptest.NewRequest(http.MethodGet, "/chat", nil)
	req.Header.Set("Accept-Language", "fr-FR;q=0.9, ja-JP;q=0.8")
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if body := rec.Body.String(); !strings.Contains(body, `<html lang="ja" dir="ltr"`) || !strings.Contains(body, "新しいスレッド") {
		t.Fatalf("Japanese shell was not negotiated: %s", body)
	}
}

func TestUIPanelReturnsServerLocalizedChineseFragment(t *testing.T) {
	server := NewServer(nil, &fakeLLMClient{})
	req := httptest.NewRequest(http.MethodGet, "/v1/ui/panel?panel=data&locale=zh-CN", nil)
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var payload uiPanelResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode panel: %v", err)
	}
	if payload.Locale != uiLocaleChineseSimplified {
		t.Fatalf("locale=%q", payload.Locale)
	}
	if got := rec.Header().Get("Content-Language"); got != string(uiLocaleChineseSimplified) {
		t.Fatalf("Content-Language=%q", got)
	}
	for _, want := range []string{"数据库分析", "数据", "正在加载数据"} {
		if !strings.Contains(payload.HTML.Bundle, want) {
			t.Errorf("Chinese data panel missing %q: %s", want, payload.HTML.Bundle)
		}
	}
}

func TestUIEndpointsRejectUnsupportedExplicitLocale(t *testing.T) {
	server := NewServer(nil, &fakeLLMClient{})
	for _, target := range []string{"/chat?locale=fr", "/v1/ui/panel?panel=chat&locale=fr", "/v1/ui/session?locale=fr"} {
		t.Run(target, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, target, nil)
			rec := httptest.NewRecorder()
			server.Handler().ServeHTTP(rec, req)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestUnlocalizedStaticShellRouteIsRemoved(t *testing.T) {
	server := NewServer(nil, &fakeLLMClient{})
	for _, target := range []string{"/ui/", "/ui/index.html"} {
		req := httptest.NewRequest(http.MethodGet, target, nil)
		rec := httptest.NewRecorder()
		server.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("target=%s status=%d body=%s", target, rec.Code, rec.Body.String())
		}
	}
}

func TestUISessionValidatesAndCanonicalizesLocalePatch(t *testing.T) {
	server := NewServer(nil, &fakeLLMClient{})
	req := httptest.NewRequest(http.MethodPatch, "/v1/ui/session", strings.NewReader(`{"state":{"locale":"ru-RU"}}`))
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var payload uiSessionResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode session: %v", err)
	}
	if payload.Locale != uiLocaleRussian || payload.State["locale"] != string(uiLocaleRussian) {
		t.Fatalf("unexpected localized state: %#v", payload)
	}

	req = httptest.NewRequest(http.MethodPatch, "/v1/ui/session", strings.NewReader(`{"state":{"locale":"klingon"}}`))
	rec = httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid locale status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestFrontendLocalizationHasNoLocalStorageOrEnglishFallback(t *testing.T) {
	source := string(mustReadFrontendTestFile(t, "web/src/lib/i18n/index.ts"))
	for _, forbidden := range []string{"local" + "Storage", "?? catalogs.en", "?? key", "apply" + "I18n"} {
		if strings.Contains(source, forbidden) {
			t.Errorf("forbidden client localization fallback %q remains", forbidden)
		}
	}
	shell := string(mustReadFrontendTestFile(t, "web/src/controllers/shell_controller.ts"))
	for _, forbidden := range []string{"localeSelectTarget.innerHTML", "omni:locale-changed", "apply" + "DocumentLocale"} {
		if strings.Contains(shell, forbidden) {
			t.Errorf("forbidden client locale rendering path %q remains", forbidden)
		}
	}
}

func mustReadFrontendTestFile(t *testing.T, path string) []byte {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return raw
}
