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
	selected, dispatches, err := reduceObjectiveDatabaseCandidates(
		ctx, evidenceNeedID, exactNeed, objectiveContext, candidates, stations,
	)
	if err != nil {
		return nil, dispatches, err
	}
	if len(selected) == 0 {
		return nil, dispatches, fmt.Errorf("database schema selection found no relation for evidence need %q", evidenceNeedID)
	}
	ids := make([]string, len(selected))
	for index, candidate := range selected {
		ids[index] = candidate.RelationID
	}
	return ids, dispatches, nil
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
	var total int
	selected := make([]assemblyline.DatabaseSchemaCandidate, 0, databaseSchemaSelectionLimit)
	for start := 0; start < len(candidates); start += databaseSchemaSelectionChunk {
		if len(selected) == databaseSchemaSelectionLimit {
			break
		}
		if total > maxDatabaseSchemaSelectionModelCalls-exactSemanticLeafCalls {
			return nil, total, fmt.Errorf(
				"database schema selection exceeded its %d-call semantic reduction bound",
				maxDatabaseSchemaSelectionModelCalls,
			)
		}
		end := start + databaseSchemaSelectionChunk
		if end > len(candidates) {
			end = len(candidates)
		}
		chunk := append([]assemblyline.DatabaseSchemaCandidate(nil), candidates[start:end]...)
		remainingCapacity := databaseSchemaSelectionLimit - len(selected)
		bound := remainingCapacity
		if bound > len(chunk) {
			bound = len(chunk)
		}
		input := assemblyline.DatabaseSchemaSelectionInput{
			EvidenceNeedID: evidenceNeedID, ExactNeed: exactNeed,
			Context:              assemblyline.CloneObjectiveContext(objectiveContext),
			Candidates:           chunk,
			MaxSelections:        bound,
			HasAcceptedRelations: len(selected) > 0,
		}
		decision, dispatches, err := stations.SelectSchema(ctx, input)
		total += dispatches
		if err != nil {
			return nil, total, err
		}
		if err := validateObjectiveCallCount(
			"database schema selection chunk", dispatches, maxDatabaseSchemaSelectionLeafCalls,
		); err != nil {
			return nil, total, err
		}
		if total > maxDatabaseSchemaSelectionModelCalls {
			return nil, total, fmt.Errorf(
				"database schema selection exceeded its %d-call semantic reduction bound",
				maxDatabaseSchemaSelectionModelCalls,
			)
		}
		if err := decision.ValidateFor(input); err != nil {
			return nil, total, err
		}
		byID := make(map[string]assemblyline.DatabaseSchemaCandidate, len(chunk))
		for _, candidate := range chunk {
			byID[candidate.RelationID] = candidate
		}
		for _, id := range decision.RelationIDs {
			selected = append(selected, byID[id])
		}
		if len(selected) > databaseSchemaSelectionLimit {
			return nil, total, fmt.Errorf(
				"database schema selection found more than %d necessary relations without reopening accepted selections",
				databaseSchemaSelectionLimit,
			)
		}
	}
	return selected, total, nil
}

func objectiveDatabaseRelationDescriptor(snapshot datasource.SchemaSnapshot, relationID string) (string, error) {
	projection, err := datasource.ProjectSchemaForIntent(snapshot, []string{relationID})
	if err != nil {
		return "", err
	}
	relation := projection.Relations[0]
	var rendered strings.Builder
	fmt.Fprintf(
		&rendered, "%s.%s is a %s relation.\n",
		relation.SchemaName, relation.Name, relation.Kind,
	)
	for _, column := range relation.Columns {
		nullability := "does not allow missing values"
		if column.Nullable {
			nullability = "allows missing values"
		}
		fmt.Fprintf(
			&rendered, "Field %s contains %s values and %s.",
			column.Name, column.TypeCategory, nullability,
		)
		if len(column.AllowedValues) > 0 {
			fmt.Fprintf(&rendered, " Its allowed values are %s.", strings.Join(column.AllowedValues, ", "))
		}
		rendered.WriteByte('\n')
	}
	for _, foreignKey := range relation.ForeignKeys {
		columnLabels, err := objectiveDatabaseColumnLabels(relation.Columns, foreignKey.ColumnIDs)
		if err != nil {
			return "", fmt.Errorf("database relation %q foreign key: %w", relation.Name, err)
		}
		referencedRelation, err := snapshot.Relation(foreignKey.ReferencedRelationID)
		if err != nil {
			return "", err
		}
		referencedLabels, err := objectiveDatabaseSchemaColumnLabels(
			referencedRelation.Columns, foreignKey.ReferencedColumnIDs,
		)
		if err != nil {
			return "", fmt.Errorf("database relation %q referenced foreign key: %w", relation.Name, err)
		}
		fmt.Fprintf(
			&rendered, "Fields %s reference %s.%s fields %s.\n",
			strings.Join(columnLabels, ", "), referencedRelation.Schema, referencedRelation.Name,
			strings.Join(referencedLabels, ", "),
		)
	}
	value := strings.TrimSpace(rendered.String())
	if strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("database relation descriptor is empty")
	}
	return value, nil
}

func objectiveDatabaseColumnLabels(
	columns []datasource.IntentColumnProjection,
	ids []string,
) ([]string, error) {
	labels := make([]string, len(ids))
	for index, id := range ids {
		found := false
		for _, column := range columns {
			if column.ID == id {
				labels[index] = column.Name
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("unknown local field reference")
		}
	}
	return labels, nil
}

func objectiveDatabaseSchemaColumnLabels(
	columns []datasource.SchemaColumn,
	ids []string,
) ([]string, error) {
	labels := make([]string, len(ids))
	for index, id := range ids {
		found := false
		for _, column := range columns {
			if column.ID == id {
				labels[index] = column.Name
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("unknown referenced field")
		}
	}
	return labels, nil
}
