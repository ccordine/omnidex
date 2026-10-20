package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

const codingLedgerInternalOwnerTurnPrefix = "omnidex-internal:"

func validateCodingLedgerExternalOwnerTurnID(turnID string) error {
	if strings.HasPrefix(turnID, codingLedgerInternalOwnerTurnPrefix) {
		return fmt.Errorf("coding ledger owner turn identity uses reserved internal namespace")
	}
	return nil
}

func codingLedgerSignOffTurnID(ownerTurnID string) string {
	return codingLedgerInternalDerivedID("sign-off", ownerTurnID)
}

func codingLedgerInternalDerivedID(kind, authority string) string {
	digest := fnv.New128a()
	_, _ = digest.Write([]byte(authority))
	return fmt.Sprintf("%s%s:%x", codingLedgerInternalOwnerTurnPrefix, kind, digest.Sum(nil))
}

type ledgerRepositoryRecord struct {
	kind    string
	item    model.LedgerItem
	payload any
}

func ledgerRepositoryRecords(ledger model.EngineeringLedger) []ledgerRepositoryRecord {
	records := make([]ledgerRepositoryRecord, 0, len(ledger.Tasks))
	for i := range ledger.Tasks {
		task := &ledger.Tasks[i]
		// Persist the complete task payload, including source-bound context and
		// criterion-bound verification; file/goal projections are never storage.
		records = append(records, ledgerRepositoryRecord{"task", task.LedgerItem, task})
	}
	return records
}

func sortCodingLedgerProjection(ledger *model.EngineeringLedger) {
	sort.SliceStable(ledger.Tasks, func(i, j int) bool {
		a, b := ledger.Tasks[i], ledger.Tasks[j]
		return a.Rank < b.Rank || a.Rank == b.Rank && a.ID < b.ID
	})
}

func (r *Repository) CreateCodingLedger(ctx context.Context, command CreateCodingLedgerCommand) (CodingLedgerRevision, error) {
	if r == nil || r.pool == nil {
		return CodingLedgerRevision{}, fmt.Errorf("coding ledger repository is unavailable")
	}
	if command.JobID <= 0 || strings.TrimSpace(command.OwnerTurnID) == "" ||
		strings.TrimSpace(command.OwnerTurnText) == "" {
		return CodingLedgerRevision{}, fmt.Errorf("initial coding ledger requires job, operation, and owner-turn authority")
	}
	if err := validateCodingLedgerOperationID(command.OperationID); err != nil {
		return CodingLedgerRevision{}, err
	}
	if err := validateCodingLedgerExternalOwnerTurnID(command.OwnerTurnID); err != nil {
		return CodingLedgerRevision{}, err
	}
	command.Ledger.Revision = 1
	if err := validateDraftLedgerForRepository(command.Ledger); err != nil {
		return CodingLedgerRevision{}, err
	}
	if err := validateInitialCodingLedgerReviewStates(command.Ledger); err != nil {
		return CodingLedgerRevision{}, err
	}
	if err := validateOwnerTurnDisposition(command.Ledger, command.OwnerTurnDisposition); err != nil {
		return CodingLedgerRevision{}, err
	}
	if err := validateCodingLedgerRoutes(command.Routes, 1); err != nil {
		return CodingLedgerRevision{}, err
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return CodingLedgerRevision{}, err
	}
	defer tx.Rollback(ctx)

	// Serialize proposal retries by durable job identity so concurrent deliveries
	// observe and replay the committed receipt instead of racing the unique job.
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('coding-ledger-job:' || $1::bigint::text,0))`, command.JobID); err != nil {
		return CodingLedgerRevision{}, fmt.Errorf("lock coding ledger job identity: %w", err)
	}

	var existingLedgerID int64
	var replayOwner, replayText string
	err = tx.QueryRow(ctx, `
		SELECT ledger.id,revision.owner_turn_id,turn_record.exact_text
		FROM coding_ledger_revisions revision
		JOIN coding_ledgers ledger ON ledger.id=revision.ledger_id
		JOIN coding_ledger_owner_turns turn_record
		  ON turn_record.ledger_id=revision.ledger_id AND turn_record.owner_turn_id=revision.owner_turn_id
		WHERE ledger.job_id=$1 AND revision.operation_id=$2
	`, command.JobID, command.OperationID).Scan(&existingLedgerID, &replayOwner, &replayText)
	if err == nil {
		if replayOwner != command.OwnerTurnID || replayText != command.OwnerTurnText {
			return CodingLedgerRevision{}, fmt.Errorf("coding ledger operation %q was already used by a different proposal", command.OperationID)
		}
		result, readErr := readCodingLedgerRevisionTx(ctx, tx, existingLedgerID, 1, CodingLedgerReadOptions{Limit: -1})
		if readErr != nil {
			return CodingLedgerRevision{}, readErr
		}
		expectedLedger := command.Ledger
		expectedLedger.ID = result.Ledger.ID
		resultLedger := result.Ledger
		sortCodingLedgerProjection(&expectedLedger)
		sortCodingLedgerProjection(&resultLedger)
		normalizeCodingLedgerProjectionSlices(&expectedLedger)
		normalizeCodingLedgerProjectionSlices(&resultLedger)
		expectedRaw, encodeErr := json.Marshal(expectedLedger)
		if encodeErr != nil {
			return CodingLedgerRevision{}, encodeErr
		}
		resultRaw, encodeErr := json.Marshal(resultLedger)
		if encodeErr != nil {
			return CodingLedgerRevision{}, encodeErr
		}
		dispositionMatches, matchErr := codingLedgerDispositionMatchesTx(
			ctx, tx, existingLedgerID, 1, command.OwnerTurnID, command.OwnerTurnDisposition, command.Ledger,
		)
		if matchErr != nil {
			return CodingLedgerRevision{}, matchErr
		}
		if string(expectedRaw) != string(resultRaw) || !codingLedgerRoutesEqual(command.Routes, result.Routes) ||
			!dispositionMatches {
			return CodingLedgerRevision{}, fmt.Errorf("coding ledger operation %q was already used by a different proposal", command.OperationID)
		}
		if err := tx.Commit(ctx); err != nil {
			return CodingLedgerRevision{}, err
		}
		result.Applied = false
		return result, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return CodingLedgerRevision{}, err
	}

	var ledgerID int64
	if err := tx.QueryRow(ctx, `INSERT INTO coding_ledgers(job_id) VALUES($1) RETURNING id`, command.JobID).Scan(&ledgerID); err != nil {
		return CodingLedgerRevision{}, fmt.Errorf("create coding ledger: %w", err)
	}
	if err := insertCodingLedgerOwnerTurnTx(ctx, tx, ledgerID, command.OwnerTurnID, 0, "planning", command.OwnerTurnText); err != nil {
		return CodingLedgerRevision{}, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO coding_ledger_revisions(ledger_id,revision,base_revision,operation_id,owner_turn_id,state)
		VALUES($1,1,0,$2,$3,'proposed')
	`, ledgerID, command.OperationID, command.OwnerTurnID); err != nil {
		return CodingLedgerRevision{}, fmt.Errorf("create initial coding ledger revision: %w", err)
	}
	if err := persistCodingLedgerSnapshotTx(ctx, tx, ledgerID, 1, 0, command.OwnerTurnID, command.Ledger, command.Routes); err != nil {
		return CodingLedgerRevision{}, err
	}
	if err := insertCodingLedgerDispositionTx(ctx, tx, ledgerID, 1, command.OwnerTurnID, command.OwnerTurnDisposition, command.Ledger); err != nil {
		return CodingLedgerRevision{}, err
	}
	result, err := readCodingLedgerRevisionTx(ctx, tx, ledgerID, 1, CodingLedgerReadOptions{Limit: -1})
	if err != nil {
		return CodingLedgerRevision{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return CodingLedgerRevision{}, err
	}
	result.Applied = true
	return result, nil
}

func normalizeCodingLedgerProjectionSlices(ledger *model.EngineeringLedger) {
	if ledger.Tasks == nil {
		ledger.Tasks = []model.LedgerTask{}
	}
}

func codingLedgerRoutesEqual(left, right []CodingLedgerRoute) bool {
	if len(left) != len(right) {
		return false
	}
	byRole := func(routes []CodingLedgerRoute) map[string]CodingLedgerRoute {
		result := make(map[string]CodingLedgerRoute, len(routes))
		for _, route := range routes {
			result[route.Role] = route
		}
		return result
	}
	return reflect.DeepEqual(byRole(left), byRole(right))
}

func insertCodingLedgerOwnerTurnTx(ctx context.Context, tx pgx.Tx, ledgerID int64, turnID string, baseRevision int64, action, exactText string) error {
	if strings.TrimSpace(turnID) == "" || strings.TrimSpace(exactText) == "" {
		return fmt.Errorf("coding ledger owner turn requires identity and exact text")
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO coding_ledger_owner_turns(ledger_id,owner_turn_id,base_revision,action,exact_text)
		VALUES($1,$2,$3,$4,$5)
	`, ledgerID, turnID, baseRevision, action, exactText)
	if err != nil {
		return fmt.Errorf("record coding ledger owner turn: %w", err)
	}
	return nil
}

func insertCodingLedgerDispositionTx(
	ctx context.Context,
	tx pgx.Tx,
	ledgerID, revision int64,
	turnID string,
	disposition model.OwnerTurnDisposition,
	ledger model.EngineeringLedger,
	authorityLedgers ...model.EngineeringLedger,
) error {
	statements := disposition.StatementDispositions()
	if statements == nil {
		statements = []model.OwnerStatementDisposition{}
	}
	rawStatements, err := json.Marshal(statements)
	if err != nil {
		return fmt.Errorf("encode coding ledger owner-turn dispositions: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO coding_ledger_owner_turn_dispositions(ledger_id,owner_turn_id,revision,statements)
		VALUES($1,$2,$3,$4::jsonb)
	`, ledgerID, turnID, revision, string(rawStatements)); err != nil {
		return fmt.Errorf("record coding ledger owner-turn disposition: %w", err)
	}
	ids := make([]model.LedgerItemID, 0)
	for _, statement := range statements {
		ids = append(ids, statement.ItemIDs...)
	}
	targets := make(map[model.LedgerItemID]bool)
	for _, id := range ids {
		targets[id] = true
	}
	for id := range targets {
		result, err := tx.Exec(ctx, `
			INSERT INTO coding_ledger_owner_turn_items(ledger_id,owner_turn_id,item_id)
			SELECT $1,$2,$3
			WHERE EXISTS (
				SELECT 1 FROM coding_ledger_items WHERE ledger_id=$1 AND item_id=$3
			)
			ON CONFLICT DO NOTHING
		`, ledgerID, turnID, id)
		if err != nil {
			return fmt.Errorf("record coding ledger owner-turn item %s: %w", id, err)
		}
		if result.RowsAffected() != 1 {
			return fmt.Errorf("record coding ledger owner-turn item %s: target is unavailable", id)
		}
	}
	return nil
}

func persistCodingLedgerSnapshotTx(ctx context.Context, tx pgx.Tx, ledgerID, revision, baseRevision int64, ownerTurnID string, ledger model.EngineeringLedger, routes []CodingLedgerRoute) error {
	previous := map[model.LedgerItemID]CodingLedgerItemVersion{}
	if baseRevision > 0 {
		rows, err := tx.Query(ctx, `
			SELECT projection.item_id,item.item_kind,version.revision,
			       version.disposition,COALESCE(version.review_state,''),version.rank,
			       version.payload,version.rationale
			FROM coding_ledger_revision_items projection
			JOIN coding_ledger_items item USING(ledger_id,item_id)
			JOIN coding_ledger_item_versions version
			  ON (version.ledger_id,version.item_id,version.revision)=
			     (projection.ledger_id,projection.item_id,projection.item_revision)
			WHERE projection.ledger_id=$1 AND projection.revision=$2
		`, ledgerID, baseRevision)
		if err != nil {
			return err
		}
		for rows.Next() {
			var value CodingLedgerItemVersion
			if err := rows.Scan(&value.ItemID, &value.Kind, &value.Revision, &value.Disposition,
				&value.ReviewState, &value.Rank, &value.Payload, &value.Rationale); err != nil {
				rows.Close()
				return err
			}
			previous[value.ItemID] = value
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return err
		}
		rows.Close()
	}
	for _, record := range ledgerRepositoryRecords(ledger) {
		raw, err := json.Marshal(record.payload)
		if err != nil {
			return fmt.Errorf("encode coding ledger item %s: %w", record.item.ID, err)
		}
		disposition := "active"
		if record.item.Disposition == model.LedgerDispositionTombstoned {
			disposition = "removed"
		}
		review := any(record.item.ReviewState)
		if disposition == "removed" {
			review = nil
		}
		prior, known := previous[record.item.ID]
		expectedDisposition := record.item.Disposition
		if prior.Disposition == model.LedgerDisposition("removed") {
			prior.Disposition = model.LedgerDispositionTombstoned
		}
		changed := !known || prior.Kind != record.kind || !sameCodingLedgerJSON(prior.Payload, raw) ||
			prior.Disposition != expectedDisposition || prior.ReviewState != record.item.ReviewState ||
			prior.Rank != record.item.Rank || prior.Rationale != record.item.Rationale
		itemRevision := prior.Revision
		if !known {
			if _, err := tx.Exec(ctx, `
				INSERT INTO coding_ledger_items(ledger_id,item_id,item_kind,created_revision)
				VALUES($1,$2,$3,$4)
			`, ledgerID, record.item.ID, record.kind, revision); err != nil {
				return fmt.Errorf("create coding ledger item %s: %w", record.item.ID, err)
			}
		}
		if changed {
			itemRevision = revision
			var removedBy any
			if disposition == "removed" {
				removedBy = ownerTurnID
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO coding_ledger_item_versions(
					ledger_id,item_id,revision,disposition,review_state,rank,payload,rationale,removed_by_turn_id
				) VALUES($1,$2,$3,$4,$5,$6,$7::jsonb,$8,$9)
			`, ledgerID, record.item.ID, revision, disposition, review, record.item.Rank, string(raw),
				record.item.Rationale, removedBy); err != nil {
				return fmt.Errorf("version coding ledger item %s: %w", record.item.ID, err)
			}
			detail := fmt.Sprintf("%s:%s", record.item.Provenance.Kind, record.item.Provenance.Reference)
			if _, err := tx.Exec(ctx, `
				INSERT INTO coding_ledger_origins(ledger_id,item_id,revision,origin_kind,detail)
				VALUES($1,$2,$3,'planner',$4)
			`, ledgerID, record.item.ID, revision, detail); err != nil {
				return fmt.Errorf("record coding ledger item %s origin: %w", record.item.ID, err)
			}
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO coding_ledger_revision_items(ledger_id,revision,item_id,item_revision)
			VALUES($1,$2,$3,$4)
		`, ledgerID, revision, record.item.ID, itemRevision); err != nil {
			return fmt.Errorf("project coding ledger item %s: %w", record.item.ID, err)
		}
	}
	for _, route := range routes {
		if _, err := tx.Exec(ctx, `
			INSERT INTO coding_ledger_routes(
				ledger_id,revision,role,configuration_revision,provider,model,reasoning_effort,service_tier
			) VALUES($1,$2,$3,$4,$5,$6,$7,$8)
		`, ledgerID, revision, route.Role, route.ConfigurationRevision, route.Provider, route.Model,
			route.ReasoningEffort, route.ServiceTier); err != nil {
			return fmt.Errorf("record coding ledger %s route: %w", route.Role, err)
		}
	}
	return nil
}

func codingLedgerExternalID(ledgerID int64) string { return strconv.FormatInt(ledgerID, 10) }

func sameCodingLedgerJSON(left, right []byte) bool {
	decode := func(raw []byte) (any, error) {
		decoder := json.NewDecoder(strings.NewReader(string(raw)))
		decoder.UseNumber()
		var value any
		if err := decoder.Decode(&value); err != nil {
			return nil, err
		}
		return value, nil
	}
	leftValue, leftErr := decode(left)
	rightValue, rightErr := decode(right)
	return leftErr == nil && rightErr == nil && reflect.DeepEqual(leftValue, rightValue)
}
