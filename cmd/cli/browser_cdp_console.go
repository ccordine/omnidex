package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"
)

func discoverBrowserEndpoints(ports []int) []browserEndpoint {
	endpoints := make([]browserEndpoint, 0, len(ports))
	for _, port := range ports {
		endpoint, ok := fetchBrowserEndpoint(port)
		if !ok {
			continue
		}
		endpoints = append(endpoints, endpoint)
	}
	sort.Slice(endpoints, func(i, j int) bool {
		return endpoints[i].Port < endpoints[j].Port
	})
	return endpoints
}

func fetchBrowserEndpoint(port int) (browserEndpoint, bool) {
	base := fmt.Sprintf("http://127.0.0.1:%d", port)
	var version browserVersion
	if err := browserHTTPGetJSON(base+"/json/version", &version); err != nil {
		return browserEndpoint{}, false
	}

	targets := make([]browserTarget, 0, 8)
	_ = browserHTTPGetJSON(base+"/json/list", &targets)
	return browserEndpoint{
		Port:    port,
		Version: version,
		Targets: targets,
	}, true
}

func browserHTTPGetJSON(endpoint string, out any) error {
	ctx, cancel := context.WithTimeout(context.Background(), browserProbeTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: browserProbeTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("status=%d", resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func collectConsoleEvents(endpoints []browserEndpoint, duration time.Duration, limit int) []browserConsoleEntry {
	if duration <= 0 {
		duration = 2 * time.Second
	}
	if limit <= 0 {
		limit = 50
	}

	events := make([]browserConsoleEntry, 0, limit)
	for _, endpoint := range endpoints {
		for _, target := range endpoint.Targets {
			if len(events) >= limit {
				return events
			}
			if strings.ToLower(strings.TrimSpace(target.Type)) != "page" {
				continue
			}
			wsURL := strings.TrimSpace(target.WebSocketDebuggerURL)
			if wsURL == "" {
				continue
			}
			budget := limit - len(events)
			captured := cdpCaptureConsole(wsURL, target.Title, target.URL, duration, budget)
			events = append(events, captured...)
		}
	}
	return events
}

func cdpCaptureConsole(wsURL, tabTitle, tabURL string, duration time.Duration, limit int) []browserConsoleEntry {
	conn, err := cdpDialWebSocket(wsURL)
	if err != nil {
		return nil
	}
	defer conn.Close()

	_ = conn.SendJSON(map[string]any{"id": 1, "method": "Log.enable"})
	_ = conn.SendJSON(map[string]any{"id": 2, "method": "Runtime.enable"})

	deadline := time.Now().Add(duration)
	out := make([]browserConsoleEntry, 0, limit)
	for len(out) < limit {
		if time.Now().After(deadline) {
			break
		}
		msg, err := conn.ReadJSONUntil(deadline)
		if err != nil {
			break
		}
		entry, ok := cdpEventToConsoleEntry(msg, tabTitle, tabURL)
		if !ok {
			continue
		}
		out = append(out, entry)
	}
	return out
}

func cdpEventToConsoleEntry(msg map[string]any, tabTitle, tabURL string) (browserConsoleEntry, bool) {
	method, _ := msg["method"].(string)
	method = strings.TrimSpace(method)
	if method == "" {
		return browserConsoleEntry{}, false
	}

	params, _ := msg["params"].(map[string]any)
	switch method {
	case "Runtime.consoleAPICalled":
		level, _ := params["type"].(string)
		args, _ := params["args"].([]any)
		parts := make([]string, 0, len(args))
		for _, raw := range args {
			arg, _ := raw.(map[string]any)
			text := ""
			if value, ok := arg["value"]; ok {
				text = strings.TrimSpace(fmt.Sprintf("%v", value))
			}
			if text == "" {
				if desc, ok := arg["description"]; ok {
					text = strings.TrimSpace(fmt.Sprintf("%v", desc))
				}
			}
			if text != "" {
				parts = append(parts, text)
			}
		}
		event := browserConsoleEntry{
			Time:     time.Now().Format(time.RFC3339),
			Level:    safeValue(level, "log"),
			Source:   "runtime",
			Text:     strings.TrimSpace(strings.Join(parts, " ")),
			TabTitle: tabTitle,
			TabURL:   tabURL,
		}
		if event.Text == "" {
			return browserConsoleEntry{}, false
		}
		return event, true
	case "Log.entryAdded":
		entry, _ := params["entry"].(map[string]any)
		if len(entry) == 0 {
			return browserConsoleEntry{}, false
		}
		line := asInt(entry["lineNumber"])
		column := asInt(entry["columnNumber"])
		return browserConsoleEntry{
			Time:     time.Now().Format(time.RFC3339),
			Level:    safeValue(fmt.Sprintf("%v", entry["level"]), "info"),
			Source:   safeValue(fmt.Sprintf("%v", entry["source"]), "log"),
			Text:     safeValue(fmt.Sprintf("%v", entry["text"]), ""),
			URL:      strings.TrimSpace(fmt.Sprintf("%v", entry["url"])),
			Line:     line,
			Column:   column,
			TabTitle: tabTitle,
			TabURL:   tabURL,
		}, true
	default:
		return browserConsoleEntry{}, false
	}
}
