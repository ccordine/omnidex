package hostbridge

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"

	"github.com/gryph/omnidex/internal/agentstream"
)

func streamCommandNDJSON(w http.ResponseWriter, cmd *exec.Cmd, agent string) {
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := cmd.Start(); err != nil {
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	flusher, _ := w.(http.Flusher)

	errCh := make(chan []byte, 1)
	go func() {
		blob, _ := io.ReadAll(stderr)
		errCh <- blob
	}()

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), agentstream.MaxEventBytes)
	var streamErr error
	for scanner.Scan() {
		event, err := agentstream.DecodeBoundaryLine(scanner.Text())
		if err != nil {
			streamErr = err
			_ = cmd.Process.Kill()
			break
		}
		if event.Agent != agent {
			streamErr = fmt.Errorf("external agent stream agent %q differs from host agent %q", event.Agent, agent)
			_ = cmd.Process.Kill()
			break
		}
		line, err := agentstream.EncodeBoundaryLine(event)
		if err != nil {
			streamErr = err
			_ = cmd.Process.Kill()
			break
		}
		if _, err := io.WriteString(w, line); err != nil {
			_ = cmd.Process.Kill()
			return
		}
		if _, err := io.WriteString(w, "\n"); err != nil {
			_ = cmd.Process.Kill()
			return
		}
		if flusher != nil {
			flusher.Flush()
		}
	}
	if err := scanner.Err(); err != nil && streamErr == nil {
		streamErr = fmt.Errorf("read external agent stream: %w", err)
		_ = cmd.Process.Kill()
	}
	waitErr := cmd.Wait()
	stderrBlob := <-errCh
	if streamErr != nil {
		writeAgentStreamError(w, flusher, agent, streamErr.Error())
		return
	}
	if len(stderrBlob) > 0 {
		message := strings.TrimRight(string(stderrBlob), "\r\n")
		if waitErr != nil && !strings.Contains(message, waitErr.Error()) {
			message = message + " (" + waitErr.Error() + ")"
		}
		writeAgentStreamError(w, flusher, agent, message)
		return
	}
	if waitErr != nil {
		writeAgentStreamError(w, flusher, agent, waitErr.Error())
	}
}

func writeAgentStreamError(w io.Writer, flusher http.Flusher, agent, message string) {
	line, err := agentstream.EncodeBoundaryLine(agentstream.Event{
		Agent: agent, Type: agentstream.EventError, Message: message,
	})
	if err != nil {
		return
	}
	_, _ = io.WriteString(w, line+"\n")
	if flusher != nil {
		flusher.Flush()
	}
}

func writeTempJSONRequest(pattern string, payload any) (string, error) {
	blob, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	file, err := os.CreateTemp("", pattern)
	if err != nil {
		return "", err
	}
	path := file.Name()
	if _, err := file.Write(blob); err != nil {
		file.Close()
		os.Remove(path)
		return "", err
	}
	if err := file.Close(); err != nil {
		os.Remove(path)
		return "", err
	}
	return path, nil
}
