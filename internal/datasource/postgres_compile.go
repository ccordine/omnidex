package datasource

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

type postgresIntentCompiler struct {
	snapshot          SchemaSnapshot
	intent            RelationalIntent
	aliases           map[string]string
	joins             []string
	params            []CompiledParameter
	selectedJoinPaths map[string]string
}

func CompilePostgres(snapshot SchemaSnapshot, intent RelationalIntent) (CompiledQuery, error) {
	return CompilePostgresWithJoinPaths(snapshot, intent, nil)
}

func CompilePostgresWithJoinPaths(snapshot SchemaSnapshot, intent RelationalIntent, selected map[string]string) (CompiledQuery, error) {
	if err := snapshot.ValidateIntegrity(); err != nil {
		return CompiledQuery{}, err
	}
	if err := intent.Validate(snapshot); err != nil {
		return CompiledQuery{}, err
	}
	if err := validateSelectedJoinTargets(snapshot, intent, selected); err != nil {
		return CompiledQuery{}, err
	}
	compiler := &postgresIntentCompiler{snapshot: snapshot, intent: intent, aliases: map[string]string{intent.FromRelationID: "t0"}, selectedJoinPaths: selected}
	if err := compiler.planJoins(); err != nil {
		return CompiledQuery{}, err
	}
	fromRelation, err := snapshot.Relation(intent.FromRelationID)
	if err != nil {
		return CompiledQuery{}, err
	}
	fromClause := " FROM " + quoteQualified(fromRelation.Schema, fromRelation.Name) + " AS t0"
	if len(compiler.joins) > 0 {
		fromClause += " " + strings.Join(compiler.joins, " ")
	}
	where, err := compiler.compileWhere()
	if err != nil {
		return CompiledQuery{}, err
	}
	outputs := []CompiledOutput{}
	var sqlText string
	if intent.Shape == ResultExistence {
		sqlText = "SELECT EXISTS (SELECT 1" + fromClause + where + `) AS "c1"`
		outputs = append(outputs, CompiledOutput{Name: "c1", TypeCategory: TypeBoolean})
	} else {
		selectParts := make([]string, len(intent.Projections))
		for index, projection := range intent.Projections {
			expression, output, err := compiler.compileProjection(projection, index)
			if err != nil {
				return CompiledQuery{}, err
			}
			selectParts[index] = expression + " AS " + quoteIdentifier(output.Name)
			outputs = append(outputs, output)
		}
		sqlText = "SELECT " + strings.Join(selectParts, ", ") + fromClause + where
		if len(intent.GroupBy) > 0 {
			groups := make([]string, len(intent.GroupBy))
			for index, projectionIndex := range intent.GroupBy {
				groups[index], _, err = compiler.compileProjection(intent.Projections[projectionIndex], projectionIndex)
				if err != nil {
					return CompiledQuery{}, err
				}
			}
			sqlText += " GROUP BY " + strings.Join(groups, ", ")
		}
		having, err := compiler.compileHaving()
		if err != nil {
			return CompiledQuery{}, err
		}
		sqlText += having
		if len(intent.OrderBy) > 0 {
			orders := make([]string, len(intent.OrderBy))
			for index, order := range intent.OrderBy {
				orders[index] = fmt.Sprintf("%d %s", order.Projection+1, strings.ToUpper(string(order.Direction)))
			}
			sqlText += " ORDER BY " + strings.Join(orders, ", ")
		}
	}
	limitPlaceholder := compiler.addParameter("integer", int64(intent.Limit))
	sqlText += " LIMIT " + limitPlaceholder
	intentHash, err := hashRelationalIntent(intent)
	if err != nil {
		return CompiledQuery{}, err
	}
	queryDigest := sha256.Sum256([]byte(sqlText))
	compiled := CompiledQuery{
		Schema: CompiledQueryV1, SourceID: snapshot.SourceID, SchemaFingerprint: snapshot.Fingerprint,
		IntentHash: intentHash, QueryHash: hex.EncodeToString(queryDigest[:]), SQL: sqlText,
		Parameters: compiler.params, Outputs: outputs, Limit: intent.Limit,
	}
	compiled.seal = compiledQuerySeal(compiled)
	return compiled, nil
}

func (compiler *postgresIntentCompiler) compileProjection(projection RelationalProjection, index int) (string, CompiledOutput, error) {
	output := CompiledOutput{Name: fmt.Sprintf("c%d", index+1), FieldID: projection.FieldID, Aggregate: projection.Aggregate}
	if projection.Aggregate != "" {
		expression, category, err := compiler.aggregateExpression(projection.Aggregate, projection.FieldID)
		output.TypeCategory = category
		return expression, output, err
	}
	relation, column, err := compiler.snapshot.Column(projection.FieldID)
	if err != nil {
		return "", CompiledOutput{}, err
	}
	alias, exists := compiler.aliases[relation.ID]
	if !exists {
		return "", CompiledOutput{}, fmt.Errorf("relation %q has no planned join alias", relation.ID)
	}
	expression := alias + "." + quoteIdentifier(column.Name)
	if projection.TimeBucket != "" {
		expression = "DATE_TRUNC('" + string(projection.TimeBucket) + "', " + expression + ")"
		output.TypeCategory = TypeTemporal
	} else {
		output.TypeCategory = column.TypeCategory
	}
	return expression, output, nil
}

func (compiler *postgresIntentCompiler) aggregateExpression(operation AggregateOperation, fieldID string) (string, ColumnTypeCategory, error) {
	if operation == AggregateCountRows {
		return "COUNT(*)", TypeInteger, nil
	}
	relation, column, err := compiler.snapshot.Column(fieldID)
	if err != nil {
		return "", "", err
	}
	alias, exists := compiler.aliases[relation.ID]
	if !exists {
		return "", "", fmt.Errorf("relation %q has no planned join alias", relation.ID)
	}
	field := alias + "." + quoteIdentifier(column.Name)
	switch operation {
	case AggregateCount:
		return "COUNT(" + field + ")", TypeInteger, nil
	case AggregateCountDistinct:
		return "COUNT(DISTINCT " + field + ")", TypeInteger, nil
	case AggregateSum:
		return "SUM(" + field + ")", TypeDecimal, nil
	case AggregateAverage:
		return "AVG(" + field + ")", TypeDecimal, nil
	case AggregateMinimum:
		return "MIN(" + field + ")", column.TypeCategory, nil
	case AggregateMaximum:
		return "MAX(" + field + ")", column.TypeCategory, nil
	default:
		return "", "", fmt.Errorf("unsupported aggregate %q", operation)
	}
}

func (compiler *postgresIntentCompiler) compileHaving() (string, error) {
	if len(compiler.intent.Having) == 0 {
		return "", nil
	}
	parts := make([]string, len(compiler.intent.Having))
	for index, predicate := range compiler.intent.Having {
		expression, _, err := compiler.aggregateExpression(predicate.Aggregate, predicate.FieldID)
		if err != nil {
			return "", err
		}
		value, err := parseLiteral(predicate.Value)
		if err != nil {
			return "", err
		}
		parts[index] = expression + " " + postgresOperator(predicate.Operator) + " " + compiler.addParameter(string(predicate.Value.Type), value)
	}
	return " HAVING " + strings.Join(parts, " AND "), nil
}

func (compiler *postgresIntentCompiler) addParameter(kind string, value any) string {
	position := len(compiler.params) + 1
	compiler.params = append(compiler.params, CompiledParameter{Position: position, Type: kind, value: value})
	return fmt.Sprintf("$%d", position)
}

func quoteIdentifier(identifier string) string {
	return `"` + strings.ReplaceAll(identifier, `"`, `""`) + `"`
}

func quoteQualified(schema, relation string) string {
	return quoteIdentifier(schema) + "." + quoteIdentifier(relation)
}

func hashRelationalIntent(intent RelationalIntent) (string, error) {
	normalizeIntentSlices(&intent)
	encoded, err := json.Marshal(intent)
	if err != nil {
		return "", fmt.Errorf("encode relational intent: %w", err)
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

func normalizeIntentSlices(intent *RelationalIntent) {
	if intent.Projections == nil {
		intent.Projections = []RelationalProjection{}
	}
	if intent.Filters == nil {
		intent.Filters = []RelationalPredicate{}
	}
	if intent.TemporalWindows == nil {
		intent.TemporalWindows = []TemporalWindow{}
	}
	if intent.Exists == nil {
		intent.Exists = []ExistencePredicate{}
	}
	if intent.GroupBy == nil {
		intent.GroupBy = []int{}
	}
	if intent.Having == nil {
		intent.Having = []AggregatePredicate{}
	}
	if intent.OrderBy == nil {
		intent.OrderBy = []OrderTerm{}
	}
	for index := range intent.Filters {
		if intent.Filters[index].Values == nil {
			intent.Filters[index].Values = []IntentLiteral{}
		}
	}
	for index := range intent.Exists {
		if intent.Exists[index].Filters == nil {
			intent.Exists[index].Filters = []RelationalPredicate{}
		}
	}
}

func compiledQuerySeal(query CompiledQuery) [32]byte {
	parts := []string{
		query.Schema, query.SourceID, query.SchemaFingerprint, query.IntentHash,
		query.QueryHash, query.SQL, fmt.Sprintf("%d", query.Limit),
	}
	for _, parameter := range query.Parameters {
		parts = append(parts, fmt.Sprintf("%d", parameter.Position), parameter.Type, fmt.Sprintf("%T:%v", parameter.value, parameter.value))
	}
	for _, output := range query.Outputs {
		parts = append(parts, output.Name, output.FieldID, string(output.Aggregate), string(output.TypeCategory))
	}
	return sha256.Sum256([]byte(strings.Join(parts, "\x00")))
}

func requiredJoinRelationIDs(snapshot SchemaSnapshot, intent RelationalIntent) ([]string, error) {
	ids := map[string]struct{}{}
	addField := func(fieldID string) error {
		if fieldID == "" {
			return nil
		}
		relation, _, err := snapshot.Column(fieldID)
		if err != nil {
			return err
		}
		if relation.ID != intent.FromRelationID {
			ids[relation.ID] = struct{}{}
		}
		return nil
	}
	for _, projection := range intent.Projections {
		if err := addField(projection.FieldID); err != nil {
			return nil, err
		}
	}
	for _, predicate := range intent.Filters {
		if err := addField(predicate.FieldID); err != nil {
			return nil, err
		}
	}
	for _, window := range intent.TemporalWindows {
		if err := addField(window.FieldID); err != nil {
			return nil, err
		}
	}
	for _, predicate := range intent.Having {
		if err := addField(predicate.FieldID); err != nil {
			return nil, err
		}
	}
	result := make([]string, 0, len(ids))
	for id := range ids {
		result = append(result, id)
	}
	sort.Strings(result)
	return result, nil
}
