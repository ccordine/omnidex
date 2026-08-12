package queue

import (
	"fmt"
	"math"
)

// CognitionProjectionWorkingVersion is the durable call boundary at which a
// Working Set version was projected into one or more runtime snapshots.
type CognitionProjectionWorkingVersion struct {
	CallOrdinal    int64
	WorkingVersion int64
}

type CognitionWorkingSetTracePoint string

const (
	CognitionWorkingSetTraceEpisodeStart CognitionWorkingSetTracePoint = "episode_start"
	CognitionWorkingSetTraceEvent        CognitionWorkingSetTracePoint = "event"
	CognitionWorkingSetTraceTerminal     CognitionWorkingSetTracePoint = "terminal"
)

// VerifyCognitionWorkingSetTraceTuple applies the same call/phase/sequence
// authority used when the production sealed trace is constructed.
func VerifyCognitionWorkingSetTraceTuple(
	record CognitionSealedTraceRecord,
	point CognitionWorkingSetTracePoint,
	version uint64,
	projections []CognitionProjectionWorkingVersion,
) error {
	if version > math.MaxInt64 {
		return fmt.Errorf("%w: Working Set trace version is outside the durable range", ErrCognitionConflict)
	}
	maximum := int64(0)
	priorVersion := int64(0)
	for _, projection := range projections {
		if projection.CallOrdinal <= maximum || projection.WorkingVersion < priorVersion {
			return fmt.Errorf("%w: cognition projection chronology changed", ErrCognitionConflict)
		}
		maximum = projection.CallOrdinal
		priorVersion = projection.WorkingVersion
	}

	wantKind := "working_set_snapshot"
	wantCall := int64(0)
	wantPhase := 1
	switch point {
	case CognitionWorkingSetTraceEpisodeStart:
	case CognitionWorkingSetTraceEvent:
		wantKind = "working_set_event"
		wantCall = workingSetEventCallOrdinal(version, projections, maximum)
		wantPhase = 5
	case CognitionWorkingSetTraceTerminal:
		if maximum == math.MaxInt64 {
			return fmt.Errorf("%w: Working Set terminal call ordinal overflows", ErrCognitionConflict)
		}
		wantCall = maximum + 1
		wantPhase = 90
	default:
		return fmt.Errorf("%w: Working Set trace point is not registered", ErrCognitionConflict)
	}
	if record.Kind != wantKind || record.CallOrdinal != wantCall ||
		record.Phase != wantPhase || record.Sequence != int64(version) {
		return fmt.Errorf("%w: Working Set trace tuple changed", ErrCognitionConflict)
	}
	return nil
}
