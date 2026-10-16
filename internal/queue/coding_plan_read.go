package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	if options.Revision == 0 {
		if err := tx.QueryRow(ctx, `SELECT current_revision FROM coding_ledgers WHERE id=$1`, ledgerID).Scan(&options.Revision); err != nil {
			return CodingLedgerRevision{}, err
		}
	}
	result, err := readCodingLedgerRevisionTx(ctx, tx, ledgerID, options.Revision, options)
	if err != nil {
		return CodingLedgerRevision{}, err
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
	return result, err == nil, err
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
			SELECT item.item_kind AS kind,projection.item_id AS id,
			       version.rank,version.payload
			FROM coding_ledger_revision_items projection
			JOIN coding_ledger_items item USING(ledger_id,item_id)
			JOIN coding_ledger_item_versions version
			  ON (version.ledger_id,version.item_id,version.revision)=
			     (projection.ledger_id,projection.item_id,projection.item_revision)
			WHERE projection.ledger_id=$1 AND projection.revision=$2
			  AND item.item_id NOT LIKE 'dependency.record.%'
			UNION ALL
			SELECT 'dependency',version.payload->>'id',
			       COALESCE((version.payload->>'rank')::bigint,0),version.payload
			FROM coding_ledger_revision_items projection
			JOIN coding_ledger_items item USING(ledger_id,item_id)
			JOIN coding_ledger_item_versions version
			  ON (version.ledger_id,version.item_id,version.revision)=
			     (projection.ledger_id,projection.item_id,projection.item_revision)
			WHERE projection.ledger_id=$1 AND projection.revision=$2
			  AND item.item_id LIKE 'dependency.record.%'
		),
		selected_task AS (
			SELECT payload FROM records WHERE kind='task' AND id=$8
		)
		SELECT kind,payload
		FROM records
		WHERE ($3::text[] IS NULL OR kind=ANY($3))
		  AND ($4::text IS NULL OR payload->>'disposition'=$4)
		  AND ($5::text IS NULL OR payload->>'review_state'=$5)
		  AND ($6::text IS NULL OR lower(payload::text) LIKE $6 ESCAPE E'\\')
		  AND ($7::text[] IS NULL OR id=ANY($7))
		  AND ($8::text IS NULL OR
		       (kind='task' AND id=$8) OR
		       (kind='dependency' AND
		        (payload->>'task_id'=$8 OR payload->>'prerequisite_id'=$8)) OR
		       (kind='constraint' AND id IN (
		         SELECT jsonb_array_elements_text(payload->'constraint_ids') FROM selected_task
		       )) OR
		       (kind='group' AND id=(SELECT payload->>'group_id' FROM selected_task)) OR
		       (kind='requirement' AND id IN (
		         SELECT jsonb_array_elements_text(payload->'requirement_ids') FROM selected_task
		       )) OR
		       (kind='acceptance_criterion' AND (
		         id IN (SELECT jsonb_array_elements_text(payload->'acceptance_criterion_ids') FROM selected_task)
		         OR payload->'task_ids' ? $8
		       )) OR
		       (kind='check' AND EXISTS (
		         SELECT 1
		         FROM selected_task
		         WHERE (
		           (
		             jsonb_array_length(COALESCE(selected_task.payload->'requirement_ids','[]'::jsonb))=0
		             AND btrim(COALESCE(selected_task.payload->>'delivery_obligation',''))<>''
		           )
		           OR EXISTS (
		             SELECT 1
		             FROM jsonb_array_elements_text(
		               COALESCE(selected_task.payload->'requirement_ids','[]'::jsonb)
		             ) AS linked_requirement(id)
		             WHERE records.payload->'requirement_ids' ? linked_requirement.id
		           )
		         )
		         AND EXISTS (
		           SELECT 1
		           FROM jsonb_array_elements_text(
		             COALESCE(selected_task.payload->'acceptance_criterion_ids','[]'::jsonb)
		           ) AS linked_acceptance(id)
		           WHERE records.payload->'acceptance_criterion_ids' ? linked_acceptance.id
		         )
		       )))
		ORDER BY CASE WHEN $8::text IS NOT NULL AND kind='task' THEN 0 ELSE 1 END,rank,id
		LIMIT $9 OFFSET $10
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
		switch kind {
		case "objective":
			var value model.LedgerObjective
			if err := json.Unmarshal(raw, &value); err != nil {
				return CodingLedgerRevision{}, err
			}
			result.Ledger.Objectives = append(result.Ledger.Objectives, value)
		case "requirement":
			var value model.LedgerRequirement
			if err := json.Unmarshal(raw, &value); err != nil {
				return CodingLedgerRevision{}, err
			}
			result.Ledger.Requirements = append(result.Ledger.Requirements, value)
		case "constraint":
			var value model.LedgerConstraint
			if err := json.Unmarshal(raw, &value); err != nil {
				return CodingLedgerRevision{}, err
			}
			result.Ledger.Constraints = append(result.Ledger.Constraints, value)
		case "group":
			var value model.LedgerGroup
			if err := json.Unmarshal(raw, &value); err != nil {
				return CodingLedgerRevision{}, err
			}
			result.Ledger.Groups = append(result.Ledger.Groups, value)
		case "task":
			var value model.LedgerTask
			if err := json.Unmarshal(raw, &value); err != nil {
				return CodingLedgerRevision{}, err
			}
			result.Ledger.Tasks = append(result.Ledger.Tasks, value)
		case "acceptance_criterion":
			var value model.LedgerAcceptanceCriterion
			if err := json.Unmarshal(raw, &value); err != nil {
				return CodingLedgerRevision{}, err
			}
			result.Ledger.AcceptanceCriteria = append(result.Ledger.AcceptanceCriteria, value)
		case "check":
			var value model.LedgerCheck
			if err := json.Unmarshal(raw, &value); err != nil {
				return CodingLedgerRevision{}, err
			}
			result.Ledger.Checks = append(result.Ledger.Checks, value)
		case "dependency":
			var value model.LedgerDependency
			if err := json.Unmarshal(raw, &value); err != nil {
				return CodingLedgerRevision{}, err
			}
			result.Ledger.Dependencies = append(result.Ledger.Dependencies, value)
		default:
			return CodingLedgerRevision{}, fmt.Errorf("persisted coding ledger has unsupported item kind %q", kind)
		}
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
	switch {
	case item.Objective != nil:
		return item.Objective.ID
	case item.Requirement != nil:
		return item.Requirement.ID
	case item.Constraint != nil:
		return item.Constraint.ID
	case item.Group != nil:
		return item.Group.ID
	case item.Task != nil:
		return item.Task.ID
	case item.AcceptanceCriterion != nil:
		return item.AcceptanceCriterion.ID
	case item.Check != nil:
		return item.Check.ID
	case item.Dependency != nil:
		return item.Dependency.ID
	default:
		return ""
	}
}

func planningLedgerItemsForDelta(ledger model.EngineeringLedger) []model.LedgerItemID {
	result := make([]model.LedgerItemID, 0)
	for _, record := range ledgerRepositoryRecords(ledger) {
		result = append(result, record.item.ID)
	}
	for _, dependency := range ledger.Dependencies {
		result = append(result, dependency.ID)
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
	dependencyRows, err := r.pool.Query(ctx, `
		SELECT version.revision,version.payload
		FROM coding_ledger_items item
		JOIN coding_ledger_item_versions version USING(ledger_id,item_id)
		WHERE item.ledger_id=$1 AND item.item_id LIKE 'dependency.record.%'
		  AND version.payload->>'id'=$2
		ORDER BY version.revision DESC LIMIT $3 OFFSET $4
	`, ledgerID, itemID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer dependencyRows.Close()
	for dependencyRows.Next() {
		var revision int64
		var raw []byte
		if err := dependencyRows.Scan(&revision, &raw); err != nil {
			return nil, err
		}
		var dependency model.LedgerDependency
		if err := json.Unmarshal(raw, &dependency); err != nil {
			return nil, fmt.Errorf("decode coding ledger dependency %s history: %w", itemID, err)
		}
		result = append(result, CodingLedgerItemVersion{
			ItemID: dependency.ID, Kind: "dependency", Revision: revision,
			Disposition: dependency.Disposition, ReviewState: dependency.ReviewState,
			Rank: dependency.Rank, Payload: raw, Rationale: dependency.Rationale,
		})
	}
	return result, dependencyRows.Err()
}

func (r *Repository) CodingLedgerDiff(
	ctx context.Context,
	ledgerID, fromRevision, toRevision int64,
	pages ...CodingLedgerPage,
) (CodingLedgerDiff, error) {
	if r == nil || r.pool == nil || ledgerID <= 0 || fromRevision <= 0 || toRevision <= 0 {
		return CodingLedgerDiff{}, fmt.Errorf("coding ledger diff requires positive ledger and revision identities")
	}
	if len(pages) > 1 {
		return CodingLedgerDiff{}, fmt.Errorf("coding ledger diff accepts at most one page")
	}
	page := CodingLedgerPage{Limit: 200}
	if len(pages) == 1 {
		page = pages[0]
	}
	if page.Limit == 0 {
		page.Limit = 200
	}
	if page.Limit < 0 || page.Offset < 0 {
		return CodingLedgerDiff{}, fmt.Errorf("coding ledger diff page cannot be negative")
	}
	if page.Limit > 1000 {
		page.Limit = 1000
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return CodingLedgerDiff{}, err
	}
	defer tx.Rollback(ctx)
	var revisionCount int
	if err := tx.QueryRow(ctx, `
		SELECT count(*) FROM coding_ledger_revisions
		WHERE ledger_id=$1 AND revision=ANY($2::bigint[])
	`, ledgerID, []int64{fromRevision, toRevision}).Scan(&revisionCount); err != nil {
		return CodingLedgerDiff{}, err
	}
	expectedRevisions := 2
	if fromRevision == toRevision {
		expectedRevisions = 1
	}
	if revisionCount != expectedRevisions {
		return CodingLedgerDiff{}, fmt.Errorf("%w: diff revision does not exist", ErrCodingLedgerRevision)
	}
	rows, err := tx.Query(ctx, `
		WITH requested(revision) AS (VALUES ($2::bigint),($3::bigint)),
		item_records AS (
			SELECT requested.revision,projection.item_id AS id,version.payload
			FROM requested
			JOIN coding_ledger_revision_items projection
			  ON projection.ledger_id=$1 AND projection.revision=requested.revision
			JOIN coding_ledger_items item
			  ON item.ledger_id=projection.ledger_id AND item.item_id=projection.item_id
			JOIN coding_ledger_item_versions version
			  ON (version.ledger_id,version.item_id,version.revision)=
			     (projection.ledger_id,projection.item_id,projection.item_revision)
			WHERE item.item_id NOT LIKE 'dependency.record.%'
		),
		dependency_records AS (
			SELECT requested.revision,version.payload->>'id' AS id,version.payload
			FROM requested
			JOIN coding_ledger_revision_items projection
			  ON projection.ledger_id=$1 AND projection.revision=requested.revision
			JOIN coding_ledger_items item
			  ON item.ledger_id=projection.ledger_id AND item.item_id=projection.item_id
			JOIN coding_ledger_item_versions version
			  ON (version.ledger_id,version.item_id,version.revision)=
			     (projection.ledger_id,projection.item_id,projection.item_revision)
			WHERE item.item_id LIKE 'dependency.record.%'
		),
		records AS (
			SELECT * FROM item_records
			UNION ALL
			SELECT * FROM dependency_records
		),
		before_records AS (
			SELECT id,payload FROM records WHERE revision=$2
		),
		after_records AS (
			SELECT id,payload FROM records WHERE revision=$3
		)
		SELECT COALESCE(before_records.id,after_records.id),
		       before_records.id IS NOT NULL,after_records.id IS NOT NULL,
		       COALESCE(before_records.payload->>'disposition',''),
		       COALESCE(after_records.payload->>'disposition','')
		FROM before_records
		FULL OUTER JOIN after_records USING(id)
		WHERE before_records.payload IS DISTINCT FROM after_records.payload
		ORDER BY COALESCE(before_records.id,after_records.id)
		LIMIT $4 OFFSET $5
	`, ledgerID, fromRevision, toRevision, page.Limit+1, page.Offset)
	if err != nil {
		return CodingLedgerDiff{}, err
	}
	defer rows.Close()
	type changedRecord struct {
		id              model.LedgerItemID
		existed, exists bool
		oldDisposition  model.LedgerDisposition
		newDisposition  model.LedgerDisposition
	}
	changed := make([]changedRecord, 0, page.Limit+1)
	for rows.Next() {
		var record changedRecord
		if err := rows.Scan(&record.id, &record.existed, &record.exists,
			&record.oldDisposition, &record.newDisposition); err != nil {
			return CodingLedgerDiff{}, err
		}
		changed = append(changed, record)
	}
	if err := rows.Err(); err != nil {
		return CodingLedgerDiff{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return CodingLedgerDiff{}, err
	}
	result := CodingLedgerDiff{
		LedgerID: ledgerID, FromRevision: fromRevision, ToRevision: toRevision,
		Limit: page.Limit, Offset: page.Offset,
	}
	if len(changed) > page.Limit {
		result.HasMore = true
		changed = changed[:page.Limit]
	}
	for _, record := range changed {
		switch {
		case !record.existed:
			result.Added = append(result.Added, record.id)
		case !record.exists:
			result.Removed = append(result.Removed, record.id)
		case record.oldDisposition == model.LedgerDispositionActive &&
			record.newDisposition == model.LedgerDispositionTombstoned:
			result.Removed = append(result.Removed, record.id)
		case record.oldDisposition == model.LedgerDispositionTombstoned &&
			record.newDisposition == model.LedgerDispositionActive:
			result.Restored = append(result.Restored, record.id)
		default:
			result.Amended = append(result.Amended, record.id)
		}
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
	if len(task.Ledger.Tasks) != 1 || task.Ledger.Tasks[0].ID != taskID {
		return CodingLedgerRevision{}, pgx.ErrNoRows
	}
	related, err := r.ReadCodingLedger(ctx, ledgerID, CodingLedgerReadOptions{
		Revision: task.Revision, Limit: page.Limit, Offset: page.Offset,
		Kinds: []string{
			"requirement", "constraint", "group", "acceptance_criterion", "check",
			"dependency",
		},
		Removed: &active, RelatedTaskID: taskID,
	})
	if err != nil {
		return CodingLedgerRevision{}, err
	}
	task.Ledger.Requirements = related.Ledger.Requirements
	task.Ledger.Constraints = related.Ledger.Constraints
	task.Ledger.Groups = related.Ledger.Groups
	task.Ledger.AcceptanceCriteria = related.Ledger.AcceptanceCriteria
	task.Ledger.Checks = related.Ledger.Checks
	task.Ledger.Dependencies = related.Ledger.Dependencies
	return task, nil
}

func (r *Repository) CodingLedgerGraph(
	ctx context.Context,
	ledgerID, revision int64,
	includeRemoved bool,
	pages ...CodingLedgerPage,
) ([]model.LedgerDependency, error) {
	page := CodingLedgerPage{Limit: 200}
	if len(pages) > 1 {
		return nil, fmt.Errorf("coding ledger graph accepts at most one page")
	}
	if len(pages) == 1 {
		page = pages[0]
	}
	if page.Limit == 0 {
		page.Limit = 200
	}
	if page.Limit < 0 {
		return nil, fmt.Errorf("coding ledger graph limit cannot be negative")
	}
	if page.Limit > 1000 {
		page.Limit = 1000
	}
	if page.Offset < 0 {
		return nil, fmt.Errorf("coding ledger graph offset cannot be negative")
	}
	removed := false
	options := CodingLedgerReadOptions{
		Revision: revision, Limit: page.Limit, Offset: page.Offset, Kinds: []string{"dependency"},
	}
	if !includeRemoved {
		options.Removed = &removed
	}
	projection, err := r.ReadCodingLedger(ctx, ledgerID, options)
	if err != nil {
		return nil, err
	}
	return projection.Ledger.Dependencies, nil
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
