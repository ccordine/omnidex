package contextbuilder

import (
	"fmt"

	"github.com/gryph/omnidex/internal/taskstate"
)

const maxSelectionSourceRefs = 32

func validateSourceRefs(refs []taskstate.Ref) error {
	if refs == nil || len(refs) > maxSelectionSourceRefs {
		return fmt.Errorf("source refs must be an explicit array of at most %d entries", maxSelectionSourceRefs)
	}
	seen := make(map[string]string, len(refs))
	for index, ref := range refs {
		if err := taskstate.ValidateRef(ref); err != nil {
			return fmt.Errorf("source ref %d: %w", index, err)
		}
		identity := taskstate.RefIdentity(ref)
		if hash, exists := seen[identity]; exists {
			return fmt.Errorf("source ref %d repeats identity with hashes %s and %s", index, hash, ref.Hash)
		}
		seen[identity] = ref.Hash
	}
	return nil
}

func cloneSourceRefs(refs []taskstate.Ref) []taskstate.Ref {
	if refs == nil {
		return nil
	}
	return append([]taskstate.Ref{}, refs...)
}
