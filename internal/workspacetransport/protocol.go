package workspacetransport

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/gorilla/websocket"
	"github.com/gryph/omnidex/internal/exactjson"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/projectroot"
)

const (
	// A full directory page can contain 256 paths of 4096 bytes each,
	// with up to six JSON bytes for each escaped path byte.
	MaxFrameBytes    = 8 * 1024 * 1024
	operationTimeout = 10 * time.Second
	transferTimeout  = 2 * time.Minute
)

type binding struct {
	Root     string `json:"root"`
	Identity string `json:"identity"`
}

func (value binding) validate() error {
	if err := model.ValidateChannelWorkspaceRoot(value.Root); err != nil {
		return err
	}
	return projectroot.ValidateClientWorkspaceIdentity(value.Identity)
}

type ready struct {
	Identity string `json:"identity"`
}

func readExact(conn *websocket.Conn, target any) error {
	kind, data, err := conn.ReadMessage()
	if err != nil {
		return err
	}
	if kind != websocket.TextMessage || len(data) > MaxFrameBytes {
		return fmt.Errorf("workspace connection requires bounded text frames")
	}
	if err := exactjson.ValidateObject(data, target, "workspace frame"); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("decode workspace connection frame: %w", err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return fmt.Errorf("workspace connection frame must contain exactly one value")
	}
	return nil
}

func writeExact(conn *websocket.Conn, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if len(data) > MaxFrameBytes {
		return fmt.Errorf("workspace text frame exceeds its byte bound")
	}
	if err := conn.SetWriteDeadline(time.Now().Add(operationTimeout)); err != nil {
		return err
	}
	return conn.WriteMessage(websocket.TextMessage, data)
}
