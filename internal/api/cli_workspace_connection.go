package api

import (
	"context"
	"log"
	"net/http"

	"github.com/gorilla/websocket"
	"github.com/gryph/omnidex/internal/workspacetransport"
)

// WorkspaceConnections supplies the worker with the same live transports that
// the API attests. Persisted jobs retain the exact connection binding.
func (s *Server) WorkspaceConnections() *workspacetransport.Hub { return s.workspaceConnections }

func (s *Server) handleCLIWorkspaceConnection(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := validateExactQuery(r); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if !realtimeOriginAllowed(r) {
		writeError(w, http.StatusForbidden, "workspace connection origin is not allowed")
		return
	}
	upgrader := websocket.Upgrader{CheckOrigin: realtimeOriginAllowed}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	stop := context.AfterFunc(s.lifecycleContext, cancel)
	defer stop()
	if err := s.workspaceConnections.Serve(ctx, conn); err != nil {
		log.Printf("CLI workspace connection ended remote=%q: %v", r.RemoteAddr, err)
	}
}
