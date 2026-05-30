package api

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/gorilla/websocket"
)

var terminalProxyUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

func (s *Server) resolveTerminalCWD(ctx context.Context, raw string) (string, error) {
	if client := s.hostBridgeClient(); client != nil {
		return resolveHostBridgeProjectPath(ctx, client, raw)
	}
	return s.validateProjectLocation(ctx, raw)
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
