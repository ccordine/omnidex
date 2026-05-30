package api

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/gryph/omnidex/internal/hostbridge"
)

var terminalProxyUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

func (s *Server) handleHostTerminalWS(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	client := s.hostBridgeClient()
	if client == nil {
		writeError(w, http.StatusServiceUnavailable, "host bridge unavailable: run `omni host serve` on the host and set HOST_AGENT_URL")
		return
	}

	projectID, err := s.resolveProjectID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	setupCtx, setupCancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer setupCancel()

	project, err := s.repo.GetProject(setupCtx, projectID)
	if err != nil {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}

	cwd, err := s.resolveTerminalCWD(setupCtx, project.Location)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	bridgeBase := strings.TrimRight(strings.TrimSpace(s.hostAgentURL), "/")
	if resolved, resolveErr := hostbridge.ResolveReachableURL(setupCtx, bridgeBase, s.hostAgentToken, 4*time.Second); resolveErr == nil && resolved != "" {
		bridgeBase = resolved
	}

	bridgeURL, err := buildBridgeTerminalWSURL(bridgeBase, cwd, r.URL.Query())
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	clientConn, err := terminalProxyUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}

	header := http.Header{}
	if token := strings.TrimSpace(s.hostAgentToken); token != "" {
		header.Set("Authorization", "Bearer "+token)
	}

	bridgeConn, resp, err := websocket.DefaultDialer.DialContext(r.Context(), bridgeURL, header)
	if err != nil {
		message := terminalBridgeDialError(err, resp)
		_ = clientConn.WriteMessage(websocket.TextMessage, []byte("\r\n\x1b[31m"+message+"\x1b[0m\r\n"))
		_ = clientConn.Close()
		return
	}

	proxyTerminalWebSocket(clientConn, bridgeConn)
}

func (s *Server) resolveTerminalCWD(ctx context.Context, raw string) (string, error) {
	if client := s.hostBridgeClient(); client != nil {
		return resolveHostBridgeProjectPath(ctx, client, raw)
	}
	return s.validateProjectLocation(ctx, raw)
}

func terminalBridgeDialError(err error, resp *http.Response) string {
	message := strings.TrimSpace(err.Error())
	if resp != nil {
		switch resp.StatusCode {
		case http.StatusUnauthorized:
			return "host bridge rejected the terminal connection (401). Check HOST_AGENT_TOKEN matches on core and omni host serve."
		case http.StatusForbidden:
			return "host bridge rejected the project directory (403). Confirm the path is under your home directory or HOST_BROWSE_ROOTS."
		case http.StatusBadRequest:
			return "host bridge rejected the terminal request (400). The project directory may not exist on the host."
		default:
			message = fmt.Sprintf("bridge terminal dial failed (%d): %s", resp.StatusCode, message)
		}
	}
	if strings.Contains(strings.ToLower(message), "bad handshake") {
		return "host bridge terminal handshake failed. Ensure omni host serve is running, reachable from core, and the project path exists on the host."
	}
	return message
}

func buildBridgeTerminalWSURL(base, cwd string, query url.Values) (string, error) {
	parsed, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http":
		parsed.Scheme = "ws"
	case "https":
		parsed.Scheme = "wss"
	default:
		return "", fmt.Errorf("unsupported host bridge URL scheme")
	}

	params := url.Values{}
	params.Set("cwd", cwd)
	if cols := strings.TrimSpace(query.Get("cols")); cols != "" {
		params.Set("cols", cols)
	}
	if rows := strings.TrimSpace(query.Get("rows")); rows != "" {
		params.Set("rows", rows)
	}
	parsed.Path = "/v1/terminal/ws"
	parsed.RawQuery = params.Encode()
	return parsed.String(), nil
}

func proxyTerminalWebSocket(clientConn, bridgeConn *websocket.Conn) {
	defer clientConn.Close()
	defer bridgeConn.Close()

	errc := make(chan error, 2)
	copyMessages := func(dst, src *websocket.Conn) {
		for {
			msgType, msg, err := src.ReadMessage()
			if err != nil {
				errc <- err
				return
			}
			if err := dst.WriteMessage(msgType, msg); err != nil {
				errc <- err
				return
			}
		}
	}

	go copyMessages(bridgeConn, clientConn)
	go copyMessages(clientConn, bridgeConn)
	<-errc
}
