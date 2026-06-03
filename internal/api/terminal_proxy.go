package api

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

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

	const (
		readIdleTimeout = 5 * time.Minute
		pingInterval    = 30 * time.Second
		writeTimeout    = 10 * time.Second
	)

	type guardedConn struct {
		conn *websocket.Conn
		mu   sync.Mutex
	}
	writeMessage := func(dst *guardedConn, msgType int, msg []byte) error {
		dst.mu.Lock()
		defer dst.mu.Unlock()
		_ = dst.conn.SetWriteDeadline(time.Now().Add(writeTimeout))
		return dst.conn.WriteMessage(msgType, msg)
	}
	for _, conn := range []*websocket.Conn{clientConn, bridgeConn} {
		_ = conn.SetReadDeadline(time.Now().Add(readIdleTimeout))
		conn.SetPongHandler(func(string) error {
			return conn.SetReadDeadline(time.Now().Add(readIdleTimeout))
		})
	}

	client := &guardedConn{conn: clientConn}
	bridge := &guardedConn{conn: bridgeConn}
	errc := make(chan error, 2)
	copyMessages := func(dst *guardedConn, src *websocket.Conn) {
		for {
			msgType, msg, err := src.ReadMessage()
			if err != nil {
				errc <- err
				return
			}
			if err := writeMessage(dst, msgType, msg); err != nil {
				errc <- err
				return
			}
		}
	}

	go copyMessages(bridge, clientConn)
	go copyMessages(client, bridgeConn)
	ping := time.NewTicker(pingInterval)
	defer ping.Stop()
	for {
		select {
		case <-ping.C:
			if err := writeMessage(client, websocket.PingMessage, []byte("ping")); err != nil {
				return
			}
			if err := writeMessage(bridge, websocket.PingMessage, []byte("ping")); err != nil {
				return
			}
		case <-errc:
			return
		}
	}
}
