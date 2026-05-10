package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const browserProbeTimeout = 1500 * time.Millisecond
const defaultBrowserProbePorts = "9222,9223,9229,9333"
const browserEmailStateEnv = "OMNI_BROWSER_EMAIL_STATE_PATH"
const browserEmailStateVersion = 1
const browserEmailStateMaxSeen = 6000

var browserDebugPortPattern = regexp.MustCompile(`--remote-debugging-port=(\d+)`)
var browserSecondsPattern = regexp.MustCompile(`\b(\d{1,2})\s*(?:seconds?|secs?|s)\b`)
var numericDirPattern = regexp.MustCompile(`^\d+$`)

type browserProcess struct {
	PID       int
	Name      string
	ExecName  string
	Cmdline   string
	DebugPort int
}

type browserTarget struct {
	ID                   string `json:"id"`
	Type                 string `json:"type"`
	Title                string `json:"title"`
	URL                  string `json:"url"`
	WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
	DevtoolsFrontendURL  string `json:"devtoolsFrontendUrl"`
	BrowserContextID     string `json:"browserContextId"`
	OpenerID             string `json:"openerId"`
	Attached             bool   `json:"attached"`
	CanAccessOpener      bool   `json:"canAccessOpener"`
	TargetURL            string `json:"targetUrl"`
	FaviconURL           string `json:"faviconUrl"`
}

type browserVersion struct {
	Browser              string `json:"Browser"`
	ProtocolVersion      string `json:"Protocol-Version"`
	UserAgent            string `json:"User-Agent"`
	WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
}

type browserEndpoint struct {
	Port    int
	Version browserVersion
	Targets []browserTarget
}

type browserConsoleEntry struct {
	Time     string `json:"time"`
	Level    string `json:"level"`
	Source   string `json:"source"`
	Text     string `json:"text"`
	URL      string `json:"url,omitempty"`
	Line     int    `json:"line,omitempty"`
	Column   int    `json:"column,omitempty"`
	TabTitle string `json:"tab_title,omitempty"`
	TabURL   string `json:"tab_url,omitempty"`
}

type browserScanIntent struct {
	WithConsole bool
	EmailWatch  bool
	Seconds     int
	Limit       int
}

type browserEmailItem struct {
	Provider string `json:"provider,omitempty"`
	Sender   string `json:"sender,omitempty"`
	Subject  string `json:"subject,omitempty"`
	Preview  string `json:"preview,omitempty"`
	TimeText string `json:"time_text,omitempty"`
	Unread   bool   `json:"unread"`
	Key      string `json:"key,omitempty"`
}

type browserEmailTabSnapshot struct {
	Provider string             `json:"provider"`
	Mailbox  string             `json:"mailbox"`
	TabTitle string             `json:"tab_title"`
	TabURL   string             `json:"tab_url"`
	Items    []browserEmailItem `json:"items"`
	Error    string             `json:"error,omitempty"`
}

type browserEmailState struct {
	Version   int               `json:"version"`
	UpdatedAt string            `json:"updated_at"`
	Seen      map[string]string `json:"seen"`
}

func tryHandleLocalBrowserCommand(input string) (bool, string) {
	intent, ok := parseBrowserScanIntent(input)
	if !ok {
		return false, ""
	}
	if err := ensureLocalPermission(permissionKeyBrowserInspect, "Allow inspecting local browser processes and tab metadata."); err != nil {
		return true, "Local browser action blocked: " + err.Error()
	}

	warnings := make([]string, 0, 1)
	if intent.WithConsole {
		if err := ensureLocalPermission(permissionKeyBrowserConsole, "Allow reading JavaScript console events from local browser DevTools endpoints."); err != nil {
			intent.WithConsole = false
			warnings = append(warnings, err.Error())
		}
	}
	if intent.EmailWatch {
		if err := ensureLocalPermission(permissionKeyBrowserConsole, "Allow reading inbox summaries from local browser email tabs via DevTools endpoints."); err != nil {
			return true, "Local browser action blocked: " + err.Error()
		}
	}

	if intent.EmailWatch {
		report, err := browserEmailReport(defaultBrowserProbePorts)
		if err != nil {
			return true, "Local browser action failed: " + err.Error()
		}
		if len(warnings) > 0 {
			report["warnings"] = warnings
		}
		return true, browserEmailReportToText(report)
	}

	report, err := browserScanReport(intent.WithConsole, intent.Seconds, intent.Limit, defaultBrowserProbePorts)
	if err != nil {
		return true, "Local browser action failed: " + err.Error()
	}
	if len(warnings) > 0 {
		report["warnings"] = warnings
	}

	return true, reportToText(report)
}

func parseBrowserScanIntent(input string) (browserScanIntent, bool) {
	clean := strings.TrimSpace(input)
	if clean == "" {
		return browserScanIntent{}, false
	}
	lower := strings.ToLower(clean)
	consoleCue := containsAnyPhrase(lower, []string{
		"javascript console",
		"js console",
		"devtools console",
		"browser console",
		"console logs",
		"console log",
		"console errors",
		"read console",
		"inspect console",
	})
	emailCue := containsAnyPhrase(lower, []string{
		"email", "mailbox", "inbox", "gmail", "outlook", "protonmail", "yahoo mail", "new mail", "new email",
	})
	emailFreshCue := containsAnyPhrase(lower, []string{
		"just came in",
		"just come in",
		"what came in",
		"what has come in",
		"what has just come in",
		"what's new",
		"latest email",
		"latest mail",
		"new email",
		"new mail",
		"unread",
		"check my email",
		"check email",
	})
	browserCue := containsAnyPhrase(lower, []string{
		"browser", "chrome", "chromium", "firefox", "edge", "brave", "opera", "vivaldi",
	})
	tabCue := strings.Contains(lower, "tab")

	triggerPhrases := []string{
		"browser-scan",
		"open tabs",
		"active tabs",
		"running tabs",
		"what tabs",
		"which tabs",
		"list tabs",
		"show tabs",
		"read tabs",
		"read my tabs",
		"active browser",
		"active browsers",
		"running browser",
		"running browsers",
		"what browsers are running",
		"which browser is running",
		"scan browsers",
		"check browser",
		"check browsers",
		"javascript console",
		"js console",
		"devtools console",
		"browser console",
		"console logs",
		"console log",
		"console errors",
		"check my email",
		"check email",
		"email tabs",
		"inbox tabs",
	}
	triggered := containsAnyPhrase(lower, triggerPhrases)
	if !triggered {
		tabActionCue := containsAnyPhrase(lower, []string{
			"show", "list", "read", "scan", "check", "what", "which", "open", "active", "running", "inspect",
		})
		browserStateCue := containsAnyPhrase(lower, []string{
			"running", "active", "scan", "check", "inspect", "on my", "this machine", "local",
		})
		triggered = (tabCue && tabActionCue) || (browserCue && browserStateCue) || consoleCue || (emailCue && (tabCue || browserCue || emailFreshCue))
	}
	if !triggered {
		return browserScanIntent{}, false
	}

	intent := browserScanIntent{
		WithConsole: consoleCue,
		EmailWatch:  emailCue && emailFreshCue,
		Seconds:     2,
		Limit:       50,
	}
	if intent.WithConsole {
		intent.Seconds = 3
		intent.Limit = 80
	}

	if containsAnyPhrase(lower, []string{"live console", "stream console", "watch console"}) {
		intent.WithConsole = true
		intent.Seconds = 5
	}
	if matches := browserSecondsPattern.FindStringSubmatch(lower); len(matches) == 2 {
		if value, err := strconv.Atoi(strings.TrimSpace(matches[1])); err == nil {
			if value < 1 {
				value = 1
			}
			if value > 30 {
				value = 30
			}
			intent.WithConsole = true
			intent.Seconds = value
		}
	}
	if intent.EmailWatch {
		intent.WithConsole = false
		intent.Seconds = 0
		intent.Limit = 40
	}

	return intent, true
}

func containsAnyPhrase(input string, phrases []string) bool {
	for _, phrase := range phrases {
		if strings.Contains(input, phrase) {
			return true
		}
	}
	return false
}

func runBrowserScan(args []string) {
	fs := flag.NewFlagSet("browser-scan", flag.ExitOnError)
	withConsole := fs.Bool("console", false, "capture live JavaScript console events from debuggable tabs")
	emailWatch := fs.Bool("email-watch", false, "inspect email tabs and report newly visible inbox items since last scan")
	seconds := fs.Int("seconds", 2, "seconds to listen for console events per tab when --console is on")
	limit := fs.Int("limit", 50, "maximum console events to return when --console is on")
	defaultPorts := fs.String("ports", defaultBrowserProbePorts, "comma-separated debug ports to probe in addition to detected process flags")
	jsonOutput := fs.Bool("json", false, "print JSON output")
	_ = fs.Parse(args)
	if err := ensureLocalPermission(permissionKeyBrowserInspect, "Allow inspecting local browser processes and tab metadata."); err != nil {
		die(err.Error())
	}
	warnings := make([]string, 0, 1)
	if *withConsole {
		if err := ensureLocalPermission(permissionKeyBrowserConsole, "Allow reading JavaScript console events from local browser DevTools endpoints."); err != nil {
			*withConsole = false
			warnings = append(warnings, err.Error())
		}
	}
	if *emailWatch {
		if err := ensureLocalPermission(permissionKeyBrowserConsole, "Allow reading inbox summaries from local browser email tabs via DevTools endpoints."); err != nil {
			die(err.Error())
		}
	}

	if *emailWatch {
		report, err := browserEmailReport(*defaultPorts)
		if err != nil {
			die(err.Error())
		}
		if len(warnings) > 0 {
			report["warnings"] = warnings
		}
		if *jsonOutput {
			payload, err := json.MarshalIndent(report, "", "  ")
			if err != nil {
				die(err.Error())
			}
			fmt.Println(string(payload))
			return
		}
		fmt.Println(browserEmailReportToText(report))
		return
	}

	report, err := browserScanReport(*withConsole, *seconds, *limit, *defaultPorts)
	if err != nil {
		die(err.Error())
	}
	if len(warnings) > 0 {
		report["warnings"] = warnings
	}

	if *jsonOutput {
		payload, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			die(err.Error())
		}
		fmt.Println(string(payload))
		return
	}

	fmt.Println(reportToText(report))
}

func browserScanReport(withConsole bool, seconds, limit int, defaultPorts string) (map[string]any, error) {
	if seconds <= 0 {
		seconds = 2
	}
	if limit <= 0 {
		limit = 50
	}

	processes := discoverBrowserProcesses()
	ports := mergePorts(extractDebugPorts(processes), parsePortList(defaultPorts))
	endpoints := discoverBrowserEndpoints(ports)

	report := map[string]any{
		"generated_at":   time.Now().Format(time.RFC3339),
		"process_count":  len(processes),
		"endpoint_count": len(endpoints),
		"processes":      processes,
		"endpoints":      endpoints,
	}

	if withConsole {
		events := collectConsoleEvents(endpoints, time.Duration(seconds)*time.Second, limit)
		report["console_event_count"] = len(events)
		report["console_events"] = events
	}

	return report, nil
}

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

func cdpEvaluateJSON(wsURL, expression string, timeout time.Duration) (map[string]any, error) {
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	conn, err := cdpDialWebSocket(wsURL)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	deadline := time.Now().Add(timeout)
	_, _ = cdpCall(conn, 1001, "Runtime.enable", nil, deadline)
	response, err := cdpCall(conn, 1002, "Runtime.evaluate", map[string]any{
		"expression":    expression,
		"returnByValue": true,
		"awaitPromise":  true,
		"userGesture":   false,
		"replMode":      false,
	}, deadline)
	if err != nil {
		return nil, err
	}

	result, _ := response["result"].(map[string]any)
	if result == nil {
		return nil, errors.New("missing runtime evaluate result")
	}
	if exception, ok := result["exceptionDetails"].(map[string]any); ok && len(exception) > 0 {
		return nil, fmt.Errorf("runtime exception: %s", safeValue(fmt.Sprintf("%v", exception["text"]), "evaluate failed"))
	}
	evalResult, _ := result["result"].(map[string]any)
	if evalResult == nil {
		return nil, errors.New("missing evaluation payload")
	}
	value, _ := evalResult["value"].(map[string]any)
	if value == nil {
		return nil, errors.New("evaluation returned no JSON value")
	}
	return value, nil
}

func cdpCall(conn *cdpWebSocket, id int, method string, params map[string]any, deadline time.Time) (map[string]any, error) {
	request := map[string]any{
		"id":     id,
		"method": strings.TrimSpace(method),
	}
	if len(params) > 0 {
		request["params"] = params
	}
	if err := conn.SendJSON(request); err != nil {
		return nil, err
	}
	return cdpWaitForResponse(conn, id, deadline)
}

func cdpWaitForResponse(conn *cdpWebSocket, id int, deadline time.Time) (map[string]any, error) {
	for {
		if time.Now().After(deadline) {
			return nil, errors.New("cdp response timeout")
		}
		msg, err := conn.ReadJSONUntil(deadline)
		if err != nil {
			return nil, err
		}
		if asInt(msg["id"]) != id {
			continue
		}
		if errPayload, ok := msg["error"].(map[string]any); ok && len(errPayload) > 0 {
			message := strings.TrimSpace(fmt.Sprintf("%v", errPayload["message"]))
			if message == "" {
				message = "cdp call failed"
			}
			return nil, errors.New(message)
		}
		return msg, nil
	}
}

func browserEmailSnapshotExpression() string {
	return `(function () {
  const norm = (v) => String(v || "").replace(/\s+/g, " ").trim();
  const host = location.hostname.toLowerCase();
  let provider = "generic";
  if (host.includes("mail.google")) provider = "gmail";
  else if (host.includes("outlook.") || host.includes("office.com") || host.includes("live.com")) provider = "outlook";
  else if (host.includes("mail.yahoo")) provider = "yahoo";
  else if (host.includes("proton")) provider = "proton";

  const rows = [];
  const pushItem = (sender, subject, preview, timeText, unread) => {
    const item = {
      sender: norm(sender),
      subject: norm(subject),
      preview: norm(preview),
      time_text: norm(timeText),
      unread: !!unread
    };
    item.key = norm([item.sender, item.subject, item.preview, item.time_text].join("|")).toLowerCase();
    if (!item.key) return;
    if (!item.subject && !item.preview) return;
    rows.push(item);
  };

  const parseGenericRow = (el) => {
    const text = norm(el && el.innerText);
    if (!text || text.length < 10 || text.length > 500) return;
    const lines = text.split(/\n+/).map(norm).filter(Boolean);
    if (lines.length < 2) return;
    const sender = lines[0];
    const subject = lines[1] || "";
    const preview = lines.slice(2, 4).join(" ");
    const timeText = lines.find((line) => /\b(\d{1,2}:\d{2}|am|pm|ago|yesterday|today)\b/i.test(line)) || "";
    const marker = norm((el.getAttribute("aria-label") || "") + " " + (el.className || ""));
    const unread = /unread|new/i.test(marker);
    pushItem(sender, subject, preview, timeText, unread);
  };

  if (provider === "gmail") {
    document.querySelectorAll("tr.zA").forEach((row) => {
      const sender = row.querySelector(".yP, .yW span[email], .yW span")?.innerText || "";
      const subject = row.querySelector(".bog")?.innerText || "";
      const preview = row.querySelector(".y2")?.innerText || "";
      const timeText = row.querySelector("td.xW span")?.innerText || "";
      const unread = row.classList.contains("zE") || /unread/i.test(row.getAttribute("aria-label") || "");
      pushItem(sender, subject, preview, timeText, unread);
    });
  }

  if (provider === "outlook" && rows.length === 0) {
    document.querySelectorAll("[role='row']").forEach((row) => {
      const sender = row.querySelector("[data-automationid='Sender']")?.innerText || row.querySelector("[title]")?.innerText || "";
      const subject = row.querySelector("[data-automationid='SubjectLine']")?.innerText || "";
      const preview = row.querySelector("[data-automationid='MessagePreview']")?.innerText || "";
      const timeText = row.querySelector("[data-automationid='ReceivedTime']")?.innerText || "";
      const unread = /unread|isreadfalse/i.test((row.className || "") + " " + (row.getAttribute("aria-label") || ""));
      pushItem(sender, subject, preview, timeText, unread);
    });
  }

  if (rows.length === 0) {
    const selectors = ["[role='row']", "tr", "li", "article", "div[aria-label*='mail']", "div[aria-label*='inbox']"];
    selectors.forEach((selector) => {
      document.querySelectorAll(selector).forEach(parseGenericRow);
    });
  }

  const unique = [];
  const seen = new Set();
  for (const row of rows) {
    if (!row.key || seen.has(row.key)) continue;
    seen.add(row.key);
    unique.push(row);
    if (unique.length >= 40) break;
  }

  return {
    provider: provider,
    mailbox_key: location.origin + location.pathname.replace(/\/+$/, ""),
    page_title: document.title,
    items: unique
  };
})()`
}

func browserEmailStateKey(mailbox string, item browserEmailItem) string {
	mailboxKey := strings.ToLower(strings.TrimSpace(mailbox))
	if mailboxKey == "" {
		mailboxKey = "unknown-mailbox"
	}
	itemKey := strings.ToLower(strings.TrimSpace(item.Key))
	if itemKey == "" {
		itemKey = strings.ToLower(strings.TrimSpace(strings.Join([]string{item.Sender, item.Subject, item.Preview, item.TimeText}, "|")))
	}
	if itemKey == "" {
		return ""
	}
	return mailboxKey + "::" + itemKey
}

func defaultBrowserEmailStatePath() string {
	if raw := strings.TrimSpace(os.Getenv(browserEmailStateEnv)); raw != "" {
		return raw
	}
	if xdg := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME")); xdg != "" {
		return filepath.Join(xdg, "omni", "browser_email_state.json")
	}
	if home, err := os.UserHomeDir(); err == nil && strings.TrimSpace(home) != "" {
		return filepath.Join(home, ".config", "omni", "browser_email_state.json")
	}
	return filepath.Join(".omni", "browser_email_state.json")
}

func loadBrowserEmailState(path string) (browserEmailState, error) {
	state := browserEmailState{
		Version: browserEmailStateVersion,
		Seen:    map[string]string{},
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return state, nil
		}
		return state, err
	}
	if strings.TrimSpace(string(data)) == "" {
		return state, nil
	}
	if err := json.Unmarshal(data, &state); err != nil {
		return state, err
	}
	if state.Version == 0 {
		state.Version = browserEmailStateVersion
	}
	if state.Seen == nil {
		state.Seen = map[string]string{}
	}
	return state, nil
}

func saveBrowserEmailState(path string, state browserEmailState) error {
	state.Version = browserEmailStateVersion
	state.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	if state.Seen == nil {
		state.Seen = map[string]string{}
	}
	pruneBrowserEmailState(&state, browserEmailStateMaxSeen)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	payload, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, payload, 0o600)
}

func pruneBrowserEmailState(state *browserEmailState, maxEntries int) {
	if state == nil || maxEntries <= 0 || len(state.Seen) <= maxEntries {
		return
	}
	type kv struct {
		Key string
		At  time.Time
	}
	entries := make([]kv, 0, len(state.Seen))
	for key, ts := range state.Seen {
		at, err := time.Parse(time.RFC3339, strings.TrimSpace(ts))
		if err != nil {
			at = time.Time{}
		}
		entries = append(entries, kv{Key: key, At: at})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].At.Before(entries[j].At)
	})
	removeCount := len(entries) - maxEntries
	if removeCount <= 0 {
		return
	}
	for i := 0; i < removeCount; i++ {
		delete(state.Seen, entries[i].Key)
	}
}

func normalizeCompactText(value string) string {
	clean := strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
	return strings.TrimSpace(clean)
}

func reportToText(report map[string]any) string {
	lines := []string{
		"Local browser scan:",
		"generated_at=" + safeValue(fmt.Sprintf("%v", report["generated_at"]), "unknown"),
		fmt.Sprintf("process_count=%v", report["process_count"]),
		fmt.Sprintf("endpoint_count=%v", report["endpoint_count"]),
	}

	if processes, ok := report["processes"].([]browserProcess); ok && len(processes) > 0 {
		lines = append(lines, "processes:")
		for _, proc := range processes {
			desc := fmt.Sprintf("- pid=%d name=%s", proc.PID, safeValue(proc.Name, proc.ExecName))
			if proc.DebugPort > 0 {
				desc += fmt.Sprintf(" debug_port=%d", proc.DebugPort)
			}
			if strings.TrimSpace(proc.Cmdline) != "" {
				desc += " cmd=" + truncateText(proc.Cmdline, 220)
			}
			lines = append(lines, desc)
		}
	}

	if endpoints, ok := report["endpoints"].([]browserEndpoint); ok && len(endpoints) > 0 {
		lines = append(lines, "tabs:")
		for _, endpoint := range endpoints {
			header := fmt.Sprintf("- port=%d browser=%s protocol=%s", endpoint.Port, safeValue(endpoint.Version.Browser, "unknown"), safeValue(endpoint.Version.ProtocolVersion, "unknown"))
			lines = append(lines, header)
			for _, target := range endpoint.Targets {
				if strings.ToLower(strings.TrimSpace(target.Type)) != "page" {
					continue
				}
				lines = append(lines, fmt.Sprintf("  • %s | %s", safeValue(target.Title, "(untitled)"), safeValue(target.URL, "(no url)")))
			}
		}
	}

	if events, ok := report["console_events"].([]browserConsoleEntry); ok && len(events) > 0 {
		lines = append(lines, "console_events:")
		for _, event := range events {
			line := fmt.Sprintf("- [%s] %s %s", safeValue(event.Time, "unknown"), strings.ToUpper(safeValue(event.Level, "log")), safeValue(event.Text, ""))
			if strings.TrimSpace(event.TabTitle) != "" {
				line += " tab=" + event.TabTitle
			}
			if strings.TrimSpace(event.URL) != "" {
				line += " url=" + event.URL
			}
			lines = append(lines, line)
		}
	} else if _, ok := report["console_events"]; ok {
		lines = append(lines, "console_events: none captured")
	}
	if warningValues, ok := report["warnings"].([]string); ok && len(warningValues) > 0 {
		lines = append(lines, "warnings:")
		for _, warning := range warningValues {
			lines = append(lines, "- "+warning)
		}
	}
	if endpointCount, ok := report["endpoint_count"].(int); ok && endpointCount == 0 {
		lines = append(lines, "Note: tab and console access usually requires launching a browser with --remote-debugging-port=9222.")
	}

	if len(lines) == 4 {
		lines = append(lines, "No active browser process or debuggable endpoint found.")
	}
	return strings.Join(lines, "\n")
}

func discoverBrowserProcesses() []browserProcess {
	rootEntries, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}

	processes := make([]browserProcess, 0, 32)
	for _, entry := range rootEntries {
		if !entry.IsDir() {
			continue
		}
		pidText := strings.TrimSpace(entry.Name())
		if !numericDirPattern.MatchString(pidText) {
			continue
		}
		pid, err := strconv.Atoi(pidText)
		if err != nil || pid <= 0 {
			continue
		}
		proc, ok := readBrowserProcess(pid)
		if !ok {
			continue
		}
		processes = append(processes, proc)
	}

	sort.Slice(processes, func(i, j int) bool {
		if processes[i].Name == processes[j].Name {
			return processes[i].PID < processes[j].PID
		}
		return processes[i].Name < processes[j].Name
	})
	return processes
}

func readBrowserProcess(pid int) (browserProcess, bool) {
	commPath := filepath.Join("/proc", strconv.Itoa(pid), "comm")
	cmdPath := filepath.Join("/proc", strconv.Itoa(pid), "cmdline")
	exePath := filepath.Join("/proc", strconv.Itoa(pid), "exe")

	commBytes, err := os.ReadFile(commPath)
	if err != nil {
		return browserProcess{}, false
	}
	comm := strings.TrimSpace(string(commBytes))
	exeName := ""
	if target, err := os.Readlink(exePath); err == nil {
		exeName = strings.TrimSpace(filepath.Base(target))
	}

	cmdline := ""
	firstArg := ""
	if raw, err := os.ReadFile(cmdPath); err == nil && len(raw) > 0 {
		parts := strings.Split(string(raw), "\x00")
		filtered := make([]string, 0, len(parts))
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			filtered = append(filtered, part)
		}
		if len(filtered) > 0 {
			firstArg = filtered[0]
		}
		cmdline = strings.Join(filtered, " ")
	}

	name := classifyBrowserName(comm, exeName, firstArg, cmdline)
	if name == "" {
		return browserProcess{}, false
	}
	if isLikelyBrowserHelperProcess(cmdline) {
		return browserProcess{}, false
	}

	return browserProcess{
		PID:       pid,
		Name:      name,
		ExecName:  exeName,
		Cmdline:   cmdline,
		DebugPort: parseDebugPortFromCmdline(cmdline),
	}, true
}

func classifyBrowserName(comm, exeName, firstArg, cmdline string) string {
	joined := strings.ToLower(strings.TrimSpace(strings.Join([]string{exeName, filepath.Base(firstArg), firstArg}, " ")))
	commLower := strings.ToLower(strings.TrimSpace(comm))
	if strings.Contains(joined, "firefox") || commLower == "firefox" {
		return "firefox"
	}
	switch {
	case strings.Contains(joined, "brave"):
		return "brave"
	case strings.Contains(joined, "chromium"):
		return "chromium"
	case strings.Contains(joined, "google-chrome"), strings.Contains(joined, "chrome"):
		return "chrome"
	case strings.Contains(joined, "microsoft-edge"), strings.Contains(joined, "msedge"), strings.Contains(joined, "edge"):
		return "edge"
	case strings.Contains(joined, "vivaldi"):
		return "vivaldi"
	case strings.Contains(joined, "opera"):
		return "opera"
	case commLower == "chrome":
		// "chrome" is ambiguous for Electron helpers; only accept when first argv
		// references a browser-like binary.
		if strings.Contains(strings.ToLower(filepath.Base(firstArg)), "chrome") {
			return "chrome"
		}
		return ""
	default:
		return ""
	}
}

func isLikelyBrowserHelperProcess(cmdline string) bool {
	lower := strings.ToLower(strings.TrimSpace(cmdline))
	if lower == "" {
		return false
	}
	helperMarkers := []string{
		"steamwebhelper",
		"chrome_crashpad_handler",
		"crashpad-handler",
		"crashhelper",
		"electron",
		"slack",
		"discord",
		"teams-for-linux",
		"--type=",
		"-contentproc",
		" --utility-sub-type=",
	}
	return containsAnyPhrase(lower, helperMarkers)
}

func parseDebugPortFromCmdline(cmdline string) int {
	match := browserDebugPortPattern.FindStringSubmatch(strings.TrimSpace(cmdline))
	if len(match) != 2 {
		return 0
	}
	port, err := strconv.Atoi(strings.TrimSpace(match[1]))
	if err != nil || port <= 0 || port > 65535 {
		return 0
	}
	return port
}

func extractDebugPorts(processes []browserProcess) []int {
	ports := make([]int, 0, len(processes))
	for _, proc := range processes {
		if proc.DebugPort > 0 {
			ports = append(ports, proc.DebugPort)
		}
	}
	return ports
}

func parsePortList(raw string) []int {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]int, 0, len(parts))
	for _, part := range parts {
		value := strings.TrimSpace(part)
		if value == "" {
			continue
		}
		port, err := strconv.Atoi(value)
		if err != nil || port <= 0 || port > 65535 {
			continue
		}
		out = append(out, port)
	}
	return out
}

func mergePorts(groups ...[]int) []int {
	seen := map[int]struct{}{}
	out := make([]int, 0, 16)
	for _, group := range groups {
		for _, port := range group {
			if port <= 0 || port > 65535 {
				continue
			}
			if _, ok := seen[port]; ok {
				continue
			}
			seen[port] = struct{}{}
			out = append(out, port)
		}
	}
	sort.Ints(out)
	return out
}

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

type cdpWebSocket struct {
	conn net.Conn
	rd   *bufio.Reader
}

func cdpDialWebSocket(rawURL string) (*cdpWebSocket, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return nil, err
	}
	if parsed.Scheme != "ws" {
		return nil, fmt.Errorf("unsupported websocket scheme %q", parsed.Scheme)
	}

	host := parsed.Host
	if !strings.Contains(host, ":") {
		host += ":80"
	}

	dialer := net.Dialer{Timeout: browserProbeTimeout}
	conn, err := dialer.Dial("tcp", host)
	if err != nil {
		return nil, err
	}

	keyBytes := make([]byte, 16)
	if _, err := rand.Read(keyBytes); err != nil {
		conn.Close()
		return nil, err
	}
	key := base64.StdEncoding.EncodeToString(keyBytes)
	path := parsed.RequestURI()
	if path == "" {
		path = "/"
	}

	request := strings.Join([]string{
		fmt.Sprintf("GET %s HTTP/1.1", path),
		"Host: " + parsed.Host,
		"Upgrade: websocket",
		"Connection: Upgrade",
		"Sec-WebSocket-Key: " + key,
		"Sec-WebSocket-Version: 13",
		"",
		"",
	}, "\r\n")

	if _, err := conn.Write([]byte(request)); err != nil {
		conn.Close()
		return nil, err
	}

	reader := bufio.NewReader(conn)
	statusLine, err := reader.ReadString('\n')
	if err != nil {
		conn.Close()
		return nil, err
	}
	if !strings.Contains(statusLine, "101") {
		conn.Close()
		return nil, fmt.Errorf("websocket upgrade failed: %s", strings.TrimSpace(statusLine))
	}

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			conn.Close()
			return nil, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
	}

	return &cdpWebSocket{conn: conn, rd: reader}, nil
}

func (w *cdpWebSocket) Close() error {
	return w.conn.Close()
}

func (w *cdpWebSocket) SendJSON(value any) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return w.writeFrame(0x1, payload)
}

func (w *cdpWebSocket) ReadJSONUntil(deadline time.Time) (map[string]any, error) {
	for {
		if err := w.conn.SetReadDeadline(deadline); err != nil {
			return nil, err
		}
		op, payload, err := w.readFrame()
		if err != nil {
			return nil, err
		}
		switch op {
		case 0x1:
			var out map[string]any
			if err := json.Unmarshal(payload, &out); err != nil {
				continue
			}
			return out, nil
		case 0x8:
			return nil, io.EOF
		case 0x9:
			_ = w.writeFrame(0xA, payload)
		default:
		}
	}
}

func (w *cdpWebSocket) writeFrame(opcode byte, payload []byte) error {
	maskKey := make([]byte, 4)
	if _, err := rand.Read(maskKey); err != nil {
		return err
	}

	header := []byte{0x80 | (opcode & 0x0F)}
	payloadLen := len(payload)
	switch {
	case payloadLen <= 125:
		header = append(header, 0x80|byte(payloadLen))
	case payloadLen <= 65535:
		header = append(header, 0x80|126)
		ext := make([]byte, 2)
		binary.BigEndian.PutUint16(ext, uint16(payloadLen))
		header = append(header, ext...)
	default:
		header = append(header, 0x80|127)
		ext := make([]byte, 8)
		binary.BigEndian.PutUint64(ext, uint64(payloadLen))
		header = append(header, ext...)
	}
	masked := make([]byte, len(payload))
	for i := range payload {
		masked[i] = payload[i] ^ maskKey[i%4]
	}

	packet := append(header, maskKey...)
	packet = append(packet, masked...)
	_, err := w.conn.Write(packet)
	return err
}

func (w *cdpWebSocket) readFrame() (byte, []byte, error) {
	header := make([]byte, 2)
	if _, err := io.ReadFull(w.rd, header); err != nil {
		return 0, nil, err
	}

	opcode := header[0] & 0x0F
	masked := (header[1] & 0x80) != 0
	length := int(header[1] & 0x7F)
	switch length {
	case 126:
		ext := make([]byte, 2)
		if _, err := io.ReadFull(w.rd, ext); err != nil {
			return 0, nil, err
		}
		length = int(binary.BigEndian.Uint16(ext))
	case 127:
		ext := make([]byte, 8)
		if _, err := io.ReadFull(w.rd, ext); err != nil {
			return 0, nil, err
		}
		size := binary.BigEndian.Uint64(ext)
		if size > 8*1024*1024 {
			return 0, nil, errors.New("websocket frame too large")
		}
		length = int(size)
	}

	var maskKey []byte
	if masked {
		maskKey = make([]byte, 4)
		if _, err := io.ReadFull(w.rd, maskKey); err != nil {
			return 0, nil, err
		}
	}

	payload := make([]byte, length)
	if length > 0 {
		if _, err := io.ReadFull(w.rd, payload); err != nil {
			return 0, nil, err
		}
	}
	if masked {
		for i := range payload {
			payload[i] ^= maskKey[i%4]
		}
	}
	return opcode, payload, nil
}

func asInt(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case string:
		num, err := strconv.Atoi(strings.TrimSpace(typed))
		if err == nil {
			return num
		}
	}
	return 0
}

func truncateText(value string, maxRunes int) string {
	clean := strings.TrimSpace(value)
	if maxRunes <= 0 {
		return clean
	}
	runes := []rune(clean)
	if len(runes) <= maxRunes {
		return clean
	}
	return string(runes[:maxRunes]) + "..."
}
