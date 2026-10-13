package codex

import (
	"encoding/json"
	"strings"

	"github.com/gryph/omnidex/internal/codingagent"
)

// mapAgentPlanResult canonicalizes only the harmless leading-dot spelling that
// native Plan commonly uses for workspace-relative files. The portable contract
// and its existing scope validator still reject absolute paths, traversal,
// protected locations, unknown fields and every other malformed value.
func mapAgentPlanResult(call codingagent.Assignment, content json.RawMessage) (json.RawMessage, error) {
	if call.Role != codingagent.Plan {
		return content, nil
	}
	var document map[string]json.RawMessage
	if err := json.Unmarshal(content, &document); err != nil {
		return content, nil
	}
	var tasks []map[string]json.RawMessage
	if err := json.Unmarshal(document["tasks"], &tasks); err != nil {
		return content, nil
	}
	for _, task := range tasks {
		for _, field := range []string{"read_files", "write_files"} {
			var paths []string
			if err := json.Unmarshal(task[field], &paths); err != nil {
				continue
			}
			for index, value := range paths {
				for strings.HasPrefix(value, "./") {
					value = strings.TrimPrefix(value, "./")
				}
				paths[index] = value
			}
			encoded, err := json.Marshal(paths)
			if err != nil {
				return nil, err
			}
			task[field] = encoded
		}
	}
	encodedTasks, err := json.Marshal(tasks)
	if err != nil {
		return nil, err
	}
	document["tasks"] = encodedTasks
	return json.Marshal(document)
}
