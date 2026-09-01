package worker

import (
	"context"
	"errors"
	"fmt"
	"strings"

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
) (datasource.RelationalQueryPlan, objectiveStationReceipt, error) {
	selected := map[string]string{}
	var ledger objectiveDatabaseBoundedCallLedger
	for attempts := 0; attempts <= datasource.MaxProjectedRelations; attempts++ {
		plan, err := datasource.BuildRelationalQueryPlan(snapshot, intent, selected)
		if err == nil {
			if ledger.count() == 0 {
				return plan, objectiveStationReceipt{}, nil
			}
			receipt, receiptErr := ledger.totalForSuccess("database join-path planning")
			return plan, receipt, receiptErr
		}
		var ambiguous *datasource.AmbiguousJoinPathError
		if !errors.As(err, &ambiguous) {
			return datasource.RelationalQueryPlan{}, ledger.partial(), err
		}
		if _, repeated := selected[ambiguous.ToRelationID]; repeated {
			return datasource.RelationalQueryPlan{}, ledger.partial(), fmt.Errorf("database join-path selection made no planning progress")
		}
		input, err := objectiveDatabaseJoinPathInput(
			snapshot, evidenceNeedID, exactNeed, objectiveContext, ambiguous,
		)
		if err != nil {
			return datasource.RelationalQueryPlan{}, ledger.partial(), err
		}
		decision, receipt, err := stations.SelectJoinPath(ctx, input)
		if err != nil {
			return datasource.RelationalQueryPlan{}, ledger.partial(), err
		}
		if receipt != (objectiveStationReceipt{}) {
			if err := ledger.record(
				"database join-path planning", "selection", receipt, exactSemanticLeafCalls,
			); err != nil {
				return datasource.RelationalQueryPlan{}, ledger.partial(), err
			}
		}
		if err := decision.ValidateFor(input); err != nil {
			return datasource.RelationalQueryPlan{}, ledger.partial(), err
		}
		selected[ambiguous.ToRelationID] = decision.PathID
	}
	return datasource.RelationalQueryPlan{}, ledger.partial(), fmt.Errorf("database join-path resolution exceeded its bounded relation count")
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
	if _, _, err := assemblyline.ResolveSoleDatabaseJoinPathSelectionDecision(input); err != nil {
		return assemblyline.DatabaseJoinPathSelectionInput{}, err
	}
	return input, nil
}

func objectiveDatabaseJoinPathDescriptor(
	snapshot datasource.SchemaSnapshot,
	path datasource.JoinPath,
) (string, error) {
	steps := make([]string, len(path.Steps))
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
		steps[index] = fmt.Sprintf(
			"Step %d: %s.%s relates to %s.%s through the %s.%s.%s foreign key; %s references %s.",
			index+1,
			from.Schema, from.Name,
			to.Schema, to.Name,
			owner.Schema, owner.Name, foreignKey.Name,
			strings.Join(columns, ", "), strings.Join(references, ", "),
		)
	}
	return strings.Join(steps, "\n"), nil
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
