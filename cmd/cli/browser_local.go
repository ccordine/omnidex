package main

import (
	"regexp"
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
