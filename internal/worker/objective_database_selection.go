package worker

import (
	"context"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/datasource"
)

const (
	databaseSchemaSelectionChunk         = 24
	databaseSchemaSelectionLimit         = 8
	maxDatabaseSchemaSelectionModelCalls = 96
)

func selectObjectiveDatabaseRelations(
	ctx context.Context,
	snapshot datasource.SchemaSnapshot,
	evidenceNeedID string,
	exactNeed string,
	objectiveContext assemblyline.ObjectiveContext,
	stations objectiveDatabaseStations,
) ([]string, int, error) {
	if len(snapshot.Relations) == 0 {
		return nil, 0, fmt.Errorf("database schema snapshot has no relations")
	}
	if len(snapshot.Relations) == 1 {
		return []string{snapshot.Relations[0].ID}, 0, nil
	}
	candidates := make([]assemblyline.DatabaseSchemaCandidate, 0, len(snapshot.Relations))
	for _, relation := range snapshot.Relations {
		descriptor, err := objectiveDatabaseRelationDescriptor(snapshot, relation.ID)
		if err != nil {
			return nil, 0, err
		}
		candidates = append(candidates, assemblyline.DatabaseSchemaCandidate{
			RelationID: relation.ID, Descriptor: descriptor,
		})
	}
	selected, calls, err := reduceObjectiveDatabaseCandidates(
		ctx, evidenceNeedID, exactNeed, objectiveContext, candidates, stations,
	)
	if err != nil {
		return nil, calls, err
	}
	if len(selected) == 0 {
		return nil, calls, fmt.Errorf("database schema selection found no relation for evidence need %q", evidenceNeedID)
	}
	ids := make([]string, len(selected))
	for index, candidate := range selected {
		ids[index] = candidate.RelationID
	}
	return ids, calls, nil
}

func reduceObjectiveDatabaseCandidates(
	ctx context.Context,
	evidenceNeedID string,
	exactNeed string,
	objectiveContext assemblyline.ObjectiveContext,
	candidates []assemblyline.DatabaseSchemaCandidate,
	stations objectiveDatabaseStations,
) ([]assemblyline.DatabaseSchemaCandidate, int, error) {
	if stations == nil {
		return nil, 0, fmt.Errorf("database schema selection station is unavailable")
	}
	current := append([]assemblyline.DatabaseSchemaCandidate(nil), candidates...)
	totalCalls := 0
	for {
		next := make([]assemblyline.DatabaseSchemaCandidate, 0)
		for start := 0; start < len(current); start += databaseSchemaSelectionChunk {
			if totalCalls > maxDatabaseSchemaSelectionModelCalls-maxTypedWorkerAttempts {
				return nil, totalCalls, fmt.Errorf(
					"database schema selection exceeded its %d-call semantic reduction bound",
					maxDatabaseSchemaSelectionModelCalls,
				)
			}
			end := start + databaseSchemaSelectionChunk
			if end > len(current) {
				end = len(current)
			}
			chunk := append([]assemblyline.DatabaseSchemaCandidate(nil), current[start:end]...)
			bound := databaseSchemaSelectionLimit
			if bound > len(chunk) {
				bound = len(chunk)
			}
			input := assemblyline.DatabaseSchemaSelectionInput{
				EvidenceNeedID: evidenceNeedID, ExactNeed: exactNeed,
				Context:    assemblyline.CloneObjectiveContext(objectiveContext),
				Candidates: chunk, MaxSelections: bound,
			}
			decision, receipt, err := stations.SelectSchema(ctx, input)
			totalCalls += receipt.Calls
			if err != nil {
				return nil, totalCalls, err
			}
			if err := validateObjectiveDatabaseStationCalls("schema selection", receipt); err != nil {
				return nil, totalCalls, err
			}
			if err := decision.ValidateFor(input); err != nil {
				return nil, totalCalls, err
			}
			byID := make(map[string]assemblyline.DatabaseSchemaCandidate, len(chunk))
			for _, candidate := range chunk {
				byID[candidate.RelationID] = candidate
			}
			for _, id := range decision.RelationIDs {
				next = append(next, byID[id])
			}
		}
		if len(next) <= databaseSchemaSelectionLimit {
			return next, totalCalls, nil
		}
		if len(next) >= len(current) {
			return nil, totalCalls, fmt.Errorf("database schema selection made no bounded reduction")
		}
		current = next
	}
}

func objectiveDatabaseRelationDescriptor(snapshot datasource.SchemaSnapshot, relationID string) (string, error) {
	projection, err := datasource.ProjectSchemaForIntent(snapshot, []string{relationID})
	if err != nil {
		return "", err
	}
	relation := projection.Relations[0]
	var rendered strings.Builder
	fmt.Fprintf(&rendered, "RELATION label=%s.%s kind=%s\n", relation.SchemaName, relation.Name, relation.Kind)
	for _, column := range relation.Columns {
		fmt.Fprintf(
			&rendered, "FIELD %s label=%s type=%s nullable=%t",
			column.ID, column.Name, column.TypeCategory, column.Nullable,
		)
		if len(column.AllowedValues) > 0 {
			fmt.Fprintf(&rendered, " allowed=%s", strings.Join(column.AllowedValues, " | "))
		}
		rendered.WriteByte('\n')
	}
	for _, foreignKey := range relation.ForeignKeys {
		fmt.Fprintf(
			&rendered, "FOREIGN KEY %s fields=%s references_relation=%s references_fields=%s\n",
			foreignKey.ID, strings.Join(foreignKey.ColumnIDs, " | "),
			foreignKey.ReferencedRelationID, strings.Join(foreignKey.ReferencedColumnIDs, " | "),
		)
	}
	value := strings.TrimSpace(rendered.String())
	if strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("database relation descriptor is empty")
	}
	return value, nil
}
