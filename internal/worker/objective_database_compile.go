package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/datasource"
)

func prepareObjectiveDatabaseQueryPlan(
	ctx context.Context,
	snapshot datasource.SchemaSnapshot,
	intent datasource.RelationalIntent,
	evidenceNeedID string,
	exactNeed string,
	objectiveContext assemblyline.ObjectiveContext,
	stations objectiveDatabaseStations,
) (datasource.RelationalQueryPlan, int, error) {
	selected := map[string]string{}
	totalCalls := 0
	for attempts := 0; attempts <= datasource.MaxProjectedRelations; attempts++ {
		plan, err := datasource.BuildRelationalQueryPlan(snapshot, intent, selected)
		if err == nil {
			return plan, totalCalls, nil
		}
		var ambiguous *datasource.AmbiguousJoinPathError
		if !errors.As(err, &ambiguous) {
			return datasource.RelationalQueryPlan{}, totalCalls, err
		}
		if _, repeated := selected[ambiguous.ToRelationID]; repeated {
			return datasource.RelationalQueryPlan{}, totalCalls, fmt.Errorf("database join-path selection made no planning progress")
		}
		input, err := objectiveDatabaseJoinPathInput(
			snapshot, evidenceNeedID, exactNeed, objectiveContext, ambiguous,
		)
		if err != nil {
			return datasource.RelationalQueryPlan{}, totalCalls, err
		}
		decision, receipt, err := stations.SelectJoinPath(ctx, input)
		totalCalls += receipt.Calls
		if err != nil {
			return datasource.RelationalQueryPlan{}, totalCalls, err
		}
		if err := validateObjectiveDatabaseStationCalls("join-path selection", receipt); err != nil {
			return datasource.RelationalQueryPlan{}, totalCalls, err
		}
		if err := decision.ValidateFor(input); err != nil {
			return datasource.RelationalQueryPlan{}, totalCalls, err
		}
		selected[ambiguous.ToRelationID] = decision.PathID
	}
	return datasource.RelationalQueryPlan{}, totalCalls, fmt.Errorf("database join-path resolution exceeded its bounded relation count")
}

func objectiveDatabaseJoinPathInput(
	snapshot datasource.SchemaSnapshot,
	evidenceNeedID string,
	exactNeed string,
	objectiveContext assemblyline.ObjectiveContext,
	ambiguous *datasource.AmbiguousJoinPathError,
) (assemblyline.DatabaseJoinPathSelectionInput, error) {
	if ambiguous == nil {
		return assemblyline.DatabaseJoinPathSelectionInput{}, fmt.Errorf("database join-path ambiguity is unavailable")
	}
	candidates := make([]assemblyline.DatabaseJoinPathCandidate, len(ambiguous.Candidates))
	for index, path := range ambiguous.Candidates {
		descriptor, err := objectiveDatabaseJoinPathDescriptor(snapshot, path)
		if err != nil {
			return assemblyline.DatabaseJoinPathSelectionInput{}, err
		}
		candidates[index] = assemblyline.DatabaseJoinPathCandidate{PathID: path.ID, Descriptor: descriptor}
	}
	input := assemblyline.DatabaseJoinPathSelectionInput{
		EvidenceNeedID: evidenceNeedID, ExactNeed: exactNeed,
		Context:        assemblyline.CloneObjectiveContext(objectiveContext),
		FromRelationID: ambiguous.FromRelationID, ToRelationID: ambiguous.ToRelationID,
		Candidates: candidates,
	}
	if _, err := assemblyline.NewDatabaseJoinPathSelectionJob(input); err != nil {
		return assemblyline.DatabaseJoinPathSelectionInput{}, err
	}
	return input, nil
}

type objectiveDatabaseJoinStepDescriptor struct {
	From       string   `json:"from"`
	To         string   `json:"to"`
	ForeignKey string   `json:"foreign_key"`
	Columns    []string `json:"columns"`
	References []string `json:"references"`
	Direction  string   `json:"direction"`
}

func objectiveDatabaseJoinPathDescriptor(
	snapshot datasource.SchemaSnapshot,
	path datasource.JoinPath,
) (string, error) {
	steps := make([]objectiveDatabaseJoinStepDescriptor, len(path.Steps))
	for index, step := range path.Steps {
		from, err := snapshot.Relation(step.FromRelationID)
		if err != nil {
			return "", err
		}
		to, err := snapshot.Relation(step.ToRelationID)
		if err != nil {
			return "", err
		}
		owner, foreignKey, err := objectiveDatabaseForeignKey(snapshot, step.ForeignKeyID)
		if err != nil {
			return "", err
		}
		columns, err := objectiveDatabaseColumnNames(snapshot, foreignKey.ColumnIDs)
		if err != nil {
			return "", err
		}
		references, err := objectiveDatabaseColumnNames(snapshot, foreignKey.ReferencedColumnIDs)
		if err != nil {
			return "", err
		}
		steps[index] = objectiveDatabaseJoinStepDescriptor{
			From: from.Schema + "." + from.Name, To: to.Schema + "." + to.Name,
			ForeignKey: owner.Schema + "." + owner.Name + "." + foreignKey.Name,
			Columns:    columns, References: references, Direction: string(step.Direction),
		}
	}
	encoded, err := json.Marshal(steps)
	if err != nil {
		return "", fmt.Errorf("encode database join-path descriptor: %w", err)
	}
	return string(encoded), nil
}

func objectiveDatabaseForeignKey(
	snapshot datasource.SchemaSnapshot,
	id string,
) (datasource.SchemaRelation, datasource.SchemaForeignKey, error) {
	for _, relation := range snapshot.Relations {
		for _, foreignKey := range relation.ForeignKeys {
			if foreignKey.ID == id {
				return relation, foreignKey, nil
			}
		}
	}
	return datasource.SchemaRelation{}, datasource.SchemaForeignKey{}, fmt.Errorf("unknown database foreign key ID %q", id)
}

func objectiveDatabaseColumnNames(snapshot datasource.SchemaSnapshot, ids []string) ([]string, error) {
	names := make([]string, len(ids))
	for index, id := range ids {
		relation, column, err := snapshot.Column(id)
		if err != nil {
			return nil, err
		}
		names[index] = relation.Schema + "." + relation.Name + "." + column.Name
	}
	return names, nil
}
