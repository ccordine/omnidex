package experiment

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"time"
)

// The Docker CLI owns daemon transport, credentials, and context selection.
// dial-stdio exposes that same connection for the Engine exec API; no host
// socket path or alternate Docker endpoint is inferred here.
func dockerAPI(ctx context.Context, method, target string, payload, output any, consume func(*http.Response, *bufio.Reader, io.WriteCloser) error) (resultErr error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if payload == nil {
		body = nil
	}
	if len(body) > 256*1024 {
		return fmt.Errorf("Docker API request exceeds its byte bound")
	}
	request, err := http.NewRequest(method, "http://docker"+target, bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Connection", "close")
	if consume != nil {
		request.Header.Set("Connection", "Upgrade")
		request.Header.Set("Upgrade", "tcp")
	}
	parent := ctx
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	process := exec.CommandContext(ctx, "docker", "system", "dial-stdio")
	process.WaitDelay = time.Second
	stderr := &boundedBuffer{cancel: cancel}
	process.Stderr = stderr
	stdin, err := process.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := process.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return err
	}
	if err := process.Start(); err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		return fmt.Errorf("open Docker API transport: %w", err)
	}
	defer func() {
		_ = stdin.Close()
		if resultErr != nil {
			cancel()
		}
		waitErr := process.Wait()
		resultErr = errors.Join(resultErr, dockerOperationError("API transport", stderr.Bytes(), waitErr))
		if parent.Err() != nil {
			resultErr = errors.Join(resultErr, parent.Err())
		}
	}()
	if err := request.Write(stdin); err != nil {
		return fmt.Errorf("write Docker API request: %w", err)
	}
	if consume == nil {
		if err := stdin.Close(); err != nil {
			return err
		}
	}
	reader := bufio.NewReader(stdout)
	response, err := http.ReadResponse(reader, request)
	if err != nil {
		return fmt.Errorf("read Docker API response: %w", err)
	}
	defer response.Body.Close()
	if consume != nil && response.StatusCode == http.StatusSwitchingProtocols {
		return consume(response, reader, stdin)
	}
	content, err := io.ReadAll(io.LimitReader(response.Body, MaxStreamBytes+1))
	if err != nil {
		return err
	}
	if len(content) > MaxStreamBytes {
		return fmt.Errorf("Docker API response exceeds its byte bound")
	}
	if consume != nil || response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("Docker API %s %s returned HTTP %d: %.4000s", method, target, response.StatusCode, content)
	}
	if output != nil {
		if err := json.Unmarshal(content, output); err != nil {
			return fmt.Errorf("decode Docker API response: %w", err)
		}
	}
	return nil
}
