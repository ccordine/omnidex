package codingagent

import (
	"fmt"
	"strings"
)

// CompletionRecap projects recorded results; it never grants completion authority.
func CompletionRecap(entries []Entry, maxBytes int) (string, error) {
	if len(entries) == 0 || entries[len(entries)-1].Phase != "overall_accepted" {
		return "", fmt.Errorf("completion recap requires accepted overall verification")
	}
	var workload *Workload
	files := []string{}
	checks := len(entries[len(entries)-1].Checks)
	accepted := map[string]int{}
	for _, entry := range entries {
		switch entry.Phase {
		case "plan_frozen":
			workload = entry.Workload
		case "files_written":
			files = append(files, entry.WrittenFiles...)
		case "task_accepted":
			accepted[entry.TaskID] = len(entry.Checks)
		case "task_reopened", "task_reviewed":
			delete(accepted, entry.TaskID)
		}
	}
	if workload == nil || len(workload.Tasks) == 0 {
		return "", fmt.Errorf("completion recap requires the frozen tasks")
	}
	for _, task := range workload.Tasks {
		count, exists := accepted[task.ID]
		if !exists {
			return "", fmt.Errorf("completion recap task %s is not accepted", task.ID)
		}
		checks += count
	}
	files = uniqueFiles(files)
	header := fmt.Sprintf("Tasks completed: %d. Checks passed: %d. Independent task and overall reviews passed.\n", len(workload.Tasks), checks)
	if maxBytes < len(header)+96 {
		return "", fmt.Errorf("completion recap output boundary is too small")
	}
	var output strings.Builder
	output.WriteString(header)
	for index, file := range files {
		line := "- " + file + "\n"
		if output.Len()+len(line)+96 > maxBytes {
			fmt.Fprintf(&output, "%d additional written files.\n", len(files)-index)
			break
		}
		if index == 0 {
			output.WriteString("Written files:\n")
		}
		output.WriteString(line)
	}
	return strings.TrimSpace(output.String()), nil
}
