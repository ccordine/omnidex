package queue

import (
	"fmt"
	"sort"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionruntime"
)

const (
	cognitionLifecycleRetirementSchemaV1 = "omnidex.cognition-lifecycle-retirement.v1"
	cognitionLifecycleSealSetSchemaV1    = "omnidex.cognition-lifecycle-seal-set.v1"
	cognitionTerminalAuthorityWorker     = "worker"
	cognitionTerminalAuthorityLifecycle  = "lifecycle"
)

type cognitionLifecycleRetirement struct {
	Schema           string                            `json:"schema"`
	ID               string                            `json:"id"`
	SHA256           string                            `json:"sha256"`
	OperationID      LifecycleOperationID              `json:"operation_id"`
	OperationKind    LifecycleOperationKind            `json:"operation_kind"`
	OperationSHA256  string                            `json:"operation_sha256"`
	EpisodeID        cognition.EpisodeID               `json:"episode_id"`
	JobID            int64                             `json:"job_id"`
	JobGeneration    int64                             `json:"job_generation"`
	StepID           int64                             `json:"step_id"`
	Code             cognitionruntime.CancellationCode `json:"code"`
	ExpectedRevision cognition.WorldRevision           `json:"expected_revision"`
	GraphVersion     uint64                            `json:"graph_version"`
	GraphSHA256      string                            `json:"graph_sha256"`
}

type cognitionLifecycleSealEntry struct {
	EpisodeID        cognition.EpisodeID `json:"episode_id"`
	RetirementID     string              `json:"retirement_id"`
	RetirementSHA256 string              `json:"retirement_sha256"`
	TraceSHA256      string              `json:"trace_sha256"`
}

type cognitionLifecycleSealSet struct {
	Schema          string                        `json:"schema"`
	OperationID     LifecycleOperationID          `json:"operation_id"`
	OperationKind   LifecycleOperationKind        `json:"operation_kind"`
	OperationSHA256 string                        `json:"operation_sha256"`
	JobID           int64                         `json:"job_id"`
	Generation      int64                         `json:"generation"`
	Entries         []cognitionLifecycleSealEntry `json:"entries"`
	SHA256          string                        `json:"sha256"`
}

func newCognitionLifecycleRetirement(
	descriptor lifecycleOperationDescriptor,
	episode CognitionEpisode,
	graph CognitionObligationGraphRecord,
	code cognitionruntime.CancellationCode,
) (cognitionLifecycleRetirement, error) {
	value := cognitionLifecycleRetirement{
		Schema:      cognitionLifecycleRetirementSchemaV1,
		OperationID: descriptor.ID, OperationKind: descriptor.Kind,
		OperationSHA256: descriptor.SHA256, EpisodeID: episode.EpisodeID,
		JobID: episode.Authority.JobID, JobGeneration: episode.Authority.Generation,
		StepID: episode.Authority.StepID, Code: code,
		ExpectedRevision: episode.CurrentRevision, GraphVersion: graph.Version,
		GraphSHA256: graph.Graph.SHA256,
	}
	sha, err := cognitionLifecycleRetirementSHA(value)
	if err != nil {
		return cognitionLifecycleRetirement{}, err
	}
	value.SHA256, value.ID = sha, "cognition_retirement_"+sha
	if err := value.Validate(); err != nil {
		return cognitionLifecycleRetirement{}, err
	}
	return value, nil
}

func (value cognitionLifecycleRetirement) Validate() error {
	if value.Schema != cognitionLifecycleRetirementSchemaV1 ||
		value.ID != "cognition_retirement_"+value.SHA256 ||
		!cognitionDigestPattern.MatchString(value.SHA256) ||
		!cognitionDigestPattern.MatchString(value.OperationSHA256) ||
		value.JobID <= 0 || value.JobGeneration <= 0 || value.StepID <= 0 ||
		value.ExpectedRevision.EpisodeID != value.EpisodeID || value.ExpectedRevision.Validate() != nil ||
		value.GraphVersion == 0 || !cognitionDigestPattern.MatchString(value.GraphSHA256) ||
		!validCognitionLifecycleKindCode(value.OperationKind, value.Code) {
		return fmt.Errorf("%w: cognition lifecycle retirement identity is invalid", ErrCognitionConflict)
	}
	if _, err := ParseLifecycleOperationID(string(value.OperationID)); err != nil {
		return err
	}
	want, err := cognitionLifecycleRetirementSHA(value)
	if err != nil || want != value.SHA256 {
		return fmt.Errorf("%w: cognition lifecycle retirement hash changed", ErrCognitionConflict)
	}
	return nil
}

func cognitionLifecycleRetirementSHA(value cognitionLifecycleRetirement) (string, error) {
	value.ID, value.SHA256 = "", ""
	_, sha, err := cognitionJSON(value)
	return sha, err
}

func validCognitionLifecycleKindCode(kind LifecycleOperationKind, code cognitionruntime.CancellationCode) bool {
	return (kind == LifecycleCancelJob && code == cognitionruntime.CancellationJobCanceled) ||
		(kind == LifecycleReplanJob && code == cognitionruntime.CancellationGenerationRetired)
}

func newCognitionLifecycleSealSet(
	descriptor lifecycleOperationDescriptor,
	jobID, generation int64,
	entries []cognitionLifecycleSealEntry,
) (cognitionLifecycleSealSet, error) {
	copyEntries := append([]cognitionLifecycleSealEntry{}, entries...)
	sort.Slice(copyEntries, func(left, right int) bool {
		return copyEntries[left].EpisodeID < copyEntries[right].EpisodeID
	})
	value := cognitionLifecycleSealSet{
		Schema: cognitionLifecycleSealSetSchemaV1, OperationID: descriptor.ID,
		OperationKind: descriptor.Kind, OperationSHA256: descriptor.SHA256,
		JobID: jobID, Generation: generation, Entries: copyEntries,
	}
	sha, err := cognitionLifecycleSealSetSHA(value)
	if err != nil {
		return cognitionLifecycleSealSet{}, err
	}
	value.SHA256 = sha
	if err := value.Validate(); err != nil {
		return cognitionLifecycleSealSet{}, err
	}
	return value, nil
}

func (value cognitionLifecycleSealSet) Validate() error {
	if value.Schema != cognitionLifecycleSealSetSchemaV1 || value.JobID <= 0 || value.Generation <= 0 ||
		!cognitionDigestPattern.MatchString(value.OperationSHA256) ||
		!cognitionDigestPattern.MatchString(value.SHA256) ||
		(value.OperationKind != LifecycleCancelJob && value.OperationKind != LifecycleReplanJob) ||
		value.Entries == nil {
		return fmt.Errorf("%w: cognition lifecycle seal set identity is invalid", ErrCognitionConflict)
	}
	if _, err := ParseLifecycleOperationID(string(value.OperationID)); err != nil {
		return err
	}
	for index, entry := range value.Entries {
		if entry.EpisodeID == "" || entry.RetirementID != "cognition_retirement_"+entry.RetirementSHA256 ||
			!cognitionDigestPattern.MatchString(entry.RetirementSHA256) ||
			!cognitionDigestPattern.MatchString(entry.TraceSHA256) ||
			(index > 0 && value.Entries[index-1].EpisodeID >= entry.EpisodeID) {
			return fmt.Errorf("%w: cognition lifecycle seal set entry %d is invalid", ErrCognitionConflict, index)
		}
	}
	want, err := cognitionLifecycleSealSetSHA(value)
	if err != nil || want != value.SHA256 {
		return fmt.Errorf("%w: cognition lifecycle seal set hash changed", ErrCognitionConflict)
	}
	return nil
}

func cognitionLifecycleSealSetSHA(value cognitionLifecycleSealSet) (string, error) {
	value.SHA256 = ""
	_, sha, err := cognitionJSON(value)
	return sha, err
}
