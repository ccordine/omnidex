package api

import (
	"context"
	"fmt"
	"strings"
)

type hostBridgeStatusResponse struct {
	Configured   bool     `json:"configured"`
	Reachable    bool     `json:"reachable"`
	URL          string   `json:"url,omitempty"`
	Error        string   `json:"error,omitempty"`
	Message      string   `json:"message,omitempty"`
	NativePicker bool     `json:"native_picker,omitempty"`
	Service      string   `json:"service,omitempty"`
	PickerReady  bool     `json:"picker_ready"`
	Suggestions  []string `json:"suggestions,omitempty"`
}

func (s *Server) collectHostBridgeStatus(ctx context.Context) hostBridgeStatusResponse {
	url := strings.TrimSpace(s.hostAgentURL)
	if url == "" {
		return hostBridgeStatusResponse{
			Configured:  false,
			Reachable:   false,
			PickerReady: false,
			Message:     "HOST_AGENT_URL is not configured on core.",
			Suggestions: hostBridgeSuggestions(false, false, ""),
		}
	}

	client := s.hostBridgeClient()
	if client == nil {
		return hostBridgeStatusResponse{
			Configured:  false,
			Reachable:   false,
			URL:         url,
			PickerReady: false,
			Message:     "Host bridge client is unavailable.",
			Suggestions: hostBridgeSuggestions(false, false, url),
		}
	}

	payload, err := client.Health(ctx)
	if err != nil {
		return hostBridgeStatusResponse{
			Configured:  true,
			Reachable:   false,
			URL:         url,
			Error:       err.Error(),
			PickerReady: false,
			Message:     "Core cannot reach the host bridge.",
			Suggestions: hostBridgeSuggestions(true, false, url),
		}
	}

	nativePicker := mapBool(payload, "native_picker")
	return hostBridgeStatusResponse{
		Configured:   true,
		Reachable:    true,
		URL:          url,
		NativePicker: nativePicker,
		Service:      mapString(payload, "service"),
		PickerReady:  true,
		Message:      "Host bridge is reachable. Native folder picker and host filesystem browse are available.",
	}
}

func hostBridgeSuggestions(configured, reachable bool, url string) []string {
	if configured && reachable {
		return nil
	}
	out := []string{}
	if !configured {
		out = append(out,
			"On the host machine, start the bridge: omni host serve --listen 0.0.0.0:8091",
			"In core .env set HOST_AGENT_URL=http://host.docker.internal:8091 (or http://127.0.0.1:8091 when core runs on the host)",
			"Rebuild/restart core: docker compose up --build -d core",
			"Linux native picker requires zenity or kdialog installed on the host",
		)
		return out
	}
	out = append(out,
		"Verify the bridge is running on the host: curl http://127.0.0.1:8091/healthz",
		"If core runs in Docker, start the bridge with --listen 0.0.0.0:8091 (not 127.0.0.1 only)",
	)
	if url != "" {
		out = append(out, "Confirm HOST_AGENT_URL matches the bridge listen address (currently "+url+")")
	} else {
		out = append(out, "Confirm HOST_AGENT_URL matches the bridge listen address")
	}
	out = append(out,
		"If HOST_AGENT_TOKEN is set on core, pass the same token to omni host serve --token …",
		"From inside core: docker compose exec core wget -qO- --timeout=5 http://host.docker.internal:8091/healthz",
	)
	return out
}

func mapString(payload map[string]any, key string) string {
	raw, ok := payload[key]
	if !ok || raw == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(raw))
}

func mapBool(payload map[string]any, key string) bool {
	raw, ok := payload[key]
	if !ok || raw == nil {
		return false
	}
	switch value := raw.(type) {
	case bool:
		return value
	default:
		return strings.EqualFold(mapString(payload, key), "true")
	}
}
