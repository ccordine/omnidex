package operation

import "github.com/gryph/omnidex/internal/evidence"

// Result is the code-owned record of one deterministic operation.
type Result struct {
	Summary  string
	Output   map[string]any
	Warnings []string
	Evidence []evidence.Record
}
