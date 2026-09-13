package client

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/gorilla/websocket"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/projectroot"
	"github.com/gryph/omnidex/internal/workspacetransport"
)

// OpenWorkspace borrows the already retained directory for the connection's
// lifetime. The caller closes it after the connection has ended.
func (client *Client) OpenWorkspace(ctx context.Context, directory *projectroot.DirectoryHandle, clientID string) (*workspacetransport.Local, error) {
	if client == nil || client.httpClient == nil || ctx == nil || directory == nil {
		return nil, fmt.Errorf("Omnidex workspace connection requires client and context")
	}
	if _, err := directory.Identity(); err != nil {
		return nil, err
	}
	root := directory.Path()
	if err := model.ValidateChannelWorkspaceRoot(root); err != nil {
		return nil, err
	}
	if err := projectroot.ValidateClientIdentity(clientID); err != nil {
		return nil, err
	}
	endpoint, err := url.Parse(client.baseURL)
	if err != nil {
		return nil, err
	}
	switch endpoint.Scheme {
	case "http":
		endpoint.Scheme = "ws"
	case "https":
		endpoint.Scheme = "wss"
	default:
		return nil, fmt.Errorf("workspace transport requires HTTP or HTTPS base URL")
	}
	endpoint.Path = strings.TrimRight(endpoint.Path, "/") + "/v1/cli/workspace/ws"
	dialer := websocket.Dialer{HandshakeTimeout: client.httpClient.Timeout}
	conn, response, err := dialer.DialContext(ctx, endpoint.String(), http.Header{})
	if err != nil {
		if response != nil {
			_ = response.Body.Close()
			return nil, &HTTPError{StatusCode: response.StatusCode, Message: fmt.Sprintf("connect client workspace: %v", err)}
		}
		return nil, fmt.Errorf("connect client workspace: %w", err)
	}
	return workspacetransport.Connect(ctx, conn, directory, clientID)
}
