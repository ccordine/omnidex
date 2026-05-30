package hostbridge

import (
	"bufio"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
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
	scanner.Buffer(make([]byte, 0, 64*1024), 4<<20)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		if _, err := w.Write(line); err != nil {
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
	waitErr := cmd.Wait()
	stderrBlob := <-errCh
	if len(stderrBlob) > 0 {
		message := strings.TrimSpace(string(stderrBlob))
		if waitErr != nil && !strings.Contains(message, waitErr.Error()) {
			message = message + " (" + waitErr.Error() + ")"
		}
		payload, _ := json.Marshal(map[string]string{
			"agent":   agent,
			"type":    "error",
			"message": message,
		})
		_, _ = w.Write(payload)
		_, _ = io.WriteString(w, "\n")
		if flusher != nil {
			flusher.Flush()
		}
		return
	}
	if waitErr != nil {
		payload, _ := json.Marshal(map[string]string{
			"agent":   agent,
			"type":    "error",
			"message": waitErr.Error(),
		})
		_, _ = w.Write(payload)
		_, _ = io.WriteString(w, "\n")
		if flusher != nil {
			flusher.Flush()
		}
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
