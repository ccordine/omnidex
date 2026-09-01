package main

import (
	"regexp"
	"strings"
	"time"
)

const browserProbeTimeout = 1500 * time.Millisecond
const defaultBrowserProbePorts = "9222,9223,9229,9333"
const browserEmailStateEnv = "OMNI_BROWSER_EMAIL_STATE_PATH"
const browserEmailStateVersion = 1
const browserEmailStateMaxSeen = 6000

var browserDebugPortPattern = regexp.MustCompile(`--remote-debugging-port=(\d+)`)
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

func containsAnyPhrase(input string, phrases []string) bool {
	for _, phrase := range phrases {
		if strings.Contains(input, phrase) {
			return true
		}
	}
	return false
}
