package codex

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/model"
)

const maxAgentCheckOutputBytes = 1 << 20

type checkActivityOutput struct {
	raw     string
	pending []byte
}
type checkActivityStream struct {
	call          codingagent.Assignment
	check         codingagent.Check
	processID     string
	sink          func(model.AgentActivity) error
	outputs       [2]checkActivityOutput
	capturedBytes int
	started       bool
}

func (stream *checkActivityStream) base(kind model.AgentActivityKind, status model.AgentActivityStatus) model.AgentActivity {
	activity := model.AgentActivity{Schema: 1, Source: model.AgentActivityVerification, Kind: kind, Status: status, CheckID: stream.check.ID, Role: string(stream.call.Role)}
	if stream.call.Task != nil {
		activity.TaskID = stream.call.Task.ID
	}
	return activity
}

func (stream *checkActivityStream) publish(activity model.AgentActivity) error {
	if err := activity.Validate(); err != nil {
		return err
	}
	if stream.sink != nil {
		return stream.sink(activity)
	}
	return nil
}

func (stream *checkActivityStream) command(status model.AgentActivityStatus, exitCode *int) error {
	activity := stream.base(model.AgentActivityCommand, status)
	activity.Argv = append([]string(nil), stream.check.Argv...)
	activity.CWD, activity.ExitCode = stream.call.Root, exitCode
	activity = model.BoundAgentActivityCommand(activity)
	return stream.publish(activity)
}

func (stream *checkActivityStream) notification(frame rpcFrame) error {
	if frame.Method != "command/exec/outputDelta" {
		return nil
	}
	var params struct {
		ProcessID   string `json:"processId"`
		Stream      string `json:"stream"`
		DeltaBase64 string `json:"deltaBase64"`
	}
	if err := json.Unmarshal(frame.Params, &params); err != nil {
		return err
	}
	if params.ProcessID != stream.processID || !stream.started {
		return fmt.Errorf("verification output belongs to another command")
	}
	index := 0
	switch params.Stream {
	case "stdout":
	case "stderr":
		index = 1
	default:
		return fmt.Errorf("verification output has unknown stream")
	}
	data, err := base64.StdEncoding.DecodeString(params.DeltaBase64)
	if err != nil {
		return fmt.Errorf("decode verification output: %w", err)
	}
	return stream.output(index, data, false)
}

func (stream *checkActivityStream) output(index int, data []byte, final bool) error {
	stream.capturedBytes += len(data)
	if stream.capturedBytes > maxAgentCheckOutputBytes {
		return fmt.Errorf("verification output exceeds its native byte boundary")
	}
	output := &stream.outputs[index]
	output.raw += string(data)
	output.pending = append(output.pending, data...)
	var text strings.Builder
	for len(output.pending) > 0 {
		if !final && !utf8.FullRune(output.pending) {
			break
		}
		character, size := utf8.DecodeRune(output.pending)
		if character == utf8.RuneError && size == 1 || character == 0 {
			fmt.Fprintf(&text, "\\x%02x", output.pending[0])
		} else {
			text.WriteRune(character)
		}
		output.pending = output.pending[size:]
	}
	activity := stream.base(model.AgentActivityCommandOutput, model.AgentActivityDelta)
	activity.Text = text.String()
	return publishAgentActivityText(activity, stream.publish)
}

func (stream *checkActivityStream) completeOutput(stdout, stderr string) error {
	for index, final := range []string{stdout, stderr} {
		prefix := stream.outputs[index].raw
		// Streaming command/exec completion may carry only its exit status.
		// An empty aggregate leaves the process-bound captured bytes intact.
		if final != "" && !strings.HasPrefix(final, prefix) {
			return fmt.Errorf("verification completion contradicts its streamed output prefix")
		}
		var remainder []byte
		if final != "" {
			remainder = []byte(final[len(prefix):])
		}
		if err := stream.output(index, remainder, true); err != nil {
			return err
		}
	}
	return nil
}

func (client *Client) executeAgentCheck(connection *connection, scope codingagent.Assignment, check codingagent.Check, sink func(model.AgentActivity) error) (result codingagent.CheckResult, err error) {
	stream := checkActivityStream{call: scope, check: check, processID: check.ID, sink: sink}
	defer func() {
		if err != nil && stream.started {
			if publicationErr := stream.command(model.AgentActivityFailed, nil); publicationErr != nil {
				err = fmt.Errorf("%w; verification failure publication: %v", err, publicationErr)
			}
		}
	}()
	var response struct {
		ExitCode *int   `json:"exitCode"`
		Stdout   string `json:"stdout"`
		Stderr   string `json:"stderr"`
	}
	_, err = connection.callObserved("command/exec", map[string]any{"command": check.Argv, "cwd": scope.Root,
		"sandboxPolicy": map[string]any{"type": "dangerFullAccess"},
		"timeoutMs":     int64(client.timeout / time.Millisecond), "outputBytesCap": maxAgentCheckOutputBytes,
		"processId": stream.processID, "streamStdoutStderr": true}, &response,
		func() error { stream.started = true; return stream.command(model.AgentActivityStarted, nil) }, stream.notification)
	if err != nil {
		return result, err
	}
	if response.ExitCode == nil {
		return result, fmt.Errorf("verification check %s lacks its actual exit status", check.ID)
	}
	if err = stream.completeOutput(response.Stdout, response.Stderr); err != nil {
		return result, err
	}
	status := model.AgentActivityCompleted
	if *response.ExitCode != 0 {
		status = model.AgentActivityFailed
	}
	if err = stream.command(status, response.ExitCode); err != nil {
		return result, err
	}
	stream.started = false
	return codingagent.CheckResult{ID: check.ID, ExitCode: *response.ExitCode, Output: stream.outputs[0].raw + stream.outputs[1].raw}, nil
}
