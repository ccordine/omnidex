package hostbridge

import (
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/creack/pty"
	"github.com/gorilla/websocket"
)

var terminalUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

func (s *Server) handleTerminalWS(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.authorize(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	cwd := strings.TrimSpace(r.URL.Query().Get("cwd"))
	if cwd == "" {
		writeError(w, http.StatusBadRequest, "cwd is required")
		return
	}
	abs, err := filepath.Abs(cwd)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	abs = filepath.Clean(abs)
	if err := ensureBrowseAllowed(abs, BrowseOptions{}); err != nil {
		writeError(w, http.StatusForbidden, err.Error())
		return
	}
	stat, err := os.Stat(abs)
	if err != nil || !stat.IsDir() {
		writeError(w, http.StatusBadRequest, "cwd must be an existing directory")
		return
	}

	cols := parseTerminalSize(r.URL.Query().Get("cols"), 120)
	rows := parseTerminalSize(r.URL.Query().Get("rows"), 32)

	shell := strings.TrimSpace(os.Getenv("SHELL"))
	if shell == "" {
		shell = "/bin/bash"
	}

	cmd := exec.Command(shell, "-l")
	cmd.Dir = abs
	cmd.Env = append(os.Environ(), "TERM=xterm-256color", "COLORTERM=truecolor")

	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	conn, err := terminalUpgrader.Upgrade(w, r, nil)
	if err != nil {
		_ = ptmx.Close()
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		return
	}

	runTerminalSession(conn, cmd, ptmx)
}

func parseTerminalSize(raw string, fallback int) int {
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || value < 1 {
		return fallback
	}
	if value > 500 {
		return 500
	}
	return value
}

func runTerminalSession(conn *websocket.Conn, cmd *exec.Cmd, ptmx *os.File) {
	defer conn.Close()
	defer func() {
		_ = ptmx.Close()
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		_ = cmd.Wait()
	}()

	var writeMu sync.Mutex
	done := make(chan struct{})

	go func() {
		buf := make([]byte, 32*1024)
		for {
			n, err := ptmx.Read(buf)
			if n > 0 {
				writeMu.Lock()
				werr := conn.WriteMessage(websocket.BinaryMessage, buf[:n])
				writeMu.Unlock()
				if werr != nil {
					close(done)
					return
				}
			}
			if err != nil {
				close(done)
				return
			}
		}
	}()

	for {
		select {
		case <-done:
			return
		default:
		}

		msgType, data, err := conn.ReadMessage()
		if err != nil {
			return
		}

		if msgType == websocket.TextMessage && len(data) > 0 && data[0] == '{' {
			var ctrl struct {
				Type string `json:"type"`
				Cols int    `json:"cols"`
				Rows int    `json:"rows"`
			}
			if json.Unmarshal(data, &ctrl) == nil && ctrl.Type == "resize" && ctrl.Cols > 0 && ctrl.Rows > 0 {
				_ = pty.Setsize(ptmx, &pty.Winsize{
					Cols: uint16(ctrl.Cols),
					Rows: uint16(ctrl.Rows),
				})
				continue
			}
		}

		if _, err := ptmx.Write(data); err != nil {
			return
		}
	}
}
