package worker

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/gryph/omnidex/internal/artifacts"
	"github.com/gryph/omnidex/internal/specialists"
	toolruntime "github.com/gryph/omnidex/internal/tools"
)

func implementationFileCheckCall(item artifacts.ImplementationWorkItem) (toolruntime.Call, bool) {
	if filepath.ToSlash(filepath.Clean(item.Path)) == "go.mod" {
		return toolruntime.Call{Name: "command.run", Input: map[string]any{
			"program": "go", "args": []string{"list", "-m"}, "timeout_seconds": 60,
		}}, true
	}
	extension := strings.ToLower(filepath.Ext(item.Path))
	switch extension {
	case ".go":
		directory := filepath.ToSlash(filepath.Dir(item.Path))
		packagePath := "."
		if directory != "." {
			packagePath = "./" + directory
		}
		return toolruntime.Call{Name: "command.run", Input: map[string]any{
			"program": "go", "args": []string{"test", packagePath}, "timeout_seconds": 180,
		}}, true
	case ".js", ".cjs", ".mjs":
		return toolruntime.Call{Name: "command.run", Input: map[string]any{
			"program": "node", "args": []string{"--check", filepath.ToSlash(filepath.Clean(item.Path))}, "timeout_seconds": 60,
		}}, true
	default:
		return toolruntime.Call{}, false
	}
}

func (r *nativeRuntimeV3) checkWrittenImplementationFile(
	ledger *artifacts.ImplementationLedgerArtifact,
	index int,
	spec specialists.Spec,
) (*subtaskToolRecord, error) {
	item := &ledger.Items[index]
	call, required := implementationFileCheckCall(*item)
	if !required {
		return nil, nil
	}
	record, err := r.executeSubtaskToolCall(spec, ledger.Objective, []string{"workspace.write", "command.run"}, call)
	if err != nil {
		if toolruntime.IsCallRejected(err) {
			return &record, r.retryImplementationItem(ledger, index, "DETERMINISTIC FILE CHECK REJECTED: "+err.Error())
		}
		return &record, err
	}
	if !toolResultSucceeded(record.Result) {
		failure := implementationVerificationFailure(record)
		return &record, r.retryImplementationItem(ledger, index, "DETERMINISTIC FILE CHECK FAILED.\n"+failure)
	}
	r.svc.emitStepEvent(r.claim.Step.ID, "implementation_file_check_completed", fmt.Sprintf(
		"id=%s path=%s command=%s",
		safeLine(item.ID, "unknown"), safeLine(item.Path, "unknown"), safeLine(subtaskCommandText(record.Call), "unknown"),
	))
	return &record, nil
}
