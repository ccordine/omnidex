package main

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

type workspaceFocus uint8

const (
	workspaceCompose workspaceFocus = iota
	workspaceConversation
	workspaceActivity
	workspaceLedgerPane
	workspaceSessionsPane
)

// chatWorkspace is the single physical terminal presenter. The existing
// terminal editor and review router keep their exact input/approval authority.
type chatWorkspace struct {
	sessions                                             workspaceSessions
	reviewWriting                                        bool
	editorSwitching                                      bool
	helpOffset                                           int
	modeRequests                                         chan string
	modeOffsets                                          map[string]int
	conversationMode                                     string
	researchLines                                        []string
	ledger                                               workspaceLedger
	lastCursorRow, lastCursorColumn                      int
	lastCursorVisible                                    bool
	reviewFocusPrefix                                    string
	reviewFocus                                          bool
	mu                                                   sync.Mutex
	output                                               io.Writer
	width, height                                        int
	conversation, review                                 workspaceScreen
	reviewing, cursorVisible, closed                     bool
	pending                                              []byte
	lastFrame                                            []string
	focus                                                workspaceFocus
	zoom, help, prefix                                   bool
	conversationOffset, activityOffset                   int
	project, branch                                      string
	activity                                             chatActivity
	detail                                               string
	events                                               []string
	role, model                                          string
	routeJob                                             int64
	routes                                               map[string]string
	editing                                              bool
	draftRow, draftColumn, draftDropped, transcriptBytes int
	transcriptWriting                                    bool
	transcriptAnchors                                    []workspaceResizeAnchor
	color                                                bool
}

func newChatWorkspace(output io.Writer, width, height int) (*chatWorkspace, error) {
	view := &chatWorkspace{modeRequests: make(chan string, 16), modeOffsets: make(map[string]int), conversationMode: "infer", output: output, width: max(1, width), height: max(1, height), cursorVisible: true, color: os.Getenv("NO_COLOR") == ""}
	view.ledger.requests = make(chan workspaceLedgerRequest, 1)
	columns, _ := view.editorSizeLocked()
	view.conversation, view.review = newWorkspaceScreen(columns), newWorkspaceScreen(columns)
	if err := writeTerminalControl(output, "\x1b[?1049h\x1b[2J\x1b[H"); err != nil {
		return nil, err
	}
	return view, nil
}

func (view *chatWorkspace) Close() error {
	view.mu.Lock()
	defer view.mu.Unlock()
	if view.closed {
		return nil
	}
	view.closed = true
	return writeTerminalControl(view.output, "\x1b[0m\x1b[?25h\x1b[?1049l")
}

func (view *chatWorkspace) Write(value []byte) (int, error) {
	view.mu.Lock()
	defer view.mu.Unlock()
	if view.closed {
		return 0, io.ErrClosedPipe
	}
	view.pending = append(view.pending, value...)
	for len(view.pending) > 0 {
		if view.pending[0] == '\x1b' {
			if len(view.pending) < 2 {
				break
			}
			if view.pending[1] != '[' {
				return 0, fmt.Errorf("unsupported editor escape")
			}
			end := 2
			for end < len(view.pending) && (view.pending[end] < 0x40 || view.pending[end] > 0x7e) {
				end++
			}
			if end == len(view.pending) {
				break
			}
			if err := view.controlLocked(string(view.pending[2:end]), view.pending[end]); err != nil {
				return 0, err
			}
			view.pending = view.pending[end+1:]
			continue
		}
		if !utf8.FullRune(view.pending) {
			break
		}
		character, count := utf8.DecodeRune(view.pending)
		view.screenLocked().put(character)
		if view.transcriptWriting && view.transcriptBytes > 0 {
			view.transcriptBytes -= count
			if view.transcriptBytes == 0 {
				view.markDraftLocked()
			}
		}
		view.pending = view.pending[count:]
	}
	if err := view.drawLocked(); err != nil {
		return 0, err
	}
	return len(value), nil
}

func (view *chatWorkspace) screenLocked() *workspaceScreen {
	if view.reviewWriting || view.reviewVisibleLocked() {
		return &view.review
	}
	return &view.conversation
}

func (view *chatWorkspace) controlLocked(parameters string, command byte) error {
	if strings.HasPrefix(parameters, "?") {
		on := command == 'h'
		switch parameters {
		case "?1049":
			view.reviewing = on
			if on {
				columns, _ := view.editorSizeLocked()
				view.review = newWorkspaceScreen(columns)
			}
		case "?25":
			view.cursorVisible = on
		case "?2004":
			return writeTerminalControl(view.output, fmt.Sprintf("\x1b[%s%c", parameters, command))
		default:
			return fmt.Errorf("unsupported editor private terminal control %q", parameters)
		}
		return nil
	}
	return view.screenLocked().control(parameters, command)
}

func (view *chatWorkspace) editorSizeLocked() (int, int) {
	columns := view.width - 4
	return max(10, columns), max(3, view.height-5)
}

func (view *chatWorkspace) beginInput() {
	view.mu.Lock()
	defer view.mu.Unlock()
	view.editing = true
	view.markDraftLocked()
}

func (view *chatWorkspace) endInput() {
	view.mu.Lock()
	defer view.mu.Unlock()
	view.editing = false
}

func (view *chatWorkspace) beginTranscript(value string) {
	view.mu.Lock()
	defer view.mu.Unlock()
	view.transcriptWriting = true
	view.transcriptAnchors = view.resizeAnchorsLocked()
	view.transcriptBytes = len(strings.ReplaceAll(value, "\n", "\r\n"))
}

func (view *chatWorkspace) endTranscript() error {
	view.mu.Lock()
	defer view.mu.Unlock()
	view.transcriptWriting = false
	view.restoreResizeAnchorsLocked(view.transcriptAnchors)
	view.transcriptAnchors = nil
	return view.drawLocked()
}

func (view *chatWorkspace) markDraftLocked() {
	view.draftRow, view.draftColumn, view.draftDropped = view.conversation.row, view.conversation.column, view.conversation.dropped
	if view.conversation.wrap {
		view.draftRow++
		view.draftColumn = 0
	}
}

func (view *chatWorkspace) Resize(width, height int) (int, int, error) {
	view.mu.Lock()
	defer view.mu.Unlock()
	if width != view.width || height != view.height {
		anchors := view.resizeAnchorsLocked()
		if view.focus == workspaceLedgerPane {
			paneWidth, paneHeight, _, _ := view.paneGeometryLocked()
			row := view.ledgerSelectedRowLocked(max(1, paneWidth-4))
			view.ledger.followSelection = view.ledger.followSelection || row >= view.ledger.offset && row < view.ledger.offset+paneHeight
		}
		view.width, view.height = max(1, width), max(1, height)
		view.restoreResizeAnchorsLocked(anchors)
		view.lastFrame = nil
	}
	columns, rows := view.editorSizeLocked()
	view.conversation.width = columns
	if !view.reviewing {
		view.review.width = columns
	}
	return columns, rows, view.drawLocked()
}

func (view *chatWorkspace) SetProject(project, branch string) error {
	view.mu.Lock()
	defer view.mu.Unlock()
	view.project, view.branch = safeConsoleText(project), safeConsoleText(branch)
	return view.drawLocked()
}

func (view *chatWorkspace) SetActivity(activity chatActivity, detail string) error {
	view.mu.Lock()
	defer view.mu.Unlock()
	activity.Label = safeConsoleText(activity.Label)
	if activity.JobID == 0 {
		view.role, view.model = "", ""
		view.routeJob = 0
		view.routes = nil
	}
	view.activity, view.detail = activity, safeConsoleText(detail)
	return view.drawLocked()
}

func (view *chatWorkspace) Observe(role, model, text string) error {
	view.mu.Lock()
	defer view.mu.Unlock()
	if role != "" {
		view.role = safeConsoleText(role)
	}
	if model != "" {
		view.model = safeConsoleText(model)
	}
	if text != "" {
		view.events = append(view.events, safeConsoleText(text))
		if len(view.events) > 80 {
			view.events = append([]string(nil), view.events[len(view.events)-80:]...)
		}
	}
	return view.drawLocked()
}

func (view *chatWorkspace) elapsedLocked() string {
	if !view.activity.Working || view.activity.StartedAt.IsZero() {
		return ""
	}
	return max(time.Duration(0), time.Since(view.activity.StartedAt)).Truncate(time.Second).String()
}

func (view *chatWorkspace) reviewVisibleLocked() bool {
	return view.focus != workspaceSessionsPane && view.reviewing && view.conversationMode != "research" && view.conversationMode != "ledger"
}
