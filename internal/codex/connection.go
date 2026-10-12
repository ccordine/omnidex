package codex

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"time"

	"github.com/gryph/omnidex/internal/llm"
)

const maximumFrameBytes = llm.MaxExactPreparedProviderResponseBytes

type rpcFrame struct {
	ID     *int            `json:"id,omitempty"`
	Method string          `json:"method,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
	Params json.RawMessage `json:"params,omitempty"`
	Error  *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

type connection struct {
	process   *exec.Cmd
	input     io.WriteCloser
	output    *bufio.Scanner
	sequence  int
	lastFrame json.RawMessage
}

type rpcError struct {
	method  string
	code    int
	message string
}

func (failure *rpcError) Error() string {
	return fmt.Sprintf("Codex RPC %s failed with code %d", failure.method, failure.code)
}

func startConnection(ctx context.Context, command, directory string) (*connection, error) {
	return startConfiguredConnection(ctx, command, directory, false, nil)
}

func startConfiguredConnection(ctx context.Context, command, directory string, agent bool, overrides []string) (*connection, error) {
	enabled := fmt.Sprint(agent)
	arguments := []string{"app-server", "--listen", "stdio://",
		"-c", "features.shell_tool=" + enabled, "-c", "features.unified_exec=" + enabled,
		"-c", "features.apps=false", "-c", "features.plugins=false", "-c", "agents.enabled=false",
		"-c", "features.hooks=false", "-c", "features.multi_agent=false",
		"-c", "project_doc_max_bytes=0", "-c", "web_search=\"disabled\""}
	arguments = append(arguments, overrides...)
	process := exec.CommandContext(ctx, command, arguments...)
	process.Dir = directory
	if agent {
		process.Env = agentProcessEnvironment()
	}
	process.WaitDelay = 5 * time.Second
	configureProcessLifetime(process)
	// Runtime diagnostics must not expose credential/configuration material.
	process.Stderr = io.Discard
	input, err := process.StdinPipe()
	if err != nil {
		return nil, err
	}
	output, err := process.StdoutPipe()
	if err != nil {
		_ = input.Close()
		return nil, err
	}
	if err := process.Start(); err != nil {
		_ = input.Close()
		return nil, err
	}
	scanner := bufio.NewScanner(output)
	scanner.Buffer(make([]byte, 64<<10), maximumFrameBytes)
	return &connection{process: process, input: input, output: scanner}, nil
}

func (connection *connection) close() {
	_ = connection.input.Close()
	_ = interruptProcess(connection.process)
	done := make(chan struct{})
	go func() { _ = connection.process.Wait(); close(done) }()
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	select {
	case <-done:
	case <-timer.C:
		_ = connection.process.Cancel()
		<-done
	}
}

func (connection *connection) send(method string, params any, id *int) error {
	encoded, err := json.Marshal(struct {
		ID     *int   `json:"id,omitempty"`
		Method string `json:"method"`
		Params any    `json:"params"`
	}{id, method, params})
	if err != nil {
		return err
	}
	encoded = append(encoded, '\n')
	count, err := connection.input.Write(encoded)
	if err != nil {
		return err
	}
	if count != len(encoded) {
		return io.ErrShortWrite
	}
	return nil
}

func (connection *connection) next() (rpcFrame, error) {
	return connection.readFrame(false)
}

func (connection *connection) nextAgent() (rpcFrame, error) { return connection.readFrame(true) }

func (connection *connection) readFrame(agent bool) (rpcFrame, error) {
	if !connection.output.Scan() {
		if err := connection.output.Err(); err != nil {
			return rpcFrame{}, err
		}
		return rpcFrame{}, io.ErrUnexpectedEOF
	}
	connection.lastFrame = append(json.RawMessage(nil), connection.output.Bytes()...)
	var frame rpcFrame
	if err := json.Unmarshal(connection.lastFrame, &frame); err != nil {
		return frame, err
	}
	if frame.ID != nil && frame.Method != "" && !(agent && frame.Method == "item/tool/call") {
		return frame, fmt.Errorf("Codex attempted a client tool or approval request")
	}
	return frame, nil
}

func (connection *connection) call(method string, params, result any) (json.RawMessage, error) {
	connection.sequence++
	id := connection.sequence
	if err := connection.send(method, params, &id); err != nil {
		return nil, err
	}
	for {
		frame, err := connection.next()
		if err != nil {
			return nil, err
		}
		if frame.ID == nil {
			continue
		}
		if *frame.ID != id {
			return nil, fmt.Errorf("Codex returned a mismatched RPC response")
		}
		if frame.Error != nil {
			return nil, &rpcError{method: method, code: frame.Error.Code, message: frame.Error.Message}
		}
		if result != nil {
			if err := json.Unmarshal(frame.Result, result); err != nil {
				return nil, err
			}
		}
		return append(json.RawMessage(nil), connection.lastFrame...), nil
	}
}
