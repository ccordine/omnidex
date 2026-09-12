package worker

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/datasource"
	"github.com/gryph/omnidex/internal/queue"
)

const (
	maxDatabaseEvidenceCapsules     = 12
	maxDatabaseEvidenceContextBytes = 8 * 1024
	maxDatabaseEvidenceNeedBytes    = 2 * 1024
)

func projectObjectiveDatabaseEvidence(record queue.DatabaseEvidenceRecord) ([]objectiveEvidence, error) {
	if record.ID < 1 || record.JobID < 1 {
		return nil, fmt.Errorf("database evidence projection requires its recorded execution")
	}
	snapshot, intent, evidence := record.Snapshot, record.Plan.Intent, record.Evidence
	if err := evidence.ValidateForPlan(snapshot, record.Plan, objectiveDatabaseExecutionLimits()); err != nil {
		return nil, err
	}
	base, err := datasource.ProjectEvidenceRows(snapshot, intent, evidence.Result, 0, evidence.Result.RowCount)
	if err != nil {
		return nil, err
	}
	base.Rows = nil
	groups, err := splitObjectiveDatabaseRows(base, evidence.Result.Rows)
	if err != nil {
		return nil, err
	}
	if len(groups) > maxDatabaseEvidenceCapsules {
		return nil, fmt.Errorf("database evidence requires %d capsules; maximum is %d", len(groups), maxDatabaseEvidenceCapsules)
	}
	projected := make([]objectiveEvidence, 0, len(groups))
	total, rowStart := 0, 0
	for index, rows := range groups {
		rowEnd := rowStart + len(rows)
		payload, err := datasource.ProjectEvidenceRows(snapshot, intent, evidence.Result, rowStart, rowEnd)
		if err != nil {
			return nil, err
		}
		encoded, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("encode database evidence capsule: %w", err)
		}
		total += len(encoded)
		if total > maxDatabaseEvidenceContextBytes {
			return nil, fmt.Errorf("database evidence projection exceeds %d context bytes", maxDatabaseEvidenceContextBytes)
		}
		item, err := newObjectiveEvidence(fmt.Sprintf("DB-%02d", index+1), string(encoded), "postgres_query", record.SourceRef())
		if err != nil {
			return nil, err
		}
		item.DatabaseEvidenceID = record.ID
		item.DatabaseRowStart, item.DatabaseRowEnd = rowStart, rowEnd
		item.ObservedAt = evidence.Execution.AcquiredAt
		projected = append(projected, item)
		rowStart = rowEnd
	}
	return projected, nil
}

func splitObjectiveDatabaseRows(
	base datasource.EvidenceRows,
	rows [][]datasource.EvidenceValue,
) ([][][]datasource.EvidenceValue, error) {
	if len(rows) == 0 {
		return [][][]datasource.EvidenceValue{{}}, nil
	}
	groups := [][][]datasource.EvidenceValue{}
	current := [][]datasource.EvidenceValue{}
	for _, row := range rows {
		candidate := append(append([][]datasource.EvidenceValue(nil), current...), row)
		probe := base
		probe.Rows = candidate
		encoded, err := json.Marshal(probe)
		if err != nil {
			return nil, err
		}
		if len(encoded) <= maxObjectiveEvidenceTextBytes {
			current = candidate
			continue
		}
		if len(current) == 0 {
			return nil, fmt.Errorf("one database evidence row exceeds %d context bytes", maxObjectiveEvidenceTextBytes)
		}
		groups = append(groups, current)
		current = [][]datasource.EvidenceValue{row}
		probe.Rows = current
		encoded, err = json.Marshal(probe)
		if err != nil || len(encoded) > maxObjectiveEvidenceTextBytes {
			return nil, fmt.Errorf("one database evidence row cannot fit the bounded context projection")
		}
	}
	groups = append(groups, current)
	return groups, nil
}

func validateDatabaseEvidenceNeed(value string) error {
	if value == "" || value != strings.TrimSpace(value) || len(value) > maxDatabaseEvidenceNeedBytes {
		return fmt.Errorf("database evidence need must be one bounded trimmed semantic value")
	}
	return nil
}
