package main

import (
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"
)

type browserEmailTabResult struct {
	Provider     string             `json:"provider"`
	Mailbox      string             `json:"mailbox"`
	TabTitle     string             `json:"tab_title"`
	TabURL       string             `json:"tab_url"`
	VisibleCount int                `json:"visible_count"`
	NewItems     []browserEmailItem `json:"new_items"`
	Error        string             `json:"error,omitempty"`
}

func browserEmailReport(defaultPorts string) (map[string]any, error) {
	processes := discoverBrowserProcesses()
	ports := mergePorts(extractDebugPorts(processes), parsePortList(defaultPorts))
	endpoints := discoverBrowserEndpoints(ports)
	snapshots := collectEmailTabSnapshots(endpoints)

	statePath := defaultBrowserEmailStatePath()
	state, loadErr := loadBrowserEmailState(statePath)
	warnings := make([]string, 0, 2)
	if loadErr != nil {
		warnings = append(warnings, "email state load failed: "+loadErr.Error())
		state = browserEmailState{
			Version: browserEmailStateVersion,
			Seen:    map[string]string{},
		}
	}
	if state.Seen == nil {
		state.Seen = map[string]string{}
	}

	now := time.Now().UTC().Format(time.RFC3339)
	tabResults := make([]browserEmailTabResult, 0, len(snapshots))
	newCount := 0
	visibleCount := 0
	for _, snapshot := range snapshots {
		result := browserEmailTabResult{
			Provider: snapshot.Provider,
			Mailbox:  snapshot.Mailbox,
			TabTitle: snapshot.TabTitle,
			TabURL:   snapshot.TabURL,
			Error:    snapshot.Error,
		}
		result.VisibleCount = len(snapshot.Items)
		visibleCount += result.VisibleCount
		for _, item := range snapshot.Items {
			stateKey := browserEmailStateKey(snapshot.Mailbox, item)
			if stateKey == "" {
				continue
			}
			if _, seen := state.Seen[stateKey]; !seen {
				result.NewItems = append(result.NewItems, item)
				newCount++
			}
			state.Seen[stateKey] = now
		}
		tabResults = append(tabResults, result)
	}

	pruneBrowserEmailState(&state, browserEmailStateMaxSeen)
	if err := saveBrowserEmailState(statePath, state); err != nil {
		warnings = append(warnings, "email state save failed: "+err.Error())
	}

	report := map[string]any{
		"generated_at":       time.Now().Format(time.RFC3339),
		"process_count":      len(processes),
		"endpoint_count":     len(endpoints),
		"email_tab_count":    len(tabResults),
		"visible_item_count": visibleCount,
		"new_item_count":     newCount,
		"email_tabs":         tabResults,
		"state_path":         statePath,
	}
	if len(warnings) > 0 {
		report["warnings"] = warnings
	}
	return report, nil
}

func browserEmailReportToText(report map[string]any) string {
	lines := []string{
		"Local browser email scan:",
		"generated_at=" + safeValue(fmt.Sprintf("%v", report["generated_at"]), "unknown"),
		fmt.Sprintf("process_count=%v", report["process_count"]),
		fmt.Sprintf("endpoint_count=%v", report["endpoint_count"]),
		fmt.Sprintf("email_tab_count=%v", report["email_tab_count"]),
		fmt.Sprintf("visible_item_count=%v", report["visible_item_count"]),
		fmt.Sprintf("new_item_count=%v", report["new_item_count"]),
	}
	if statePath := strings.TrimSpace(fmt.Sprintf("%v", report["state_path"])); statePath != "" {
		lines = append(lines, "state_path="+statePath)
	}

	if tabs, ok := report["email_tabs"].([]browserEmailTabResult); ok && len(tabs) > 0 {
		lines = append(lines, "email_tabs:")
		for _, tab := range tabs {
			head := fmt.Sprintf("- provider=%s mailbox=%s title=%s visible=%d new=%d",
				safeValue(tab.Provider, "unknown"),
				safeValue(tab.Mailbox, "unknown"),
				safeValue(tab.TabTitle, "(untitled)"),
				tab.VisibleCount,
				len(tab.NewItems),
			)
			if strings.TrimSpace(tab.TabURL) != "" {
				head += " url=" + tab.TabURL
			}
			if strings.TrimSpace(tab.Error) != "" {
				head += " error=" + tab.Error
			}
			lines = append(lines, head)
			for _, item := range tab.NewItems {
				unreadText := ""
				if item.Unread {
					unreadText = " unread=true"
				}
				lines = append(lines, fmt.Sprintf("  • sender=%s subject=%s time=%s%s",
					safeValue(item.Sender, "(unknown)"),
					safeValue(item.Subject, "(no subject)"),
					safeValue(item.TimeText, "(no time)"),
					unreadText,
				))
				if strings.TrimSpace(item.Preview) != "" {
					lines = append(lines, "    preview="+truncateText(item.Preview, 180))
				}
			}
		}
	} else {
		lines = append(lines, "No debuggable email tabs found.")
		lines = append(lines, "Note: email tab inspection usually requires launching browser with --remote-debugging-port=9222.")
	}

	if warningValues, ok := report["warnings"].([]string); ok && len(warningValues) > 0 {
		lines = append(lines, "warnings:")
		for _, warning := range warningValues {
			lines = append(lines, "- "+warning)
		}
	}

	return strings.Join(lines, "\n")
}

func collectEmailTabSnapshots(endpoints []browserEndpoint) []browserEmailTabSnapshot {
	out := make([]browserEmailTabSnapshot, 0, 8)
	for _, endpoint := range endpoints {
		for _, target := range endpoint.Targets {
			if strings.ToLower(strings.TrimSpace(target.Type)) != "page" {
				continue
			}
			if !looksLikeEmailTarget(target) {
				continue
			}
			snapshot := browserEmailTabSnapshot{
				Provider: detectEmailProvider(target.URL),
				Mailbox:  normalizeMailboxKey(target.URL),
				TabTitle: strings.TrimSpace(target.Title),
				TabURL:   strings.TrimSpace(target.URL),
			}
			wsURL := strings.TrimSpace(target.WebSocketDebuggerURL)
			if wsURL == "" {
				snapshot.Error = "tab has no debugger websocket"
				out = append(out, snapshot)
				continue
			}
			payload, err := cdpEvaluateJSON(wsURL, browserEmailSnapshotExpression(), 2500*time.Millisecond)
			if err != nil {
				snapshot.Error = err.Error()
				out = append(out, snapshot)
				continue
			}

			if provider := strings.TrimSpace(fmt.Sprintf("%v", payload["provider"])); provider != "" {
				snapshot.Provider = provider
			}
			if mailbox := strings.TrimSpace(fmt.Sprintf("%v", payload["mailbox_key"])); mailbox != "" {
				snapshot.Mailbox = normalizeMailboxKey(mailbox)
			}
			if title := strings.TrimSpace(fmt.Sprintf("%v", payload["page_title"])); title != "" {
				snapshot.TabTitle = title
			}
			snapshot.Items = parseEmailItems(payload["items"], snapshot.Provider)
			out = append(out, snapshot)
		}
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].Provider == out[j].Provider {
			if out[i].Mailbox == out[j].Mailbox {
				return out[i].TabTitle < out[j].TabTitle
			}
			return out[i].Mailbox < out[j].Mailbox
		}
		return out[i].Provider < out[j].Provider
	})
	return out
}

func looksLikeEmailTarget(target browserTarget) bool {
	return looksLikeEmailURL(target.URL) || looksLikeEmailTitle(target.Title)
}

func looksLikeEmailURL(rawURL string) bool {
	value := strings.ToLower(strings.TrimSpace(rawURL))
	if value == "" {
		return false
	}
	emailMarkers := []string{
		"mail.google.com",
		"outlook.office.com",
		"outlook.live.com",
		"mail.yahoo.com",
		"proton.me/mail",
		"protonmail",
		"/mail",
		"/inbox",
	}
	return containsAnyPhrase(value, emailMarkers)
}

func looksLikeEmailTitle(title string) bool {
	value := strings.ToLower(strings.TrimSpace(title))
	if value == "" {
		return false
	}
	return containsAnyPhrase(value, []string{"inbox", "gmail", "outlook", "yahoo mail", "proton mail", "mail -"})
}

func detectEmailProvider(rawURL string) string {
	value := strings.ToLower(strings.TrimSpace(rawURL))
	switch {
	case strings.Contains(value, "mail.google.com"):
		return "gmail"
	case strings.Contains(value, "outlook.office.com"), strings.Contains(value, "outlook.live.com"), strings.Contains(value, "office.com"):
		return "outlook"
	case strings.Contains(value, "mail.yahoo.com"), strings.Contains(value, "yahoo"):
		return "yahoo"
	case strings.Contains(value, "proton.me"), strings.Contains(value, "protonmail"):
		return "proton"
	default:
		return "generic"
	}
}

func normalizeMailboxKey(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "unknown-mailbox"
	}
	parsed, err := url.Parse(value)
	if err != nil || strings.TrimSpace(parsed.Scheme) == "" || strings.TrimSpace(parsed.Host) == "" {
		return strings.ToLower(strings.TrimRight(value, "/"))
	}
	path := strings.TrimRight(strings.TrimSpace(parsed.Path), "/")
	if path == "" {
		path = "/"
	}
	return strings.ToLower(parsed.Scheme + "://" + parsed.Host + path)
}

func parseEmailItems(raw any, provider string) []browserEmailItem {
	itemsAny, ok := raw.([]any)
	if !ok || len(itemsAny) == 0 {
		return nil
	}
	items := make([]browserEmailItem, 0, len(itemsAny))
	for _, value := range itemsAny {
		itemMap, ok := value.(map[string]any)
		if !ok {
			continue
		}
		item := browserEmailItem{
			Provider: safeValue(strings.TrimSpace(fmt.Sprintf("%v", itemMap["provider"])), provider),
			Sender:   normalizeCompactText(fmt.Sprintf("%v", itemMap["sender"])),
			Subject:  normalizeCompactText(fmt.Sprintf("%v", itemMap["subject"])),
			Preview:  normalizeCompactText(fmt.Sprintf("%v", itemMap["preview"])),
			TimeText: normalizeCompactText(fmt.Sprintf("%v", itemMap["time_text"])),
			Unread:   strings.EqualFold(strings.TrimSpace(fmt.Sprintf("%v", itemMap["unread"])), "true"),
			Key:      normalizeCompactText(fmt.Sprintf("%v", itemMap["key"])),
		}
		if item.Key == "" {
			item.Key = strings.ToLower(strings.TrimSpace(strings.Join([]string{item.Sender, item.Subject, item.Preview, item.TimeText}, "|")))
		}
		if item.Key == "" {
			continue
		}
		if item.Subject == "" && item.Preview == "" {
			continue
		}
		items = append(items, item)
	}
	return items
}
