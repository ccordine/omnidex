package api

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/gryph/omnidex/internal/hostbridge"
)

type terminalPreflightResponse struct {
	Mode      string `json:"mode"`
	WSURL     string `json:"ws_url"`
	Workspace string `json:"workspace,omitempty"`
	Hint      string `json:"hint,omitempty"`
}

func (s *Server) handleHostTerminalPreflight(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	payload, err := s.prepareTerminalConnection(r)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	resp := terminalPreflightResponse{
		Mode:      payload.mode,
		WSURL:     payload.wsURL,
		Workspace: payload.cwd,
	}
	if payload.mode == "direct" {
		resp.Hint = "Terminal connects directly to the host bridge over plain ws:// (no SSL). Open the UI via http://, not https://."
	} else {
		resp.Hint = "Terminal connects through core over plain ws:// when the UI uses http://."
	}
	writeJSON(w, http.StatusOK, resp)
}

type terminalConnectionPayload struct {
	mode     string
	wsURL    string
	cwd      string
	bridgeURL string
}

func (s *Server) prepareTerminalConnection(r *http.Request) (terminalConnectionPayload, error) {
	client := s.hostBridgeClient()
	if client == nil {
		return terminalConnectionPayload{}, fmt.Errorf("host bridge unavailable: run `omni host serve --listen 0.0.0.0:8091` and set HOST_AGENT_URL in core")
	}

	projectID, err := s.resolveProjectID(r)
	if err != nil {
		return terminalConnectionPayload{}, err
	}

	setupCtx, setupCancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer setupCancel()

	project, err := s.repo.GetProject(setupCtx, projectID)
	if err != nil {
		return terminalConnectionPayload{}, fmt.Errorf("project not found")
	}

	cwd, err := s.resolveTerminalCWD(setupCtx, project.Location)
	if err != nil {
		return terminalConnectionPayload{}, err
	}

	bridgeBase := strings.TrimRight(strings.TrimSpace(s.hostAgentURL), "/")
	if resolved, resolveErr := hostbridge.ResolveReachableURL(setupCtx, bridgeBase, s.hostAgentToken, 4*time.Second); resolveErr == nil && resolved != "" {
		bridgeBase = resolved
	}

	bridgeURL, err := buildBridgeTerminalWSURL(bridgeBase, cwd, r.URL.Query())
	if err != nil {
		return terminalConnectionPayload{}, err
	}

	if err := probeBridgeTerminal(setupCtx, bridgeURL, s.hostAgentToken); err != nil {
		return terminalConnectionPayload{}, err
	}

	query := r.URL.Query()
	if terminalUseDirectBridge(s.coreURLDefault) {
		publicBase := publicBridgeWSBase(s.coreURLDefault)
		directURL, err := buildDirectTerminalWSURL(publicBase, cwd, query, strings.TrimSpace(s.hostAgentToken))
		if err != nil {
			return terminalConnectionPayload{}, err
		}
		return terminalConnectionPayload{
			mode:      "direct",
			wsURL:     directURL,
			cwd:       cwd,
			bridgeURL: bridgeURL,
		}, nil
	}

	proxyURL := buildProxyTerminalWSURL(coreWSBase(r, s.coreURLDefault), query)
	return terminalConnectionPayload{
		mode:      "proxy",
		wsURL:     proxyURL,
		cwd:       cwd,
		bridgeURL: bridgeURL,
	}, nil
}

func probeBridgeTerminal(ctx context.Context, bridgeURL, token string) error {
	header := http.Header{}
	if strings.TrimSpace(token) != "" {
		header.Set("Authorization", "Bearer "+strings.TrimSpace(token))
	}
	conn, resp, err := websocket.DefaultDialer.DialContext(ctx, bridgeURL, header)
	if err != nil {
		return fmt.Errorf("host bridge terminal probe failed: %s", terminalBridgeDialError(err, resp))
	}
	_ = conn.WriteMessage(websocket.TextMessage, []byte(""))
	_ = conn.Close()
	return nil
}

func (s *Server) handleHostTerminalWS(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	payload, err := s.prepareTerminalConnection(r)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	if payload.mode == "direct" {
		writeJSON(w, http.StatusConflict, map[string]any{
			"error":   "terminal uses direct host bridge websocket; call /v1/host/terminal/preflight instead",
			"ws_url":  payload.wsURL,
			"mode":    "direct",
			"workspace": payload.cwd,
		})
		return
	}

	header := http.Header{}
	if token := strings.TrimSpace(s.hostAgentToken); token != "" {
		header.Set("Authorization", "Bearer "+token)
	}

	bridgeConn, resp, err := websocket.DefaultDialer.DialContext(r.Context(), payload.bridgeURL, header)
	if err != nil {
		writeError(w, http.StatusBadGateway, terminalBridgeDialError(err, resp))
		return
	}

	clientConn, err := terminalProxyUpgrader.Upgrade(w, r, nil)
	if err != nil {
		_ = bridgeConn.Close()
		return
	}

	proxyTerminalWebSocket(clientConn, bridgeConn)
}

func (s *Server) handleUIRuntimeConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"core_url":              strings.TrimSpace(s.coreURLDefault),
		"ws_base":               coreWSBase(r, s.coreURLDefault),
		"terminal_direct":       terminalUseDirectBridge(s.coreURLDefault),
		"host_bridge_public_ws": publicBridgeWSBase(s.coreURLDefault),
		"plain_http_ok":         true,
	})
}

func terminalBridgeDialError(err error, resp *http.Response) string {
	message := strings.TrimSpace(err.Error())
	if resp != nil && resp.Body != nil {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		_ = resp.Body.Close()
		snippet := strings.TrimSpace(string(body))
		if snippet != "" && !strings.Contains(message, snippet) {
			message = strings.TrimSpace(message + ": " + snippet)
		}
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
		return "host bridge terminal handshake failed — the bridge is reachable but did not upgrade to websocket. Restart the host bridge (`omni host serve --listen 0.0.0.0:8091`) and rebuild core. This is not an SSL issue; use plain http:// and ws:// on your LAN."
	}
	return message
}
