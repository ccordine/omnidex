package queue

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

func TestReadCodingPlanObtainsHeaderAndOrderedLeavesInOneStatement(t *testing.T) {
	t.Parallel()
	statement := "A user can confirm the item."
	leafID, err := model.NewCodingPlanLeafID(statement)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	query := &codingPlanSingleRowQuery{row: codingPlanSingleRow{
		jobID: 11, generation: 3, revision: 7,
		state: model.CodingPlanStateReview, scopeMode: model.CodingScopeModeNormal,
		requestSHA256: strings.Repeat("a", 64), createdAt: now, updatedAt: now,
		leavesJSON: fmt.Sprintf(
			`[{"id":%q,"statement":%q,"annotation":"grounded","decision":"approved"}]`,
			leafID,
			statement,
		),
	}}
	plan, err := readCodingPlan(context.Background(), query, 11, 3)
	if err != nil {
		t.Fatal(err)
	}
	if query.calls != 1 || !strings.Contains(query.statement, "jsonb_agg") ||
		!strings.Contains(query.statement, "ORDER BY leaf.sort_index") {
		t.Fatalf("coding plan snapshot used %d statements:\n%s", query.calls, query.statement)
	}
	if plan.Revision != 7 || len(plan.Leaves) != 1 || plan.Leaves[0].ID != leafID ||
		plan.Leaves[0].Decision != model.CodingPlanDecisionApproved {
		t.Fatalf("coding plan snapshot = %#v", plan)
	}
}

type codingPlanSingleRowQuery struct {
	calls     int
	statement string
	row       codingPlanSingleRow
}

func (query *codingPlanSingleRowQuery) QueryRow(
	_ context.Context,
	statement string,
	_ ...any,
) pgx.Row {
	query.calls++
	query.statement = statement
	return query.row
}

type codingPlanSingleRow struct {
	jobID, generation, revision int64
	state                       model.CodingPlanState
	scopeMode                   model.CodingScopeMode
	requestSHA256, leavesJSON   string
	createdAt, updatedAt        time.Time
	frozenAt                    *time.Time
}

func (row codingPlanSingleRow) Scan(destinations ...any) error {
	if len(destinations) != 10 {
		return fmt.Errorf("coding plan row received %d destinations", len(destinations))
	}
	*destinations[0].(*int64) = row.jobID
	*destinations[1].(*int64) = row.generation
	*destinations[2].(*int64) = row.revision
	*destinations[3].(*model.CodingPlanState) = row.state
	*destinations[4].(*model.CodingScopeMode) = row.scopeMode
	*destinations[5].(*string) = row.requestSHA256
	*destinations[6].(*time.Time) = row.createdAt
	*destinations[7].(*time.Time) = row.updatedAt
	*destinations[8].(**time.Time) = row.frozenAt
	*destinations[9].(*[]byte) = []byte(row.leavesJSON)
	return nil
}
