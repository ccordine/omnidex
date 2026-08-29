package worker

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/datasource"
)

const (
	maxDatabaseEvidenceCapsules     = 12
	maxDatabaseEvidenceContextBytes = 8 * 1024
	maxDatabaseEvidenceNeedBytes    = 2 * 1024
)

type objectiveDatabaseEvidenceColumn struct {
	Label string                        `json:"label"`
	Kind  datasource.ColumnTypeCategory `json:"kind"`
}

type objectiveDatabaseEvidencePayload struct {
	Columns []objectiveDatabaseEvidenceColumn `json:"columns"`
	Rows    [][]datasource.EvidenceValue      `json:"rows"`
}

func projectObjectiveDatabaseEvidence(
	round int,
	snapshot datasource.SchemaSnapshot,
	intent datasource.RelationalIntent,
	evidence datasource.EvidenceResult,
) ([]objectiveEvidence, error) {
	if round < 1 || evidence.Schema != datasource.EvidenceResultV1 ||
		evidence.Provenance.SourceID != snapshot.SourceID ||
		evidence.Provenance.SchemaFingerprint != snapshot.Fingerprint ||
		evidence.Result.Hash != evidence.Provenance.ResultHash {
		return nil, fmt.Errorf("database evidence does not match its schema and execution authority")
	}
	if !validObjectiveSHA256(evidence.Provenance.IntentHash) ||
		!validObjectiveSHA256(evidence.Provenance.QueryHash) ||
		!validObjectiveSHA256(evidence.Provenance.ResultHash) {
		return nil, fmt.Errorf("database evidence provenance hashes are invalid")
	}
	columns, err := objectiveDatabaseEvidenceColumns(snapshot, intent, evidence)
	if err != nil {
		return nil, err
	}
	base := objectiveDatabaseEvidencePayload{
		Columns: columns,
	}
	groups, err := splitObjectiveDatabaseRows(base, evidence.Result.Rows)
	if err != nil {
		return nil, err
	}
	if len(groups) > maxDatabaseEvidenceCapsules {
		return nil, fmt.Errorf("database evidence requires %d capsules; maximum is %d", len(groups), maxDatabaseEvidenceCapsules)
	}
	projected := make([]objectiveEvidence, 0, len(groups))
	total := 0
	for index, rows := range groups {
		payload := base
		payload.Rows = rows
		encoded, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("encode database evidence capsule: %w", err)
		}
		total += len(encoded)
		if total > maxDatabaseEvidenceContextBytes {
			return nil, fmt.Errorf("database evidence projection exceeds %d context bytes", maxDatabaseEvidenceContextBytes)
		}
		id := fmt.Sprintf("DB%d-%02d-%s", round, index+1, evidence.Provenance.ResultHash[:12])
		sourceRef := fmt.Sprintf(
			"database:%s:intent:%s:result:%s",
			snapshot.SourceID, evidence.Provenance.IntentHash[:12], evidence.Provenance.ResultHash[:12],
		)
		item, err := newObjectiveEvidence(id, string(encoded), "postgres_query", sourceRef)
		if err != nil {
			return nil, err
		}
		item.SourceSHA256 = evidence.Provenance.ResultHash
		item.ObservedAt = evidence.Provenance.AcquiredAt
		projected = append(projected, item)
	}
	return projected, nil
}

func objectiveDatabaseEvidenceColumns(
	snapshot datasource.SchemaSnapshot,
	intent datasource.RelationalIntent,
	evidence datasource.EvidenceResult,
) ([]objectiveDatabaseEvidenceColumn, error) {
	if intent.Shape == datasource.ResultExistence {
		if len(intent.Projections) != 0 || len(evidence.Result.Columns) != 1 {
			return nil, fmt.Errorf("database existence evidence must contain exactly one code-owned boolean column")
		}
		return []objectiveDatabaseEvidenceColumn{{
			Label: "exists", Kind: datasource.TypeBoolean,
		}}, nil
	}
	if len(evidence.Result.Columns) != len(intent.Projections) {
		return nil, fmt.Errorf("database evidence column count does not match relational intent")
	}
	columns := make([]objectiveDatabaseEvidenceColumn, len(intent.Projections))
	for index, projection := range intent.Projections {
		label, category, err := objectiveDatabaseProjectionLabel(snapshot, projection)
		if err != nil {
			return nil, err
		}
		columns[index] = objectiveDatabaseEvidenceColumn{
			Label: label, Kind: category,
		}
	}
	return columns, nil
}

func objectiveDatabaseProjectionLabel(
	snapshot datasource.SchemaSnapshot,
	projection datasource.RelationalProjection,
) (string, datasource.ColumnTypeCategory, error) {
	if projection.Aggregate == datasource.AggregateCountRows {
		return "count_rows", datasource.TypeInteger, nil
	}
	relation, column, err := snapshot.Column(projection.FieldID)
	if err != nil {
		return "", "", err
	}
	name := relation.Schema + "." + relation.Name + "." + column.Name
	if projection.Aggregate != "" {
		return string(projection.Aggregate) + "(" + name + ")", column.TypeCategory, nil
	}
	if projection.TimeBucket != "" {
		return string(projection.TimeBucket) + "(" + name + ")", datasource.TypeTemporal, nil
	}
	return name, column.TypeCategory, nil
}

func splitObjectiveDatabaseRows(
	base objectiveDatabaseEvidencePayload,
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
