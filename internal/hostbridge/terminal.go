package hostbridge

import (
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"

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

	conn, err := terminalUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}

	shell := resolveShell()
	cmd := buildShellCommand(shell, abs)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)})
	if err != nil {
		_ = conn.WriteMessage(websocket.TextMessage, []byte("\r\n\x1b[31mfailed to start shell:\x1b[0m "+err.Error()+"\r\n"))
		_ = conn.Close()
		return
	}

	runTerminalSession(conn, cmd, ptmx)
}

func resolveShell() string {
	shell := strings.TrimSpace(os.Getenv("SHELL"))
	if shell == "" {
		return "/bin/bash"
	}
	return shell
}

func buildShellCommand(shell, cwd string) *exec.Cmd {
	shell = strings.TrimSpace(shell)
	if shell == "" {
		shell = "/bin/bash"
	}

	args := shellInvocationArgs(filepath.Base(shell))
	cmd := exec.Command(shell, args...)
	cmd.Dir = cwd
	cmd.Env = terminalEnv(cwd, shell)
	return cmd
}

func shellInvocationArgs(baseName string) []string {
	switch strings.ToLower(strings.TrimSpace(baseName)) {
	case "fish":
		return []string{"-i"}
	case "zsh":
		return []string{"-il"}
	case "bash", "sh", "dash", "ksh", "nu", "nushell":
		return []string{"-il"}
	default:
		return []string{"-i"}
	}
}

func terminalEnv(cwd, shell string) []string {
	pairs := map[string]string{}
	for _, kv := range os.Environ() {
		if i := strings.IndexByte(kv, '='); i > 0 {
			pairs[kv[:i]] = kv[i+1:]
		}
	}

	if u, err := user.Current(); err == nil {
		if u.Username != "" {
			pairs["USER"] = u.Username
			pairs["LOGNAME"] = u.Username
		}
		if u.HomeDir != "" {
			pairs["HOME"] = u.HomeDir
		}
	}
	if home, err := os.UserHomeDir(); err == nil && strings.TrimSpace(home) != "" {
		pairs["HOME"] = home
	}

	pairs["SHELL"] = shell
	pairs["PWD"] = cwd
	pairs["TERM"] = "xterm-256color"
	pairs["COLORTERM"] = "truecolor"
	pairs["OMNI_TERMINAL"] = "1"

	out := make([]string, 0, len(pairs))
	for key, value := range pairs {
		out = append(out, key+"="+value)
	}
	return out
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
	errc := make(chan error, 2)

	go func() {
		buf := make([]byte, 32*1024)
		for {
			n, err := ptmx.Read(buf)
			if n > 0 {
				writeMu.Lock()
				werr := conn.WriteMessage(websocket.BinaryMessage, buf[:n])
				writeMu.Unlock()
				if werr != nil {
					errc <- werr
					return
				}
			}
			if err != nil {
				errc <- err
				return
			}
		}
	}()

	go func() {
		for {
			msgType, data, err := conn.ReadMessage()
			if err != nil {
				errc <- err
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
				errc <- err
				return
			}
		}
	}()

	<-errc
}
