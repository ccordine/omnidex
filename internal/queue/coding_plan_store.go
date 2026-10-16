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

func codingLedgerAnswerTurnID(ownerTurnID string, questionID model.LedgerItemID) string {
	return codingLedgerInternalDerivedID("answer", ownerTurnID+"\x00"+string(questionID))
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
	records := make([]ledgerRepositoryRecord, 0,
		len(ledger.Objectives)+len(ledger.Requirements)+len(ledger.Constraints)+len(ledger.Groups)+
			len(ledger.Tasks)+len(ledger.AcceptanceCriteria)+len(ledger.Checks)+
			len(ledger.DecisionsAndQuestions)+len(ledger.Findings))
	for index := range ledger.Objectives {
		value := &ledger.Objectives[index]
		records = append(records, ledgerRepositoryRecord{"objective", value.LedgerItem, value})
	}
	for index := range ledger.Requirements {
		value := &ledger.Requirements[index]
		records = append(records, ledgerRepositoryRecord{"requirement", value.LedgerItem, value})
	}
	for index := range ledger.Constraints {
		value := &ledger.Constraints[index]
		records = append(records, ledgerRepositoryRecord{"constraint", value.LedgerItem, value})
	}
	for index := range ledger.Groups {
		value := &ledger.Groups[index]
		records = append(records, ledgerRepositoryRecord{"group", value.LedgerItem, value})
	}
	for index := range ledger.Tasks {
		value := &ledger.Tasks[index]
		records = append(records, ledgerRepositoryRecord{"task", value.LedgerItem, value})
	}
	for index := range ledger.AcceptanceCriteria {
		value := &ledger.AcceptanceCriteria[index]
		records = append(records, ledgerRepositoryRecord{"acceptance_criterion", value.LedgerItem, value})
	}
	for index := range ledger.Checks {
		value := &ledger.Checks[index]
		records = append(records, ledgerRepositoryRecord{"check", value.LedgerItem, value})
	}
	for index := range ledger.DecisionsAndQuestions {
		value := &ledger.DecisionsAndQuestions[index]
		kind := "decision"
		if value.Kind == model.LedgerQuestion {
			kind = "question"
		}
		records = append(records, ledgerRepositoryRecord{kind, value.LedgerItem, value})
	}
	for index := range ledger.Findings {
		value := &ledger.Findings[index]
		records = append(records, ledgerRepositoryRecord{"finding", value.LedgerItem, value})
	}
	return records
}

func sortCodingLedgerProjection(ledger *model.EngineeringLedger) {
	less := func(leftRank int64, leftID model.LedgerItemID, rightRank int64, rightID model.LedgerItemID) bool {
		return leftRank < rightRank || (leftRank == rightRank && leftID < rightID)
	}
	sort.SliceStable(ledger.Objectives, func(i, j int) bool {
		return less(ledger.Objectives[i].Rank, ledger.Objectives[i].ID, ledger.Objectives[j].Rank, ledger.Objectives[j].ID)
	})
	sort.SliceStable(ledger.Requirements, func(i, j int) bool {
		return less(ledger.Requirements[i].Rank, ledger.Requirements[i].ID, ledger.Requirements[j].Rank, ledger.Requirements[j].ID)
	})
	sort.SliceStable(ledger.Constraints, func(i, j int) bool {
		return less(ledger.Constraints[i].Rank, ledger.Constraints[i].ID, ledger.Constraints[j].Rank, ledger.Constraints[j].ID)
	})
	sort.SliceStable(ledger.Groups, func(i, j int) bool {
		return less(ledger.Groups[i].Rank, ledger.Groups[i].ID, ledger.Groups[j].Rank, ledger.Groups[j].ID)
	})
	sort.SliceStable(ledger.Tasks, func(i, j int) bool {
		return less(ledger.Tasks[i].Rank, ledger.Tasks[i].ID, ledger.Tasks[j].Rank, ledger.Tasks[j].ID)
	})
	sort.SliceStable(ledger.AcceptanceCriteria, func(i, j int) bool {
		return less(ledger.AcceptanceCriteria[i].Rank, ledger.AcceptanceCriteria[i].ID, ledger.AcceptanceCriteria[j].Rank, ledger.AcceptanceCriteria[j].ID)
	})
	sort.SliceStable(ledger.Checks, func(i, j int) bool {
		return less(ledger.Checks[i].Rank, ledger.Checks[i].ID, ledger.Checks[j].Rank, ledger.Checks[j].ID)
	})
	sort.SliceStable(ledger.DecisionsAndQuestions, func(i, j int) bool {
		return less(ledger.DecisionsAndQuestions[i].Rank, ledger.DecisionsAndQuestions[i].ID, ledger.DecisionsAndQuestions[j].Rank, ledger.DecisionsAndQuestions[j].ID)
	})
	sort.SliceStable(ledger.Dependencies, func(i, j int) bool {
		return less(ledger.Dependencies[i].Rank, ledger.Dependencies[i].ID, ledger.Dependencies[j].Rank, ledger.Dependencies[j].ID)
	})
	sort.SliceStable(ledger.Findings, func(i, j int) bool {
		return less(ledger.Findings[i].Rank, ledger.Findings[i].ID, ledger.Findings[j].Rank, ledger.Findings[j].ID)
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
	if err := validateLedgerForRepository(command.Ledger); err != nil {
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
	var replayCoverage bool
	err = tx.QueryRow(ctx, `
		SELECT ledger.id,revision.owner_turn_id,turn_record.exact_text,revision.coverage_passed
		FROM coding_ledger_revisions revision
		JOIN coding_ledgers ledger ON ledger.id=revision.ledger_id
		JOIN coding_ledger_owner_turns turn_record
		  ON turn_record.ledger_id=revision.ledger_id AND turn_record.owner_turn_id=revision.owner_turn_id
		WHERE ledger.job_id=$1 AND revision.operation_id=$2
	`, command.JobID, command.OperationID).Scan(&existingLedgerID, &replayOwner, &replayText, &replayCoverage)
	if err == nil {
		if replayOwner != command.OwnerTurnID || replayText != command.OwnerTurnText ||
			replayCoverage != command.CoveragePassed {
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
		INSERT INTO coding_ledger_revisions(ledger_id,revision,base_revision,operation_id,owner_turn_id,state,coverage_passed)
		VALUES($1,1,0,$2,$3,'proposed',$4)
	`, ledgerID, command.OperationID, command.OwnerTurnID, command.CoveragePassed); err != nil {
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
	if ledger.Objectives == nil {
		ledger.Objectives = []model.LedgerObjective{}
	}
	if ledger.Requirements == nil {
		ledger.Requirements = []model.LedgerRequirement{}
	}
	if ledger.Constraints == nil {
		ledger.Constraints = []model.LedgerConstraint{}
	}
	if ledger.Groups == nil {
		ledger.Groups = []model.LedgerGroup{}
	}
	if ledger.Tasks == nil {
		ledger.Tasks = []model.LedgerTask{}
	}
	if ledger.AcceptanceCriteria == nil {
		ledger.AcceptanceCriteria = []model.LedgerAcceptanceCriterion{}
	}
	if ledger.Checks == nil {
		ledger.Checks = []model.LedgerCheck{}
	}
	if ledger.DecisionsAndQuestions == nil {
		ledger.DecisionsAndQuestions = []model.LedgerDecisionQuestion{}
	}
	if ledger.Dependencies == nil {
		ledger.Dependencies = []model.LedgerDependency{}
	}
	if ledger.Findings == nil {
		ledger.Findings = []model.LedgerFinding{}
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
	if len(statements) == 0 {
		return fmt.Errorf("record coding ledger owner-turn disposition: no statements")
	}
	persistedKind := statements[0].Kind
	if len(statements) > 1 {
		persistedKind = model.OwnerTurnDispositionKind("mixed")
	}
	var reason *string
	if len(statements) == 1 && statements[0].Kind == model.OwnerTurnOutOfScope {
		value := statements[0].Reason
		reason = &value
	}
	rawStatements, err := json.Marshal(statements)
	if err != nil {
		return fmt.Errorf("encode coding ledger owner-turn dispositions: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO coding_ledger_owner_turn_dispositions(ledger_id,owner_turn_id,revision,disposition,reason,statements)
		VALUES($1,$2,$3,$4,$5,$6::jsonb)
	`, ledgerID, turnID, revision, persistedKind, reason, string(rawStatements)); err != nil {
		return fmt.Errorf("record coding ledger owner-turn disposition: %w", err)
	}
	ids := make([]model.LedgerItemID, 0)
	for _, statement := range statements {
		ids = append(ids, statement.ItemIDs...)
		ids = append(ids, statement.QuestionIDs...)
	}
	dependencies := make(map[model.LedgerItemID][]model.LedgerDependency)
	ledgers := append([]model.EngineeringLedger{ledger}, authorityLedgers...)
	for _, authorityLedger := range ledgers {
		for _, dependency := range authorityLedger.Dependencies {
			dependencies[dependency.ID] = append(dependencies[dependency.ID], dependency)
		}
	}
	targets := make(map[model.LedgerItemID]bool)
	for _, id := range ids {
		if dependencyVersions := dependencies[id]; len(dependencyVersions) > 0 {
			// Dependencies are durable edges, not item rows. Attach represented
			// owner authority to both endpoints from every supplied before/after
			// version so an amendment cannot discard authority for a removed endpoint.
			for _, dependency := range dependencyVersions {
				targets[dependency.TaskID] = true
				targets[dependency.PrerequisiteID] = true
			}
		} else {
			targets[id] = true
		}
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
		// Active check definitions are revision-bound by the durable schema.
		// Tombstoned checks retain their exact removal version and attribution.
		if record.kind == "check" && disposition == "active" {
			changed = true
		}
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
	if err := persistCodingLedgerDependenciesTx(ctx, tx, ledgerID, revision, baseRevision, ownerTurnID, ledger.Dependencies); err != nil {
		return err
	}
	if err := persistCodingLedgerEdgesTx(ctx, tx, ledgerID, revision, ledger); err != nil {
		return err
	}
	for _, check := range ledger.Checks {
		if check.Disposition != model.LedgerDispositionActive {
			continue
		}
		var argv any
		var observation any
		if len(check.Argv) > 0 {
			raw, err := json.Marshal(check.Argv)
			if err != nil {
				return err
			}
			argv = string(raw)
		} else {
			observation = check.Observation
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO coding_ledger_checks(ledger_id,revision,check_item_id,required,argv,observation)
			VALUES($1,$2,$3,$4,$5::jsonb,$6)
		`, ledgerID, revision, check.ID, check.Required, argv, observation); err != nil {
			return fmt.Errorf("record coding ledger check %s: %w", check.ID, err)
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
	return syncCodingLedgerQuestionsAndFindingsTx(ctx, tx, ledgerID, revision, ownerTurnID, ledger)
}

const (
	codingLedgerDependencyRecordPrefix = "dependency.record."
	codingLedgerDerivedEdgePrefix      = "derived.edge."
	codingLedgerDependencyEdgePrefix   = "dependency.edge."
	codingLedgerOriginDetailMaxBytes   = 8192
)

func codingLedgerDependencyEdgeID(id model.LedgerItemID) string {
	return codingLedgerDependencyEdgePrefix + string(id)
}

func codingLedgerDependencyRecordID(id model.LedgerItemID) model.LedgerItemID {
	digest := fnv.New128a()
	_, _ = digest.Write([]byte(id))
	return model.LedgerItemID(codingLedgerDependencyRecordPrefix + fmt.Sprintf("%x", digest.Sum(nil)))
}

func persistCodingLedgerDependenciesTx(
	ctx context.Context,
	tx pgx.Tx,
	ledgerID, revision, baseRevision int64,
	ownerTurnID string,
	dependencies []model.LedgerDependency,
) error {
	type priorDependency struct {
		itemRevision int64
		payload      []byte
	}
	previous := make(map[model.LedgerItemID]priorDependency)
	if baseRevision > 0 {
		rows, err := tx.Query(ctx, `
			SELECT version.payload->>'id',projection.item_revision,version.payload
			FROM coding_ledger_revision_items projection
			JOIN coding_ledger_items item USING(ledger_id,item_id)
			JOIN coding_ledger_item_versions version
			  ON (version.ledger_id,version.item_id,version.revision)=
			     (projection.ledger_id,projection.item_id,projection.item_revision)
			WHERE projection.ledger_id=$1 AND projection.revision=$2
			  AND item.item_id LIKE $3
		`, ledgerID, baseRevision, codingLedgerDependencyRecordPrefix+"%")
		if err != nil {
			return err
		}
		for rows.Next() {
			var id model.LedgerItemID
			var value priorDependency
			if err := rows.Scan(&id, &value.itemRevision, &value.payload); err != nil {
				rows.Close()
				return err
			}
			previous[id] = value
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return err
		}
		rows.Close()
	}
	current := make(map[model.LedgerItemID]bool, len(dependencies))
	for _, dependency := range dependencies {
		current[dependency.ID] = true
		raw, err := json.Marshal(dependency)
		if err != nil {
			return fmt.Errorf("encode coding ledger dependency %s: %w", dependency.ID, err)
		}
		recordID := codingLedgerDependencyRecordID(dependency.ID)
		prior, known := previous[dependency.ID]
		if !known {
			if _, err := tx.Exec(ctx, `
				INSERT INTO coding_ledger_items(ledger_id,item_id,item_kind,created_revision)
				VALUES($1,$2,'decision',$3)
			`, ledgerID, recordID, revision); err != nil {
				return fmt.Errorf("create coding ledger dependency record %s: %w", dependency.ID, err)
			}
		}
		itemRevision := prior.itemRevision
		if !known || !sameCodingLedgerJSON(prior.payload, raw) {
			itemRevision = revision
			if _, err := tx.Exec(ctx, `
				INSERT INTO coding_ledger_item_versions(
					ledger_id,item_id,revision,disposition,review_state,rank,payload,rationale,removed_by_turn_id
				) VALUES($1,$2,$3,'removed',NULL,$4,$5::jsonb,$6,$7)
			`, ledgerID, recordID, revision, dependency.Rank, string(raw), dependency.Rationale, ownerTurnID); err != nil {
				return fmt.Errorf("version coding ledger dependency %s: %w", dependency.ID, err)
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO coding_ledger_origins(ledger_id,item_id,revision,origin_kind,detail)
				VALUES($1,$2,$3,'planner',$4)
			`, ledgerID, recordID, revision, "dependency:"+string(dependency.ID)); err != nil {
				return fmt.Errorf("record coding ledger dependency %s origin: %w", dependency.ID, err)
			}
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO coding_ledger_revision_items(ledger_id,revision,item_id,item_revision)
			VALUES($1,$2,$3,$4)
		`, ledgerID, revision, recordID, itemRevision); err != nil {
			return fmt.Errorf("project coding ledger dependency %s: %w", dependency.ID, err)
		}
	}
	for id := range previous {
		if !current[id] {
			return fmt.Errorf("coding ledger dependency %s was omitted instead of retained as a tombstone", id)
		}
	}
	return nil
}

func persistCodingLedgerEdgesTx(ctx context.Context, tx pgx.Tx, ledgerID, revision int64, ledger model.EngineeringLedger) error {
	type edge struct {
		id, kind  string
		from, to  model.LedgerItemID
		rationale string
	}
	edges := make([]edge, 0)
	appendEdge := func(id, kind string, from, to model.LedgerItemID, rationale string) {
		edges = append(edges, edge{id, kind, from, to, rationale})
	}
	var activeObjectiveID model.LedgerItemID
	for _, objective := range ledger.Objectives {
		if objective.Disposition == model.LedgerDispositionActive {
			activeObjectiveID = objective.ID
			break
		}
	}
	for _, task := range ledger.Tasks {
		if task.Disposition != model.LedgerDispositionActive {
			continue
		}
		if len(task.RequirementIDs) == 0 && strings.TrimSpace(task.DeliveryObligation) != "" {
			appendEdge(codingLedgerDerivedEdgePrefix+fmt.Sprintf("%s:delivery", task.ID), "covers", task.ID, activeObjectiveID, task.Rationale)
		}
		for index, id := range task.RequirementIDs {
			appendEdge(codingLedgerDerivedEdgePrefix+fmt.Sprintf("%s:req:%d", task.ID, index), "covers", task.ID, id, task.Rationale)
		}
		for index, id := range task.ConstraintIDs {
			appendEdge(codingLedgerDerivedEdgePrefix+fmt.Sprintf("%s:constraint:%d", task.ID, index), "affects", id, task.ID, task.Rationale)
		}
		for index, id := range task.AcceptanceCriterionIDs {
			appendEdge(codingLedgerDerivedEdgePrefix+fmt.Sprintf("%s:accept:%d", task.ID, index), "covers", task.ID, id, task.Rationale)
		}
		if task.GroupID != "" {
			appendEdge(codingLedgerDerivedEdgePrefix+fmt.Sprintf("%s:group", task.ID), "contains", task.GroupID, task.ID, task.Rationale)
		}
	}
	for _, group := range ledger.Groups {
		if group.Disposition == model.LedgerDispositionActive && group.ParentGroupID != "" {
			appendEdge(codingLedgerDerivedEdgePrefix+fmt.Sprintf("%s:parent", group.ID), "contains", group.ParentGroupID, group.ID, group.Rationale)
		}
	}
	for _, criterion := range ledger.AcceptanceCriteria {
		if criterion.Disposition != model.LedgerDispositionActive {
			continue
		}
		for index, id := range criterion.RequirementIDs {
			appendEdge(codingLedgerDerivedEdgePrefix+fmt.Sprintf("%s:req:%d", criterion.ID, index), "covers", criterion.ID, id, criterion.Rationale)
		}
	}
	for _, check := range ledger.Checks {
		if check.Disposition != model.LedgerDispositionActive {
			continue
		}
		for index, id := range check.AcceptanceCriterionIDs {
			appendEdge(codingLedgerDerivedEdgePrefix+fmt.Sprintf("%s:verify:%d", check.ID, index), "verifies", check.ID, id, check.Rationale)
		}
	}
	for _, value := range ledger.DecisionsAndQuestions {
		if value.Disposition != model.LedgerDispositionActive {
			continue
		}
		for index, id := range value.Affects {
			appendEdge(codingLedgerDerivedEdgePrefix+fmt.Sprintf("%s:affect:%d", value.ID, index), "affects", value.ID, id, value.Rationale)
		}
	}
	for _, value := range ledger.Findings {
		if value.Disposition != model.LedgerDispositionActive {
			continue
		}
		for index, id := range value.ResolvedBy {
			appendEdge(codingLedgerDerivedEdgePrefix+fmt.Sprintf("%s:resolve:%d", value.ID, index), "resolves", value.ID, id, value.Rationale)
		}
	}
	activeTasks := make(map[model.LedgerItemID]bool, len(ledger.Tasks))
	for _, task := range ledger.Tasks {
		activeTasks[task.ID] = task.Disposition == model.LedgerDispositionActive
	}
	for _, value := range ledger.Dependencies {
		// The durable edge trigger intentionally accepts only active endpoints.
		// When endpoint removal also tombstones its dependency, the prior edge
		// remains immutable history and the current tombstone is reconstructed
		// from the canonical ledger mutation rather than inserted as a live edge.
		if value.Disposition == model.LedgerDispositionTombstoned {
			continue
		}
		appendEdge(codingLedgerDependencyEdgeID(value.ID), string(value.Kind), value.TaskID, value.PrerequisiteID, value.Rationale)
	}
	for _, value := range edges {
		if _, err := tx.Exec(ctx, `
			INSERT INTO coding_ledger_edges(ledger_id,revision,edge_id,edge_kind,from_item_id,to_item_id,rationale)
			VALUES($1,$2,$3,$4,$5,$6,$7)
		`, ledgerID, revision, value.id, value.kind, value.from, value.to, value.rationale); err != nil {
			return fmt.Errorf("record coding ledger edge %s: %w", value.id, err)
		}
	}
	return nil
}

func closeCodingLedgerQuestionsTx(
	ctx context.Context,
	tx pgx.Tx,
	ledgerID int64,
	ownerTurnID string,
	ledger model.EngineeringLedger,
) error {
	for _, value := range ledger.DecisionsAndQuestions {
		if value.Kind != model.LedgerQuestion {
			continue
		}
		status := ""
		var answer any
		if value.Disposition == model.LedgerDispositionTombstoned {
			status = "withdrawn"
		} else if strings.TrimSpace(value.Answer) != "" {
			status = "answered"
			answer = value.Answer
		}
		if status == "" {
			continue
		}
		var answeredBy any
		if status == "answered" {
			answeredBy = ownerTurnID
		}
		if _, err := tx.Exec(ctx, `
			UPDATE coding_ledger_questions
			SET status=$3,answer=$4,answered_by_turn_id=$5,answered_at=clock_timestamp()
			WHERE ledger_id=$1 AND question_id=$2 AND status='open'
		`, ledgerID, value.ID, status, answer, answeredBy); err != nil {
			return fmt.Errorf("close coding ledger question %s: %w", value.ID, err)
		}
	}
	return nil
}

func syncCodingLedgerQuestionsAndFindingsTx(ctx context.Context, tx pgx.Tx, ledgerID, revision int64, ownerTurnID string, ledger model.EngineeringLedger) error {
	for _, value := range ledger.DecisionsAndQuestions {
		if value.Kind != model.LedgerQuestion {
			continue
		}
		status := "open"
		var answer, answeredBy, answeredAt any
		if value.Disposition == model.LedgerDispositionTombstoned {
			status = "withdrawn"
			answeredAt = "now"
		} else if strings.TrimSpace(value.Answer) != "" {
			status = "answered"
			answer = value.Answer
			answeredBy = ownerTurnID
			answeredAt = "now"
		}
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM coding_ledger_questions WHERE ledger_id=$1 AND question_id=$2)`, ledgerID, value.ID).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			if status == "answered" {
				// The revision owner observes the prior revision, while an answer is
				// valid only from authority observing the newly projected question.
				// Derive a stable, separately disposed answer turn from the exact
				// owner-supplied answer rather than weakening that schema invariant.
				answerTurnID := codingLedgerAnswerTurnID(ownerTurnID, value.ID)
				answerText := fmt.Sprintf("Answer %s: %s", value.ID, value.Answer)
				if err := insertCodingLedgerOwnerTurnTx(ctx, tx, ledgerID, answerTurnID, revision, "planning", answerText); err != nil {
					return err
				}
				if err := insertCodingLedgerDispositionTx(ctx, tx, ledgerID, revision, answerTurnID,
					model.OwnerTurnDisposition{Kind: model.OwnerTurnRepresented, ItemIDs: []model.LedgerItemID{value.ID}}, ledger); err != nil {
					return err
				}
				answeredBy = answerTurnID
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO coding_ledger_questions(
					ledger_id,question_id,item_id,revision,prompt,status,answer,answered_by_turn_id,answered_at
				) VALUES($1,$2,$2,$3,$4,$5,$6,$7,CASE WHEN $8::text='now' THEN clock_timestamp() END)
			`, ledgerID, value.ID, revision, value.Prompt, status, answer, answeredBy, answeredAt); err != nil {
				return fmt.Errorf("record coding ledger question %s: %w", value.ID, err)
			}
		} else {
			// Question closure is applied before the revision insert while the
			// answering turn's base is still the exact current revision. Terminal
			// question events are immutable; restoring the ledger item therefore
			// preserves that event while the revisioned item again carries the
			// active/open state.
			continue
		}
	}
	for _, value := range ledger.Findings {
		status := "open"
		var resolvedAt any
		if value.Disposition == model.LedgerDispositionTombstoned || len(value.ResolvedBy) > 0 {
			status = "resolved"
			resolvedAt = "now"
		}
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM coding_ledger_findings WHERE ledger_id=$1 AND finding_id=$2)`, ledgerID, value.ID).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			if _, err := tx.Exec(ctx, `
				INSERT INTO coding_ledger_findings(
					ledger_id,finding_id,item_id,revision,finding_kind,status,observation,resolution_item_id,resolved_at
				) VALUES($1,$2,$2,$3,$4,$5,$6,$7,CASE WHEN $8::text='now' THEN clock_timestamp() END)
			`, ledgerID, value.ID, revision, value.Kind, status, value.Observation, firstLedgerID(value.ResolvedBy), resolvedAt); err != nil {
				return fmt.Errorf("record coding ledger finding %s: %w", value.ID, err)
			}
		} else {
			var currentStatus string
			var currentResolution *string
			if err := tx.QueryRow(ctx, `
				SELECT status,resolution_item_id FROM coding_ledger_findings WHERE ledger_id=$1 AND finding_id=$2
			`, ledgerID, value.ID).Scan(&currentStatus, &currentResolution); err != nil {
				return err
			}
			desiredResolution := ""
			if len(value.ResolvedBy) > 0 {
				desiredResolution = string(value.ResolvedBy[0])
			}
			// Finding rows are immutable terminal audit events. Restoring the
			// revisioned item reopens only that item projection; it must not attempt
			// the forbidden resolved-to-open rewrite of the historical event.
			if currentStatus != "open" && status == "open" {
				continue
			}
			if currentStatus != status || (currentResolution == nil) != (desiredResolution == "") ||
				(currentResolution != nil && *currentResolution != desiredResolution) {
				if _, err := tx.Exec(ctx, `
					UPDATE coding_ledger_findings SET status=$3,resolution_item_id=$4,
						resolved_at=CASE WHEN $5::text='now' THEN clock_timestamp() END
					WHERE ledger_id=$1 AND finding_id=$2
				`, ledgerID, value.ID, status, firstLedgerID(value.ResolvedBy), resolvedAt); err != nil {
					return fmt.Errorf("update coding ledger finding %s: %w", value.ID, err)
				}
			}
		}
	}
	return nil
}

func firstLedgerID(ids []model.LedgerItemID) any {
	if len(ids) == 0 {
		return nil
	}
	return ids[0]
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
