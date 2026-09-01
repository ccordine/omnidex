package api

import (
	"fmt"
	"log"
	"net/http"
	"strings"
)

var uiStateQueryFields = []string{
	"panel", "admin_tab", "project_tab", "project_id", "scrum_card", "scrum_tab", "scrum_column",
	"data_source", "data_channel", "locale",
}

func validateUIStateQuery(r *http.Request) error {
	if err := validateExactQuery(r, uiStateQueryFields...); err != nil {
		return err
	}
	query := r.URL.Query()
	for _, key := range uiStateQueryFields {
		values, exists := query[key]
		if !exists {
			continue
		}
		if len(values) != 1 || values[0] == "" || values[0] != strings.TrimSpace(values[0]) {
			return fmt.Errorf("UI query field %q must be one non-empty canonical string", key)
		}
	}
	return nil
}

func mergeUIQueryState(state map[string]any, r *http.Request) error {
	if err := validateUIStateQuery(r); err != nil {
		return err
	}
	query := r.URL.Query()
	for _, key := range uiStateQueryFields {
		if key == "locale" {
			continue
		}
		if values, exists := query[key]; exists {
			state[key] = values[0]
		}
	}
	if values, exists := query["locale"]; exists {
		locale, err := parseUILocale(values[0])
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
