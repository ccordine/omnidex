package datasource

import (
	"fmt"
	"strings"
	"time"
)

func (compiler *postgresIntentCompiler) compileWhere() (string, error) {
	parts := []string{}
	for _, predicate := range compiler.intent.Filters {
		relation, _, err := compiler.snapshot.Column(predicate.FieldID)
		if err != nil {
			return "", err
		}
		part, err := compiler.compilePredicate(predicate, compiler.aliases[relation.ID])
		if err != nil {
			return "", err
		}
		parts = append(parts, part)
	}
	for _, window := range compiler.intent.TemporalWindows {
		relation, column, err := compiler.snapshot.Column(window.FieldID)
		if err != nil {
			return "", err
		}
		asOf, err := time.Parse(time.RFC3339, window.AsOf)
		if err != nil {
			return "", err
		}
		asOfParameter := compiler.addParameter("timestamp", asOf)
		intervalParameter := compiler.addParameter("interval", fmt.Sprintf("%d %ss", window.Amount, window.Unit))
		field := compiler.aliases[relation.ID] + "." + quoteIdentifier(column.Name)
		parts = append(parts, field+" >= "+asOfParameter+" - "+intervalParameter+"::interval AND "+field+" <= "+asOfParameter)
	}
	for index, exists := range compiler.intent.Exists {
		part, err := compiler.compileExistence(exists, index)
		if err != nil {
			return "", err
		}
		parts = append(parts, part)
	}
	if len(parts) == 0 {
		return "", nil
	}
	return " WHERE " + strings.Join(parts, " AND "), nil
}

func (compiler *postgresIntentCompiler) compilePredicate(predicate RelationalPredicate, alias string) (string, error) {
	if alias == "" {
		return "", fmt.Errorf("predicate field has no joined relation alias")
	}
	_, column, err := compiler.snapshot.Column(predicate.FieldID)
	if err != nil {
		return "", err
	}
	field := alias + "." + quoteIdentifier(column.Name)
	switch predicate.Operator {
	case FilterIsNull:
		return field + " IS NULL", nil
	case FilterIsNotNull:
		return field + " IS NOT NULL", nil
	case FilterIn, FilterNotIn:
		placeholders := make([]string, len(predicate.Values))
		for index, literal := range predicate.Values {
			value, err := parseLiteral(literal)
			if err != nil {
				return "", err
			}
			placeholders[index] = compiler.addParameter(string(literal.Type), value)
		}
		keyword := " IN "
		if predicate.Operator == FilterNotIn {
			keyword = " NOT IN "
		}
		return field + keyword + "(" + strings.Join(placeholders, ", ") + ")", nil
	case FilterContains, FilterPrefix:
		value := escapeLikePattern(predicate.Values[0].Value)
		if predicate.Operator == FilterContains {
			value = "%" + value + "%"
		} else {
			value += "%"
		}
		return field + " LIKE " + compiler.addParameter("string", value) + ` ESCAPE '\'`, nil
	default:
		value, err := parseLiteral(predicate.Values[0])
		if err != nil {
			return "", err
		}
		return field + " " + postgresOperator(predicate.Operator) + " " + compiler.addParameter(string(predicate.Values[0].Type), value), nil
	}
}

func (compiler *postgresIntentCompiler) compileExistence(exists ExistencePredicate, index int) (string, error) {
	path, err := ResolveJoinPath(compiler.snapshot, compiler.intent.FromRelationID, exists.RelationID, compiler.selectedJoinPaths[exists.RelationID])
	if err != nil {
		return "", err
	}
	if len(path.Steps) != 1 {
		return "", fmt.Errorf("existence predicates require one direct foreign key; relation %q needs %d joins", exists.RelationID, len(path.Steps))
	}
	target, err := compiler.snapshot.Relation(exists.RelationID)
	if err != nil {
		return "", err
	}
	alias := fmt.Sprintf("e%d", index+1)
	condition, err := compiler.joinCondition(path.Steps[0], "t0", alias)
	if err != nil {
		return "", err
	}
	parts := []string{condition}
	for _, predicate := range exists.Filters {
		part, err := compiler.compilePredicate(predicate, alias)
		if err != nil {
			return "", err
		}
		parts = append(parts, part)
	}
	expression := "EXISTS (SELECT 1 FROM " + quoteQualified(target.Schema, target.Name) + " AS " + alias + " WHERE " + strings.Join(parts, " AND ") + ")"
	if exists.Negated {
		expression = "NOT " + expression
	}
	return expression, nil
}

func postgresOperator(operator FilterOperator) string {
	switch operator {
	case FilterEqual:
		return "="
	case FilterNotEqual:
		return "<>"
	case FilterGT:
		return ">"
	case FilterGTE:
		return ">="
	case FilterLT:
		return "<"
	case FilterLTE:
		return "<="
	default:
		return ""
	}
}

func escapeLikePattern(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `%`, `\%`)
	return strings.ReplaceAll(value, `_`, `\_`)
}
