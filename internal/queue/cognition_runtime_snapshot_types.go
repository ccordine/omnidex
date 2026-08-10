package queue

import (
	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionruntime"
	"github.com/gryph/omnidex/internal/model"
)

type CognitionRuntimeSnapshotCommand struct {
	Authority model.StepAttemptAuthority
	EpisodeID cognition.EpisodeID
}

type CognitionRuntimeSnapshotRecord struct {
	Prepared    cognitionruntime.PreparedSnapshot
	CallOrdinal uint64
}

type cognitionSnapshotJournal struct {
	SnapshotSHA256         string
	PreparationID          string
	CallOrdinal            uint64
	Revision               cognition.WorldRevision
	ObligationID           cognition.ObligationID
	GraphVersion           uint64
	GraphSHA256            string
	ProjectionID           string
	WorkingSetID           string
	Budget                 cognition.RuntimeBudget
	EvidenceRefs           []cognition.EvidenceRef
	CompletionEvidenceRefs []cognition.EvidenceRef
	EnvironmentTerminal    bool
	PublicOutcome          string
}
