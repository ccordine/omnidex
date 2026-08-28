package api

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/hostbridge"
)

type hostBridgeStatusResponse struct {
	Configured   bool              `json:"configured"`
	Reachable    bool              `json:"reachable"`
	URL          string            `json:"url,omitempty"`
	Error        string            `json:"error,omitempty"`
	Message      string            `json:"message,omitempty"`
	NativePicker bool              `json:"native_picker,omitempty"`
	Service      string            `json:"service,omitempty"`
	Terminal     bool              `json:"terminal,omitempty"`
	Screen       bool              `json:"screen,omitempty"`
	PickerReady  bool              `json:"picker_ready"`
	Suggestions  []string          `json:"suggestions,omitempty"`
	HTML         chatComponentHTML `json:"html"`
}

func (s *Server) collectHostBridgeStatus(ctx context.Context) hostBridgeStatusResponse {
	return s.collectHostBridgeStatusWithTimeout(ctx, 4*time.Second)
}

func (s *Server) collectHostBridgeStatusWithTimeout(ctx context.Context, timeout time.Duration) hostBridgeStatusResponse {
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

	if timeout <= 0 {
		timeout = 4 * time.Second
	}
	client := hostbridge.NewClient(url, s.hostAgentToken, timeout)
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

	resolvedURL, resolveErr := hostbridge.ResolveReachableURL(ctx, url, s.hostAgentToken, timeout)
	if resolveErr == nil && resolvedURL != "" {
		url = resolvedURL
		client = hostbridge.NewClient(resolvedURL, s.hostAgentToken, timeout)
	}

	payload, err := client.Health(ctx)
	if err != nil {
		suggestions := hostBridgeSuggestions(true, false, url)
		if hint := hostbridge.LinuxDockerFirewallHint(err); hint != "" {
			suggestions = append(suggestions, hint)
		}
		return hostBridgeStatusResponse{
			Configured:  true,
			Reachable:   false,
			URL:         url,
			Error:       err.Error(),
			PickerReady: false,
			Message:     "Core cannot reach the host bridge.",
			Suggestions: suggestions,
		}
	}

	nativePicker := mapBool(payload, "native_picker")
	terminalReady := mapBool(payload, "terminal")
	screenReady := mapBool(payload, "screen")
	return hostBridgeStatusResponse{
		Configured:   true,
		Reachable:    true,
		URL:          url,
		NativePicker: nativePicker,
		Terminal:     terminalReady,
		Screen:       screenReady,
		Service:      mapString(payload, "service"),
		PickerReady:  true,
		Message:      hostBridgeReadyMessage(nativePicker, terminalReady, screenReady),
	}
}

func hostBridgeReadyMessage(nativePicker, terminalReady, screenReady bool) string {
	switch {
	case nativePicker && terminalReady && screenReady:
		return "Host bridge is reachable. Filesystem browse, terminal, and screen streaming are available."
	case terminalReady && screenReady:
		return "Host bridge is reachable. Terminal and screen streaming are available."
	case nativePicker && terminalReady:
		return "Host bridge is reachable. Filesystem browse and in-browser terminal are available."
	case terminalReady:
		return "Host bridge is reachable. In-app filesystem browse and terminal are available."
	default:
		return "Host bridge is reachable. In-app filesystem browse is available."
	}
}

func hostBridgeSuggestions(configured, reachable bool, url string) []string {
	if configured && reachable {
		return nil
	}
	out := []string{}
	if !configured {
		out = append(out,
			"On the host machine, install the bridge as a service: omni host service install",
			"Or run manually: omni host serve --listen 0.0.0.0:8091",
			"In core .env set HOST_AGENT_URL=http://host.docker.internal:8091 (or http://127.0.0.1:8091 when core runs on the host)",
			"Rebuild/restart core through its managed deployment identity: omni service --service core up --build",
		)
		return out
	}
	out = append(out,
		"Verify the bridge is running: omni host service status (or curl http://127.0.0.1:8091/healthz)",
		"If not installed as a service: omni host service install",
		"If core runs in Docker, start the bridge with --listen 0.0.0.0:8091 (not 127.0.0.1 only)",
	)
	if url != "" {
		out = append(out, "Confirm HOST_AGENT_URL matches the bridge listen address (currently "+url+")")
	} else {
		out = append(out, "Confirm HOST_AGENT_URL matches the bridge listen address")
	}
	out = append(out,
		"If HOST_AGENT_TOKEN is set on core, pass the same token to omni host serve --token …",
		"For a manual container probe, use Docker's default rootful context and the exact COMPOSE_PROJECT_NAME recorded in the Omnidex install .env",
		"Arch Linux + UFW: if probes time out (not refused), run scripts/ufw-docker-host.sh on the host",
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
