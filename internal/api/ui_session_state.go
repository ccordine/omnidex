package api

import (
	"fmt"
	"log"
	"net/http"
	"strings"
)

func mergeUIQueryState(state map[string]any, r *http.Request) error {
	for _, key := range []string{"panel", "admin_tab", "project_tab", "project_id", "scrum_card", "scrum_tab", "scrum_column", "data_source", "data_channel"} {
		value := strings.TrimSpace(r.URL.Query().Get(key))
		if value != "" {
			state[key] = value
		}
	}
	if r.URL.Query().Has("locale") {
		locale, err := parseUILocale(r.URL.Query().Get("locale"))
		if err != nil {
			return err
		}
		state["locale"] = string(locale)
	}
	return nil
}

func applyUIStatePatch(state, patch map[string]any) error {
	clean := sanitizeUIState(patch)
	if raw, exists := patch["locale"]; exists {
		value, ok := raw.(string)
		if !ok {
			return fmt.Errorf("UI locale must be a string")
		}
		locale, err := parseUILocale(value)
		if err != nil {
			return err
		}
		clean["locale"] = string(locale)
	}
	for key, value := range clean {
		state[key] = value
	}
	return nil
}

func ensureUIStateLocale(state map[string]any, r *http.Request) (uiLocale, error) {
	raw, exists := state["locale"]
	if !exists {
		locale := negotiateUILocale(r.Header.Get("Accept-Language"))
		state["locale"] = string(locale)
		return locale, nil
	}
	value, ok := raw.(string)
	if !ok {
		return "", fmt.Errorf("persisted UI locale must be a string, received %T", raw)
	}
	locale, err := parseUILocale(value)
	if err != nil {
		return "", fmt.Errorf("invalid persisted UI locale: %w", err)
	}
	state["locale"] = string(locale)
	return locale, nil
}

func logUILocaleRejection(sessionID, endpoint string, err error) {
	log.Printf("UI locale rejected session=%q endpoint=%q: %v", sessionID, endpoint, err)
}

func logUILocaleTransition(sessionID, previous string, locale uiLocale, source string) {
	if previous == string(locale) {
		return
	}
	log.Printf("UI locale updated session=%q from=%q to=%q source=%q", sessionID, previous, locale, source)
}

func setUILocaleResponseHeaders(w http.ResponseWriter, locale uiLocale) {
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("Content-Language", string(locale))
	w.Header().Add("Vary", "Accept-Language")
	w.Header().Add("Vary", "Cookie")
}
