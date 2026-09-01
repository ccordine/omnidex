package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

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
