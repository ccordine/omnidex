package queue

import (
	"fmt"

	"github.com/gryph/omnidex/internal/taskstate"
)

const (
	acceptedIntentProjectionSchema = "omnidex.accepted-intent-projection.v1"
	acceptedIntentObjectivePrefix  = "objective:intent:"
	acceptedIntentEdgePrefix       = "edge:intent:"
	acceptedIntentEntryPrefix      = "entry:intent:"
)

type acceptedIntentProjection struct {
	ArtifactID      int64
	JobID           int64
	StepID          int64
	JobGeneration   int64
	LedgerID        taskstate.LedgerID
	PayloadSHA256   string
	ObjectiveNodeID taskstate.NodeID
	LedgerStart     uint64
	LedgerEnd       uint64
	Items           []acceptedIntentProjectionItem
}

type acceptedIntentProjectionItem struct {
	Kind          string
	Ordinal       int
	NodeID        taskstate.NodeID
	EntryID       taskstate.EntryID
	SourceURI     string
	SourceVersion string
	SourceSHA256  string
}

func (item acceptedIntentProjectionItem) sourceRef() taskstate.Ref {
	return taskstate.Ref{
		URI: item.SourceURI, Version: item.SourceVersion,
		Hash: item.SourceSHA256, Relation: taskstate.RefSource,
	}
}

func acceptedIntentSourceURI(jobID, artifactID int64, kind string, ordinal int) string {
	return fmt.Sprintf(
		"artifact://job/%d/artifact/%d/intent/%s/%d",
		jobID, artifactID, kind, ordinal,
	)
}
