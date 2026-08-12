package queue

import "fmt"

// VerifyCognitionSealedTraceRecordOrder applies the sole production trace
// ordering and reverse-completeness rule to records obtained through a pager.
func VerifyCognitionSealedTraceRecordOrder(records []CognitionSealedTraceRecord) error {
	seen := make(map[string]struct{}, len(records))
	var previous cognitionTraceRecord
	for index, value := range records {
		current := cognitionTraceRecord{
			Kind: value.Kind, CallOrdinal: value.CallOrdinal, Phase: value.Phase,
			Sequence: value.Sequence, ID: value.ID, SHA256: value.SHA256,
		}
		key := current.Kind + "\x00" + current.ID
		if _, duplicate := seen[key]; duplicate {
			return fmt.Errorf("%w: sealed cognition trace record %d is duplicated", ErrCognitionConflict, index)
		}
		seen[key] = struct{}{}
		if index > 0 && cognitionTraceRecordLess(current, previous) {
			return fmt.Errorf("%w: sealed cognition trace record order changed", ErrCognitionConflict)
		}
		previous = current
	}
	return nil
}
