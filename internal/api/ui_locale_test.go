package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestRequestedUILocalesAreSupported(t *testing.T) {
	wants := map[uiLocale]bool{
		uiLocaleEnglish:           false,
		uiLocaleSpanish:           false,
		uiLocaleChineseSimplified: false,
		uiLocaleRussian:           false,
		uiLocaleJapanese:          false,
	}
	for _, option := range supportedUILocaleOptions {
		if _, ok := wants[option.Code]; ok {
			wants[option.Code] = true
		}
	}
	for locale, found := range wants {
		if !found {
			t.Errorf("required UI locale %q is not supported", locale)
		}
	}
}

func TestParseUILocaleNormalizesSupportedRegionalTags(t *testing.T) {
	tests := map[string]uiLocale{
		"en-US":   uiLocaleEnglish,
		"es-MX":   uiLocaleSpanish,
		"zh":      uiLocaleChineseSimplified,
		"zh-CN":   uiLocaleChineseSimplified,
		"zh_Hans": uiLocaleChineseSimplified,
		"ru-RU":   uiLocaleRussian,
		"ja-JP":   uiLocaleJapanese,
	}
	for input, want := range tests {
		t.Run(input, func(t *testing.T) {
			got, err := parseUILocale(input)
			if err != nil {
				t.Fatalf("parse locale: %v", err)
			}
			if got != want {
				t.Fatalf("locale=%q want %q", got, want)
			}
		})
	}
	if _, err := parseUILocale("fr-FR"); err == nil {
		t.Fatal("unsupported explicit locale must fail")
	}
}

func TestNegotiateUILocaleHonorsQualityAndDefaultsToEnglish(t *testing.T) {
	if got := negotiateUILocale("fr-FR;q=0.9, ja-JP;q=0.8, es;q=0.7"); got != uiLocaleJapanese {
		t.Fatalf("locale=%q want %q", got, uiLocaleJapanese)
	}
	if got := negotiateUILocale("zh-TW;q=0.5, ru-RU;q=0.9"); got != uiLocaleRussian {
		t.Fatalf("locale=%q want %q", got, uiLocaleRussian)
	}
	if got := negotiateUILocale("fr-FR, pt-BR;q=0.8"); got != uiLocaleEnglish {
		t.Fatalf("locale=%q want default %q", got, uiLocaleEnglish)
	}
}

func TestUILocaleCatalogsAreComplete(t *testing.T) {
	if err := validateUILocaleCatalogs(uiLocaleCatalogs); err != nil {
		t.Fatalf("catalog validation failed: %v", err)
	}
	for _, locale := range []uiLocale{uiLocaleSpanish, uiLocaleChineseSimplified, uiLocaleRussian, uiLocaleJapanese} {
		catalog := uiLocaleCatalogs[locale]
		if catalog["nav.newThread"] == uiLocaleCatalogs[uiLocaleEnglish]["nav.newThread"] {
			t.Errorf("locale %q did not translate nav.newThread", locale)
		}
	}
}

func TestUILocaleCatalogValidationRejectsMissingAndBlankMessages(t *testing.T) {
	catalogs := cloneUILocaleCatalogs(uiLocaleCatalogs)
	delete(catalogs[uiLocaleSpanish], "nav.newThread")
	if err := validateUILocaleCatalogs(catalogs); err == nil || !strings.Contains(err.Error(), "nav.newThread") {
		t.Fatalf("missing message error=%v", err)
	}

	catalogs = cloneUILocaleCatalogs(uiLocaleCatalogs)
	catalogs[uiLocaleJapanese]["nav.chat"] = "  "
	if err := validateUILocaleCatalogs(catalogs); err == nil || !strings.Contains(err.Error(), "nav.chat") {
		t.Fatalf("blank message error=%v", err)
	}
}

func TestUILocaleCatalogDecoderRejectsDuplicateKeys(t *testing.T) {
	_, err := decodeUIMessageCatalog([]byte(`{"duplicate":"one","duplicate":"two"}`))
	if err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("duplicate key error=%v", err)
	}
}

func TestEveryUIServerTemplateRendersInEverySupportedLocale(t *testing.T) {
	templates := map[string]string{}
	shell, err := os.ReadFile("web/index.html")
	if err != nil {
		t.Fatalf("read UI shell: %v", err)
	}
	templates["shell"] = string(shell)
	entries, err := uiPanelFS.ReadDir("web/panels")
	if err != nil {
		t.Fatalf("read UI panels: %v", err)
	}
	for _, entry := range entries {
		raw, err := uiPanelFS.ReadFile("web/panels/" + entry.Name())
		if err != nil {
			t.Fatalf("read panel %s: %v", entry.Name(), err)
		}
		templates[entry.Name()] = string(raw)
	}
	for _, option := range supportedUILocaleOptions {
		for name, source := range templates {
			t.Run(string(option.Code)+"/"+name, func(t *testing.T) {
				rendered, err := renderLocalizedHTML(source, option.Code)
				if err != nil {
					t.Fatalf("render: %v", err)
				}
				if strings.Contains(rendered, "data-i18n") || strings.Contains(rendered, "data-ui-locale-select") {
					t.Fatal("client-side localization marker remains after server rendering")
				}
			})
		}
	}
}

func TestInvalidPersistedUILocaleFailsLoudly(t *testing.T) {
	state := map[string]any{"locale": "fr"}
	if _, err := ensureUIStateLocale(state, nilRequest()); err == nil || !strings.Contains(err.Error(), "invalid persisted UI locale") {
		t.Fatalf("invalid persisted locale error=%v", err)
	}
}

func TestRenderLocalizedHTMLTranslatesTextAttributesAndDocumentLocale(t *testing.T) {
	source := `<html lang="en"><head><title data-i18n="app.pageTitle">Omni Chat</title></head><body><button data-i18n="nav.newThread" data-i18n-aria="nav.newThread" aria-label="New thread">New thread</button><textarea data-i18n-placeholder="panel.chat.placeholder" placeholder="Ask"></textarea><select data-ui-locale-select></select></body></html>`

	rendered, err := renderLocalizedHTML(source, uiLocaleSpanish)
	if err != nil {
		t.Fatalf("render localized HTML: %v", err)
	}
	for _, want := range []string{
		`<html lang="es" dir="ltr">`,
		`>Nuevo hilo</button>`,
		`aria-label="Nuevo hilo"`,
		`placeholder="Pide a Omni`,
		`<option value="es" selected>Español</option>`,
	} {
		if !strings.Contains(rendered, want) {
			t.Errorf("localized HTML missing %q: %s", want, rendered)
		}
	}
}

func TestRenderLocalizedHTMLRejectsUnknownMessageKey(t *testing.T) {
	_, err := renderLocalizedHTML(`<span data-i18n="missing.key">Fallback text</span>`, uiLocaleSpanish)
	if err == nil || !strings.Contains(err.Error(), "missing.key") {
		t.Fatalf("unknown message key error=%v", err)
	}
}

func cloneUILocaleCatalogs(source map[uiLocale]map[string]string) map[uiLocale]map[string]string {
	clone := make(map[uiLocale]map[string]string, len(source))
	for locale, messages := range source {
		clone[locale] = make(map[string]string, len(messages))
		for key, message := range messages {
			clone[locale][key] = message
		}
	}
	return clone
}

func nilRequest() *http.Request {
	return httptest.NewRequest(http.MethodGet, "/chat", nil)
}
