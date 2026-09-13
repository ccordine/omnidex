package workspacetransport

import (
	"fmt"
	"time"

	"github.com/gorilla/websocket"
	"github.com/gryph/omnidex/internal/workspace"
)

const contentChunkBytes = 1024 * 1024

type desiredHeader struct {
	State workspace.DesiredFile `json:"state"`
	Bytes int64                 `json:"bytes"`
}

func readContent(conn *websocket.Conn, size int64) ([]byte, error) {
	if size < 0 || size > workspace.MaxReconciliationFileBytes {
		return nil, fmt.Errorf("workspace file content exceeds its byte bound")
	}
	content := make([]byte, int(size))
	for offset := 0; offset < len(content); {
		count := min(contentChunkBytes, len(content)-offset)
		kind, chunk, err := conn.ReadMessage()
		if err != nil {
			return nil, err
		}
		if kind != websocket.BinaryMessage || len(chunk) != count {
			return nil, fmt.Errorf("workspace content chunk differs from its exact expected size")
		}
		copy(content[offset:offset+count], chunk)
		offset += count
	}
	return content, nil
}

func writeContent(conn *websocket.Conn, content []byte) error {
	if len(content) > workspace.MaxReconciliationFileBytes {
		return fmt.Errorf("workspace file content exceeds its byte bound")
	}
	for offset := 0; offset < len(content); {
		count := min(contentChunkBytes, len(content)-offset)
		if err := conn.SetWriteDeadline(time.Now().Add(operationTimeout)); err != nil {
			return err
		}
		if err := conn.WriteMessage(websocket.BinaryMessage, content[offset:offset+count]); err != nil {
			return err
		}
		offset += count
	}
	return nil
}

func readRequestData(conn *websocket.Conn, req request) (requestData, error) {
	data := requestData{request: req}
	if req.Kind != opPrepare {
		return data, nil
	}
	total := int64(0)
	for range req.DesiredCount {
		var header desiredHeader
		if err := readExact(conn, &header); err != nil {
			return requestData{}, err
		}
		if err := admitContentBytes(&total, header.Bytes); err != nil {
			return requestData{}, err
		}
		if !header.State.Present && header.Bytes != 0 {
			return requestData{}, fmt.Errorf("absent desired file includes content")
		}
		content, err := readContent(conn, header.Bytes)
		if err != nil {
			return requestData{}, err
		}
		if header.State.Present {
			header.State.Content = content
		}
		data.desired = append(data.desired, header.State)
	}
	total = 0
	for range req.ExpectedCount {
		var entry workspace.Entry
		if err := readExact(conn, &entry); err != nil {
			return requestData{}, err
		}
		if err := entry.Validate(entry.Path); err != nil {
			return requestData{}, err
		}
		if entry.Kind != workspace.EntryFile {
			return requestData{}, fmt.Errorf("expected workspace input is not a regular file")
		}
		if err := admitContentBytes(&total, entry.Size); err != nil {
			return requestData{}, err
		}
		content, err := readContent(conn, entry.Size)
		if err != nil {
			return requestData{}, err
		}
		data.expected = append(data.expected, workspace.File{Entry: entry, Content: content})
	}
	return data, nil
}

func writeRequestData(conn *websocket.Conn, data requestData) error {
	if err := writeExact(conn, data.request); err != nil {
		return err
	}
	for _, state := range data.desired {
		if err := writeExact(conn, desiredHeader{State: state, Bytes: int64(len(state.Content))}); err != nil {
			return err
		}
		if err := writeContent(conn, state.Content); err != nil {
			return err
		}
	}
	for _, file := range data.expected {
		if err := writeExact(conn, file.Entry); err != nil {
			return err
		}
		if err := writeContent(conn, file.Content); err != nil {
			return err
		}
	}
	return nil
}

func admitContentBytes(total *int64, size int64) error {
	if size < 0 || size > workspace.MaxReconciliationFileBytes || *total > workspace.MaxReconciliationTotalBytes-size {
		return fmt.Errorf("workspace preparation exceeds its content byte bound")
	}
	*total += size
	return nil
}
