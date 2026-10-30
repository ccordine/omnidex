package codingagent

import (
	"fmt"
	"strings"
)

// TaskLedgerCompletionRecap reports the database-driven task loop. Completion
// follows item acceptance and the final original-goal review; this function
// only renders the task acceptance evidence.
func TaskLedgerCompletionRecap(entries []Entry, taskCount, maxBytes int) (string, error) {
	if taskCount < 1 {
		return "", fmt.Errorf("task ledger completion recap requires authoritative accepted task counts")
	}
	files := []string{}
	for _, entry := range entries {
		if entry.Phase == "files_written" {
			files = append(files, entry.WrittenFiles...)
		}
	}
	files = uniqueFiles(files)
	header := fmt.Sprintf("Tasks completed: %d. Independent task reviews passed.\n", taskCount)
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
