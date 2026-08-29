package datasource

import (
	"fmt"
	"sort"
	"strings"
)

func validateColumnNames(label string, names []string, columns map[string]struct{}, allowEmpty bool) error {
	if len(names) == 0 && !allowEmpty {
		return fmt.Errorf("%s requires at least one column", label)
	}
	seen := map[string]struct{}{}
	for _, name := range names {
		name = strings.TrimSpace(name)
		if _, exists := columns[name]; !exists {
			return fmt.Errorf("%s references unknown column %q", label, name)
		}
		if _, duplicate := seen[name]; duplicate {
			return fmt.Errorf("%s repeats column %q", label, name)
		}
		seen[name] = struct{}{}
	}
	return nil
}

func sortSchemaMetadata(definition *RelationDefinition) {
	sort.Slice(definition.ForeignKeys, func(i, j int) bool { return definition.ForeignKeys[i].Name < definition.ForeignKeys[j].Name })
	sort.Slice(definition.UniqueConstraints, func(i, j int) bool {
		return definition.UniqueConstraints[i].Name < definition.UniqueConstraints[j].Name
	})
	sort.Slice(definition.CheckConstraints, func(i, j int) bool { return definition.CheckConstraints[i].Name < definition.CheckConstraints[j].Name })
	sort.Slice(definition.Indexes, func(i, j int) bool { return definition.Indexes[i].Name < definition.Indexes[j].Name })
}

func cloneRelationDefinitions(input []RelationDefinition) []RelationDefinition {
	cloned := make([]RelationDefinition, len(input))
	for index, definition := range input {
		cloned[index] = definition
		cloned[index].Columns = append([]ColumnDefinition(nil), definition.Columns...)
		for columnIndex := range cloned[index].Columns {
			cloned[index].Columns[columnIndex].AllowedValues = append([]string(nil), definition.Columns[columnIndex].AllowedValues...)
		}
		cloned[index].PrimaryKey = append([]string(nil), definition.PrimaryKey...)
		cloned[index].ForeignKeys = append([]ForeignKeyDefinition(nil), definition.ForeignKeys...)
		for fkIndex := range cloned[index].ForeignKeys {
			cloned[index].ForeignKeys[fkIndex].Columns = append([]string(nil), definition.ForeignKeys[fkIndex].Columns...)
			cloned[index].ForeignKeys[fkIndex].ReferencedColumns = append([]string(nil), definition.ForeignKeys[fkIndex].ReferencedColumns...)
		}
		cloned[index].UniqueConstraints = append([]UniqueConstraintDefinition(nil), definition.UniqueConstraints...)
		for uniqueIndex := range cloned[index].UniqueConstraints {
			cloned[index].UniqueConstraints[uniqueIndex].Columns = append([]string(nil), definition.UniqueConstraints[uniqueIndex].Columns...)
		}
		cloned[index].CheckConstraints = append([]CheckConstraintDefinition(nil), definition.CheckConstraints...)
		cloned[index].Indexes = append([]IndexDefinition(nil), definition.Indexes...)
		for indexIndex := range cloned[index].Indexes {
			cloned[index].Indexes[indexIndex].Columns = append([]string(nil), definition.Indexes[indexIndex].Columns...)
		}
	}
	return cloned
}

func normalizeNames(names []string) {
	for index := range names {
		names[index] = strings.TrimSpace(names[index])
	}
}

func registerSchemaMetadataName(seen map[string]string, kind, name string) error {
	key := kind + "\x00" + name
	if prior, duplicate := seen[key]; duplicate {
		return fmt.Errorf("duplicate schema metadata name %q used by %s and %s", name, prior, kind)
	}
	seen[key] = kind
	return nil
}

func validRelationKind(kind RelationKind) bool {
	switch kind {
	case RelationTable, RelationPartitionedTable, RelationView, RelationMaterializedView, RelationForeignTable:
		return true
	default:
		return false
	}
}

func validTypeCategory(category ColumnTypeCategory) bool {
	switch category {
	case TypeInteger, TypeDecimal, TypeText, TypeBoolean, TypeTemporal, TypeDate, TypeUUID, TypeJSON, TypeBinary, TypeOther:
		return true
	default:
		return false
	}
}
