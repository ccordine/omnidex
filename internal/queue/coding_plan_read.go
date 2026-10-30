package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

func (r *Repository) ReadCodingLedger(ctx context.Context, ledgerID int64, options CodingLedgerReadOptions) (CodingLedgerRevision, error) {
	if r == nil || r.pool == nil || ledgerID <= 0 {
		return CodingLedgerRevision{}, fmt.Errorf("coding ledger repository and identity are required")
	}
	if options.Limit < 0 {
		return CodingLedgerRevision{}, fmt.Errorf("coding ledger read limit cannot be negative")
	}
	if options.Offset < 0 {
		return CodingLedgerRevision{}, fmt.Errorf("coding ledger read offset cannot be negative")
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return CodingLedgerRevision{}, err
	}
	defer tx.Rollback(ctx)
	var currentState string
	if options.Revision == 0 {
		if err := tx.QueryRow(ctx, `SELECT current_revision,state FROM coding_ledgers WHERE id=$1`, ledgerID).Scan(&options.Revision, &currentState); err != nil {
			return CodingLedgerRevision{}, err
		}
	}
	result, err := readCodingLedgerRevisionTx(ctx, tx, ledgerID, options.Revision, options)
	if err != nil {
		return CodingLedgerRevision{}, err
	}
	if currentState == "revising" {
		result.State = currentState
	}
	if err := tx.Commit(ctx); err != nil {
		return CodingLedgerRevision{}, err
	}
	return result, nil
}

func (r *Repository) CurrentCodingLedgerForJob(ctx context.Context, jobID int64) (CodingLedgerRevision, bool, error) {
	if r == nil || r.pool == nil || jobID <= 0 {
		return CodingLedgerRevision{}, false, fmt.Errorf("coding ledger requires repository and job identity")
	}
	var ledgerID int64
	if err := r.pool.QueryRow(ctx, `SELECT id FROM coding_ledgers WHERE job_id=$1`, jobID).Scan(&ledgerID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return CodingLedgerRevision{}, false, nil
		}
		return CodingLedgerRevision{}, false, err
	}
	result, err := r.readCompleteCodingLedger(ctx, ledgerID, 0)
	if err == nil {
		result.PlanningMessages, err = r.codingLedgerItemMessages(ctx, jobID)
	}
	return result, err == nil, err
}

func (r *Repository) codingLedgerItemMessages(ctx context.Context, jobID int64) ([]model.LedgerPlanningMessage, error) {
	rows, err := r.pool.Query(ctx, `SELECT payload_json->>'task_id',payload_json->'notes' FROM evidence WHERE job_id=$1 AND kind='coding_agent' AND payload_json->>'phase'='planning_draft' AND COALESCE(payload_json->>'task_id','')<>'' ORDER BY id`, jobID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var messages []model.LedgerPlanningMessage
	seen := map[string]bool{}
	for rows.Next() {
		var itemID model.LedgerItemID
		var raw []byte
		if err := rows.Scan(&itemID, &raw); err != nil {
			return nil, err
		}
		var notes []string
		if err := json.Unmarshal(raw, &notes); err != nil {
			return nil, err
		}
		if len(notes) != 3 || seen[notes[0]] {
			continue
		}
		seen[notes[0]] = true
		messages = append(messages, model.LedgerPlanningMessage{ItemID: itemID, Role: "user", Text: notes[1]}, model.LedgerPlanningMessage{ItemID: itemID, Role: "assistant", Text: notes[2]})
	}
	return messages, rows.Err()
}

func (r *Repository) CodingLedgerRevisionForOperation(ctx context.Context, ledgerID int64, operationID string) (int64, bool, error) {
	if r == nil || r.pool == nil || ledgerID <= 0 || strings.TrimSpace(operationID) == "" {
		return 0, false, fmt.Errorf("coding ledger operation lookup requires repository, ledger, and operation identity")
	}
	var revision int64
	err := r.pool.QueryRow(ctx, `SELECT revision FROM coding_ledger_revisions WHERE ledger_id=$1 AND operation_id=$2`, ledgerID, operationID).Scan(&revision)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, false, nil
	}
	return revision, err == nil, err
}

func codingLedgerLiteralSearchPattern(search string) string {
	escaped := strings.NewReplacer(
		`\`, `\\`,
		"%", `\%`,
		"_", `\_`,
	).Replace(strings.ToLower(search))
	return "%" + escaped + "%"
}

func readCodingLedgerRevisionTx(ctx context.Context, tx pgx.Tx, ledgerID, revision int64, options CodingLedgerReadOptions) (CodingLedgerRevision, error) {
	var result CodingLedgerRevision
	var rawMutation []byte
	result.LedgerID = ledgerID
	if err := tx.QueryRow(ctx, `
		SELECT ledger.job_id,job.current_generation,revision.revision,revision.base_revision,revision.operation_id,
		       revision.state,COALESCE(revision.owner_turn_id,''),revision.mutation_payload,
		       ledger.created_at,ledger.updated_at
		FROM coding_ledgers ledger
		JOIN jobs job ON job.id=ledger.job_id
		JOIN coding_ledger_revisions revision ON revision.ledger_id=ledger.id
		WHERE ledger.id=$1 AND revision.revision=$2
	`, ledgerID, revision).Scan(&result.JobID, &result.Generation, &result.Revision, &result.BaseRevision,
		&result.OperationID, &result.State, &result.OwnerTurnID, &rawMutation,
		&result.CreatedAt, &result.UpdatedAt); err != nil {
		return CodingLedgerRevision{}, err
	}
	pending, err := pendingCodingPlanningAmendmentsTx(ctx, tx, result.JobID)
	if err != nil {
		return CodingLedgerRevision{}, err
	}
	result.PendingAmendments = pending
	if result.OwnerTurnID != "" {
		var rawDispositions []byte
		err := tx.QueryRow(ctx, `
			SELECT statements FROM coding_ledger_owner_turn_dispositions
			WHERE ledger_id=$1 AND owner_turn_id=$2 AND revision=$3
		`, ledgerID, result.OwnerTurnID, revision).Scan(&rawDispositions)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return CodingLedgerRevision{}, err
		}
		if len(rawDispositions) > 0 {
			if err := json.Unmarshal(rawDispositions, &result.OwnerTurnDispositions); err != nil {
				return CodingLedgerRevision{}, fmt.Errorf("decode owner-turn dispositions: %w", err)
			}
		}
	}
	result.Ledger = model.EngineeringLedger{ID: codingLedgerExternalID(ledgerID), Revision: revision}
	limit := options.Limit
	if limit == 0 {
		limit = 200
	}
	if limit > 1000 {
		limit = 1000
	}
	var sqlLimit any
	if limit >= 0 {
		sqlLimit = limit
	}
	offset := options.Offset
	if offset < 0 {
		offset = 0
	}
	var kinds any
	if len(options.Kinds) > 0 {
		kinds = options.Kinds
	}
	var itemIDs any
	if len(options.ItemIDs) > 0 {
		ids := make([]string, len(options.ItemIDs))
		for index := range options.ItemIDs {
			ids[index] = string(options.ItemIDs[index])
		}
		itemIDs = ids
	}
	var disposition any
	if options.Removed != nil {
		disposition = "active"
		if *options.Removed {
			disposition = "tombstoned"
		}
	}
	var reviewState any
	if options.ReviewState != "" {
		reviewState = string(options.ReviewState)
	}
	var search any
	if options.Search != "" {
		search = codingLedgerLiteralSearchPattern(options.Search)
	}
	var relatedTask any
	if options.RelatedTaskID != "" {
		relatedTask = string(options.RelatedTaskID)
	}
	rows, err := tx.Query(ctx, `
 WITH records AS (
 SELECT item.item_kind AS kind,projection.item_id AS id,version.rank,version.payload
 FROM coding_ledger_revision_items projection
 JOIN coding_ledger_items item USING(ledger_id,item_id)
 JOIN coding_ledger_item_versions version ON (version.ledger_id,version.item_id,version.revision)=(projection.ledger_id,projection.item_id,projection.item_revision)
 WHERE projection.ledger_id=$1 AND projection.revision=$2
 )
 SELECT kind,payload FROM records
 WHERE ($3::text[] IS NULL OR kind=ANY($3))
 AND ($4::text IS NULL OR payload->>'disposition'=$4)
 AND ($5::text IS NULL OR payload->>'review_state'=$5)
 AND ($6::text IS NULL OR lower(payload::text) LIKE $6 ESCAPE E'\\')
 AND ($7::text[] IS NULL OR id=ANY($7))
 AND ($8::text IS NULL OR id=$8)
 ORDER BY rank,id LIMIT $9 OFFSET $10
 `, ledgerID, revision, kinds, disposition, reviewState, search, itemIDs, relatedTask, sqlLimit, offset)
	if err != nil {
		return CodingLedgerRevision{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var kind string
		var raw []byte
		if err := rows.Scan(&kind, &raw); err != nil {
			return CodingLedgerRevision{}, err
		}
		if kind != "task" {
			return CodingLedgerRevision{}, fmt.Errorf("unsupported ledger item kind %q", kind)
		}
		var task model.LedgerTask
		if err := json.Unmarshal(raw, &task); err != nil {
			return CodingLedgerRevision{}, err
		}
		result.Ledger.Tasks = append(result.Ledger.Tasks, task)

	}
	if err := rows.Err(); err != nil {
		return CodingLedgerRevision{}, err
	}
	routeRows, err := tx.Query(ctx, `
		SELECT role,configuration_revision,provider,model,reasoning_effort,service_tier
		FROM coding_ledger_routes WHERE ledger_id=$1 AND revision=$2 ORDER BY role
	`, ledgerID, revision)
	if err != nil {
		return CodingLedgerRevision{}, err
	}
	for routeRows.Next() {
		var route CodingLedgerRoute
		if err := routeRows.Scan(&route.Role, &route.ConfigurationRevision, &route.Provider, &route.Model,
			&route.ReasoningEffort, &route.ServiceTier); err != nil {
			routeRows.Close()
			return CodingLedgerRevision{}, err
		}
		result.Routes = append(result.Routes, route)
	}
	if err := routeRows.Err(); err != nil {
		routeRows.Close()
		return CodingLedgerRevision{}, err
	}
	routeRows.Close()
	result.LatestDelta = codingLedgerLatestDelta(result, rawMutation)
	return result, nil
}

func codingLedgerLatestDelta(revision CodingLedgerRevision, rawMutation []byte) CodingLedgerDiff {
	delta := CodingLedgerDiff{LedgerID: revision.LedgerID, FromRevision: revision.BaseRevision, ToRevision: revision.Revision}
	if len(rawMutation) == 0 && revision.BaseRevision == 0 {
		for _, item := range planningLedgerItemsForDelta(revision.Ledger) {
			delta.Added = append(delta.Added, item)
		}
		return delta
	}
	var mutation model.LedgerMutation
	if json.Unmarshal(rawMutation, &mutation) != nil {
		return delta
	}
	for _, operation := range mutation.Operations {
		id := operation.TargetID
		if operation.Kind == model.LedgerMutationAdd && operation.Item != nil {
			id = codingLedgerMutationItemID(*operation.Item)
		}
		if id == "" {
			continue
		}
		switch operation.Kind {
		case model.LedgerMutationAdd:
			delta.Added = append(delta.Added, id)
		case model.LedgerMutationRemove:
			delta.Removed = append(delta.Removed, id)
		case model.LedgerMutationRestore:
			delta.Restored = append(delta.Restored, id)
		default:
			delta.Amended = append(delta.Amended, id)
		}
	}
	return delta
}

func codingLedgerMutationItemID(item model.LedgerMutationItem) model.LedgerItemID {
	if item.Task != nil {
		return item.Task.ID
	}
	return ""
}

func planningLedgerItemsForDelta(ledger model.EngineeringLedger) []model.LedgerItemID {
	result := make([]model.LedgerItemID, 0)
	for _, record := range ledgerRepositoryRecords(ledger) {
		result = append(result, record.item.ID)
	}

	return result
}

func (r *Repository) CodingLedgerItemHistory(ctx context.Context, ledgerID int64, itemID model.LedgerItemID, limit int, offsets ...int) ([]CodingLedgerItemVersion, error) {
	if r == nil || r.pool == nil || ledgerID <= 0 || strings.TrimSpace(string(itemID)) == "" {
		return nil, fmt.Errorf("coding ledger history requires repository, ledger, and item identities")
	}
	if len(offsets) > 1 {
		return nil, fmt.Errorf("coding ledger item history accepts at most one offset")
	}
	offset := 0
	if len(offsets) == 1 {
		offset = offsets[0]
	}
	if offset < 0 {
		return nil, fmt.Errorf("coding ledger item history offset cannot be negative")
	}
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}
	rows, err := r.pool.Query(ctx, `
		SELECT item.item_id,item.item_kind,version.revision,
		       CASE version.disposition WHEN 'removed' THEN 'tombstoned' ELSE version.disposition END,
		       COALESCE(version.review_state,''),version.rank,version.payload,version.rationale
		FROM coding_ledger_items item
		JOIN coding_ledger_item_versions version USING(ledger_id,item_id)
		WHERE item.ledger_id=$1 AND item.item_id=$2
		ORDER BY version.revision DESC LIMIT $3 OFFSET $4
	`, ledgerID, itemID, limit, offset)
	if err != nil {
		return nil, err
	}
	result := make([]CodingLedgerItemVersion, 0)
	for rows.Next() {
		var value CodingLedgerItemVersion
		if err := rows.Scan(&value.ItemID, &value.Kind, &value.Revision, &value.Disposition,
			&value.ReviewState, &value.Rank, &value.Payload, &value.Rationale); err != nil {
			rows.Close()
			return nil, err
		}
		result = append(result, value)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	if len(result) != 0 {
		return result, nil
	}
	return result, nil
}

func (r *Repository) CodingLedgerDiff(ctx context.Context, ledgerID, fromRevision, toRevision int64, pages ...CodingLedgerPage) (CodingLedgerDiff, error) {
	if r == nil || r.pool == nil || ledgerID <= 0 || fromRevision <= 0 || toRevision <= 0 {
		return CodingLedgerDiff{}, fmt.Errorf("ledger diff requires exact revisions")
	}
	page := CodingLedgerPage{Limit: 200}
	if len(pages) > 1 {
		return CodingLedgerDiff{}, fmt.Errorf("ledger diff accepts one page")
	}
	if len(pages) == 1 {
		page = pages[0]
	}
	if page.Limit == 0 {
		page.Limit = 200
	}
	if page.Limit < 0 || page.Offset < 0 {
		return CodingLedgerDiff{}, fmt.Errorf("ledger diff page cannot be negative")
	}
	if page.Limit > 1000 {
		page.Limit = 1000
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return CodingLedgerDiff{}, err
	}
	defer tx.Rollback(ctx)
	before, err := readCodingLedgerRevisionTx(ctx, tx, ledgerID, fromRevision, CodingLedgerReadOptions{Limit: -1})
	if err != nil {
		return CodingLedgerDiff{}, err
	}
	after, err := readCodingLedgerRevisionTx(ctx, tx, ledgerID, toRevision, CodingLedgerReadOptions{Limit: -1})
	if err != nil {
		return CodingLedgerDiff{}, err
	}
	old := map[model.LedgerItemID]model.LedgerTask{}
	next := map[model.LedgerItemID]model.LedgerTask{}
	ids := []model.LedgerItemID{}
	for _, task := range before.Ledger.Tasks {
		old[task.ID] = task
		ids = append(ids, task.ID)
	}
	for _, task := range after.Ledger.Tasks {
		next[task.ID] = task
		if _, exists := old[task.ID]; !exists {
			ids = append(ids, task.ID)
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	result := CodingLedgerDiff{LedgerID: ledgerID, FromRevision: fromRevision, ToRevision: toRevision, Limit: page.Limit, Offset: page.Offset}
	count := 0
	for _, id := range ids {
		a, was := old[id]
		b, is := next[id]
		if was && is && reflect.DeepEqual(a, b) {
			continue
		}
		count++
		if count <= page.Offset {
			continue
		}
		if count > page.Offset+page.Limit {
			result.HasMore = true
			break
		}
		switch {
		case !was:
			result.Added = append(result.Added, id)
		case !is || b.Disposition == model.LedgerDispositionTombstoned:
			result.Removed = append(result.Removed, id)
		case a.Disposition == model.LedgerDispositionTombstoned:
			result.Restored = append(result.Restored, id)
		default:
			result.Amended = append(result.Amended, id)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return CodingLedgerDiff{}, err
	}
	return result, nil
}

func (r *Repository) CodingLedgerTaskDetail(
	ctx context.Context,
	ledgerID, revision int64,
	taskID model.LedgerItemID,
	pages ...CodingLedgerPage,
) (CodingLedgerRevision, error) {
	page := CodingLedgerPage{Limit: 200}
	if len(pages) > 1 {
		return CodingLedgerRevision{}, fmt.Errorf("coding ledger task detail accepts at most one related-record page")
	}
	if len(pages) == 1 {
		page = pages[0]
	}
	if page.Limit == 0 {
		page.Limit = 200
	}
	if page.Limit < 0 || page.Offset < 0 {
		return CodingLedgerRevision{}, fmt.Errorf("coding ledger task detail page cannot be negative")
	}
	if page.Limit > 1000 {
		page.Limit = 1000
	}
	active := false
	task, err := r.ReadCodingLedger(ctx, ledgerID, CodingLedgerReadOptions{
		Revision: revision, Limit: 1, Kinds: []string{"task"}, Removed: &active,
		ItemIDs: []model.LedgerItemID{taskID},
	})
	if err != nil {
		return CodingLedgerRevision{}, err
	}
	return task, nil
}

func (r *Repository) CodingLedgerRevisionHistory(ctx context.Context, ledgerID int64, limit int, offsets ...int) ([]CodingLedgerRevision, error) {
	if r == nil || r.pool == nil || ledgerID <= 0 {
		return nil, fmt.Errorf("coding ledger revision history requires repository and ledger identity")
	}
	if len(offsets) > 1 {
		return nil, fmt.Errorf("coding ledger revision history accepts at most one offset")
	}
	offset := 0
	if len(offsets) == 1 {
		offset = offsets[0]
	}
	if offset < 0 {
		return nil, fmt.Errorf("coding ledger revision history offset cannot be negative")
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	rows, err := r.pool.Query(ctx, `
		SELECT revision FROM coding_ledger_revisions
		WHERE ledger_id=$1 ORDER BY revision DESC LIMIT $2 OFFSET $3
	`, ledgerID, limit, offset)
	if err != nil {
		return nil, err
	}
	revisions := make([]int64, 0)
	for rows.Next() {
		var revision int64
		if err := rows.Scan(&revision); err != nil {
			rows.Close()
			return nil, err
		}
		revisions = append(revisions, revision)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	result := make([]CodingLedgerRevision, 0, len(revisions))
	for _, revision := range revisions {
		value, err := r.ReadCodingLedger(ctx, ledgerID, CodingLedgerReadOptions{Revision: revision, Limit: 1})
		if err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, nil
}
