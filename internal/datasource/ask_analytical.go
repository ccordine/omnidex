package datasource

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/omni"
)

type AnalyticalAskInput struct {
	Connection Connection
	Profile    Profile
	SourceID   string
	SourceName string
	Question   string
	Catalog    SchemaCatalog
	HasCatalog bool
}

// AnalyticalAsk runs a multi-step investigation with caveman-minified hard facts between queries.
func AnalyticalAsk(ctx context.Context, input AnalyticalAskInput, llm omni.DBManagerLLMClient) (QueryResult, SchemaCatalog, error) {
	input.Profile = NormalizeProfile(input.Profile)
	question := strings.TrimSpace(input.Question)
	if question == "" {
		return QueryResult{}, SchemaCatalog{}, fmt.Errorf("question is required")
	}
	if llm == nil {
		return QueryResult{}, SchemaCatalog{}, fmt.Errorf("llm client is required")
	}

	catalog := input.Catalog
	if !input.HasCatalog || len(catalog.Tables) == 0 {
		built, err := BuildSchemaCatalog(ctx, input.Connection, input.Profile, input.SourceID, input.SourceName, llm)
		if err != nil {
			return QueryResult{}, SchemaCatalog{}, err
		}
		catalog = built
	}

	pool, err := ConnectReadOnly(ctx, input.Connection)
	if err != nil {
		return QueryResult{}, catalog, err
	}
	defer pool.Close()
	runner := omni.NewPgxMemoryRunner(pool)

	result, err := runInvestigation(ctx, input, catalog, runner, llm)
	if err != nil {
		return QueryResult{}, catalog, err
	}
	result = FinalizeQueryResult(result)
	return result, catalog, nil
}

func selectRelevantTables(catalog SchemaCatalog, question string, max int) []TableCatalog {
	if max <= 0 {
		max = 16
	}
	question = strings.ToLower(question)
	tokens := strings.FieldsFunc(question, func(r rune) bool {
		return !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9') && r != '_'
	})
	type scoredTable struct {
		table TableCatalog
		score int
	}
	items := make([]scoredTable, 0, len(catalog.Tables))
	for _, table := range catalog.Tables {
		score := 0
		blob := strings.ToLower(table.FullName + " " + table.Purpose)
		for _, col := range table.Columns {
			blob += " " + strings.ToLower(col.Name+" "+col.Hint)
		}
		for _, token := range tokens {
			if len(token) < 3 {
				continue
			}
			if strings.Contains(blob, token) {
				score += 2
			}
		}
		if table.RowEstimate > 0 {
			score++
		}
		items = append(items, scoredTable{table: table, score: score})
	}
	for i := 0; i < len(items); i++ {
		for j := i + 1; j < len(items); j++ {
			if items[j].score > items[i].score {
				items[i], items[j] = items[j], items[i]
			}
		}
	}
	out := make([]TableCatalog, 0, max)
	for _, item := range items {
		if len(out) >= max {
			break
		}
		out = append(out, item.table)
	}
	if len(out) == 0 {
		limit := catalog.Tables
		if len(limit) > max {
			limit = limit[:max]
		}
		return limit
	}
	return out
}

func EnsureCatalog(ctx context.Context, store CatalogStore, writer MemoryWriter, conn Connection, profile Profile, sourceID, sourceName string, llm omni.DBManagerLLMClient) (SchemaCatalog, error) {
	if store != nil {
		if existing, ok, err := store.Get(ctx, sourceID); err == nil && ok && len(existing.Tables) > 0 {
			return existing, nil
		}
	}
	catalog, err := BuildSchemaCatalog(ctx, conn, profile, sourceID, sourceName, llm)
	if err != nil {
		return SchemaCatalog{}, err
	}
	catalog.UpdatedAt = time.Now().UTC()
	if store != nil {
		if err := store.Save(ctx, catalog); err != nil {
			return SchemaCatalog{}, err
		}
	}
	_ = PersistCatalogMemories(ctx, writer, catalog)
	return catalog, nil
}
