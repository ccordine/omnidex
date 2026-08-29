package datasource

import (
	"fmt"
	"sort"
	"strings"
)

func (compiler *postgresIntentCompiler) planJoins() error {
	targets, err := requiredJoinRelationIDs(compiler.snapshot, compiler.intent)
	if err != nil {
		return err
	}
	type targetPath struct {
		target string
		path   JoinPath
	}
	paths := make([]targetPath, 0, len(targets))
	for _, target := range targets {
		path, err := ResolveJoinPath(compiler.snapshot, compiler.intent.FromRelationID, target, compiler.selectedJoinPaths[target])
		if err != nil {
			return err
		}
		paths = append(paths, targetPath{target: target, path: path})
	}
	sort.Slice(paths, func(i, j int) bool {
		if len(paths[i].path.Steps) == len(paths[j].path.Steps) {
			return paths[i].target < paths[j].target
		}
		return len(paths[i].path.Steps) < len(paths[j].path.Steps)
	})
	for _, target := range paths {
		for _, step := range target.path.Steps {
			if _, alreadyJoined := compiler.aliases[step.ToRelationID]; alreadyJoined {
				continue
			}
			fromAlias, exists := compiler.aliases[step.FromRelationID]
			if !exists {
				return fmt.Errorf("join path reaches relation %q before its parent", step.ToRelationID)
			}
			toRelation, err := compiler.snapshot.Relation(step.ToRelationID)
			if err != nil {
				return err
			}
			toAlias := fmt.Sprintf("t%d", len(compiler.aliases))
			condition, err := compiler.joinCondition(step, fromAlias, toAlias)
			if err != nil {
				return err
			}
			compiler.aliases[step.ToRelationID] = toAlias
			compiler.joins = append(compiler.joins, "JOIN "+quoteQualified(toRelation.Schema, toRelation.Name)+" AS "+toAlias+" ON "+condition)
		}
	}
	return nil
}

func validateSelectedJoinTargets(snapshot SchemaSnapshot, intent RelationalIntent, selected map[string]string) error {
	allowed, err := requiredJoinRelationIDs(snapshot, intent)
	if err != nil {
		return err
	}
	allowedSet := map[string]struct{}{}
	for _, relationID := range allowed {
		allowedSet[relationID] = struct{}{}
	}
	for _, exists := range intent.Exists {
		allowedSet[exists.RelationID] = struct{}{}
	}
	for relationID, pathID := range selected {
		if _, err := snapshot.Relation(relationID); err != nil {
			return err
		}
		if _, exists := allowedSet[relationID]; !exists {
			return fmt.Errorf("join path selection targets unreferenced relation %q", relationID)
		}
		if pathID == "" {
			return fmt.Errorf("join path selection for relation %q is blank", relationID)
		}
	}
	return nil
}

func (compiler *postgresIntentCompiler) joinCondition(step JoinStep, fromAlias, toAlias string) (string, error) {
	owner, foreignKey, err := findForeignKey(compiler.snapshot, step.ForeignKeyID)
	if err != nil {
		return "", err
	}
	if len(foreignKey.ColumnIDs) != len(foreignKey.ReferencedColumnIDs) || len(foreignKey.ColumnIDs) == 0 {
		return "", fmt.Errorf("foreign key %q has invalid column cardinality", foreignKey.ID)
	}
	parts := make([]string, len(foreignKey.ColumnIDs))
	for index := range foreignKey.ColumnIDs {
		_, local, err := compiler.snapshot.Column(foreignKey.ColumnIDs[index])
		if err != nil {
			return "", err
		}
		_, referenced, err := compiler.snapshot.Column(foreignKey.ReferencedColumnIDs[index])
		if err != nil {
			return "", err
		}
		switch step.Direction {
		case JoinAlongForeignKey:
			if step.FromRelationID != owner.ID || step.ToRelationID != foreignKey.ReferencedRelationID {
				return "", fmt.Errorf("foreign key step does not match along direction")
			}
			parts[index] = fromAlias + "." + quoteIdentifier(local.Name) + " = " + toAlias + "." + quoteIdentifier(referenced.Name)
		case JoinAgainstForeignKey:
			if step.FromRelationID != foreignKey.ReferencedRelationID || step.ToRelationID != owner.ID {
				return "", fmt.Errorf("foreign key step does not match reverse direction")
			}
			parts[index] = fromAlias + "." + quoteIdentifier(referenced.Name) + " = " + toAlias + "." + quoteIdentifier(local.Name)
		default:
			return "", fmt.Errorf("unsupported join direction %q", step.Direction)
		}
	}
	return strings.Join(parts, " AND "), nil
}
