package objectiveworkload

import (
	"context"

	"github.com/gryph/omnidex/internal/assemblyline"
)

type CompileLimits struct {
	MaxStationCalls int
}

type PartitionStation interface {
	Generate(context.Context, assemblyline.PortableJob) (assemblyline.PortableResult, error)
}

type GapKind string

const GapRequirementPartition GapKind = "requirement_partition"

type GapStatus string

const (
	GapOpened     GapStatus = "opened"
	GapDispatched GapStatus = "dispatched"
	GapResolved   GapStatus = "resolved"
	GapFailed     GapStatus = "failed"
)

type PartitionGapRecord struct {
	ID                      GapID
	Kind                    GapKind
	CompilationID           CompilationID
	FinalWorkloadID         WorkloadID
	JobID                   string
	Mode                    assemblyline.RequirementPartitionMode
	InputSHA256             string
	ResponseObserved        bool
	ResponseWithinBounds    bool
	ResponseJobIDMatches    bool
	ResponseJobIDBytes      int
	ResponseSHA256          string
	ResponseJobIDSHA256     string
	ResponseCandidateSHA256 string
	ResponseCandidateBytes  int
	OutputSHA256            string
	Status                  GapStatus
}

type CompileResult struct {
	EvidenceClass EvidenceClass
	Authority     Authority
	CompilationID CompilationID
	Gaps          []PartitionGapRecord
	StationCalls  int
	Workload      Workload
	Compiled      bool
}

func (limits CompileLimits) validate() error {
	if limits.MaxStationCalls < 1 || limits.MaxStationCalls > maxStationCalls {
		return ErrCompileBound
	}
	return nil
}

func cloneGaps(values []PartitionGapRecord) []PartitionGapRecord {
	return append([]PartitionGapRecord{}, values...)
}
