package codingagent

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/gryph/omnidex/internal/model"
)

const (
	coveragePlanningQuestion = "owner-turn-coverage"
	AuditPlanningQuestion    = "mandatory-pre-sign-off-audit"
)

// PlanningCoverageJudgment is read-only semantic evidence, not mutation or
// approval authority. Suggestions remain outside the authoritative ledger
// until the owner explicitly acts on them.
type PlanningCoverageJudgment struct {
	Question     string   `json:"question"`
	BaseRevision int64    `json:"base_revision"`
	Covered      bool     `json:"covered"`
	Suggestions  []string `json:"suggestions"`
	Explanation  string   `json:"explanation"`
}

func (judgment PlanningCoverageJudgment) validate(question string, ledger EngineeringLedger) error {
	if judgment.Question != question {
		return fmt.Errorf("planning judgment answered %q instead of named question %q", judgment.Question, question)
	}
	if judgment.BaseRevision != ledger.Revision {
		return fmt.Errorf("planning judgment revision %d does not match database revision %d", judgment.BaseRevision, ledger.Revision)
	}
	if strings.TrimSpace(judgment.Explanation) == "" {
		return fmt.Errorf("planning judgment requires a concrete explanation")
	}
	for _, suggestion := range judgment.Suggestions {
		if strings.TrimSpace(suggestion) == "" {
			return fmt.Errorf("planning judgment suggestions must be concrete")
		}
	}
	return nil
}

// PlanningAuditReceipt gates sign-off at one exact revision. Ready remains only
// analysis evidence; ValidatePlanningSignoff never changes ledger state.
type PlanningAuditReceipt struct {
	AuditID  string                   `json:"audit_id"`
	Revision int64                    `json:"revision"`
	Ready    bool                     `json:"ready"`
	Judgment PlanningCoverageJudgment `json:"judgment"`
}

type PersistedPlanningTurn struct {
	OwnerTurnID  string                            `json:"owner_turn_id"`
	BaseRevision int64                             `json:"base_revision"`
	ExactText    string                            `json:"exact_text"`
	Dispositions []model.OwnerStatementDisposition `json:"dispositions"`
}

func (controller *Controller) AnalyzePlanningCoverage(
	ctx context.Context,
	turn PlanningTurn,
	ledger EngineeringLedger,
	declared ...model.OwnerTurnDisposition,
) (PlanningCoverageJudgment, error) {
	var dispositions []model.OwnerStatementDisposition
	if len(declared) > 0 {
		dispositions = declared[0].StatementDispositions()
	}
	return controller.runPlanningJudgment(
		ctx, PlanningCoverage, coveragePlanningQuestion,
		"Does the resulting revision durably dispose every actionable statement in this exact owner turn?",
		turn, ledger, dispositionRelevantItemIDs(dispositions), nil, dispositions,
	)
}

func dispositionRelevantItemIDs(dispositions []model.OwnerStatementDisposition) []model.LedgerItemID {
	result := make([]model.LedgerItemID, 0)
	for _, disposition := range dispositions {
		result = append(result, disposition.ItemIDs...)
		result = append(result, disposition.QuestionIDs...)
	}
	return uniquePlanningIDs(result)
}

// AuditPlanningLedger always starts a fresh provider call over authoritative
// ledger state. Callers must load that state from the repository immediately
// before invoking it; no provider thread or prior judgment is reused.
func (controller *Controller) AuditPlanningLedger(
	ctx context.Context,
	auditID string,
	turn PlanningTurn,
	ledger EngineeringLedger,
	persistedTurns ...[]PersistedPlanningTurn,
) (PlanningAuditReceipt, error) {
	if strings.TrimSpace(auditID) == "" {
		return PlanningAuditReceipt{}, fmt.Errorf("planning audit requires a stable identity")
	}
	all := planningItemPointers(&ledger)
	if len(all) == 0 {
		return PlanningAuditReceipt{}, fmt.Errorf("planning audit requires persisted planning records")
	}
	var judgment PlanningCoverageJudgment
	partitions := make([][]model.LedgerItemID, 0, (len(all)+defaultPlanningItemLimit-1)/defaultPlanningItemLimit)
	for offset := 0; offset < len(all); offset += defaultPlanningItemLimit {
		end := offset + defaultPlanningItemLimit
		if end > len(all) {
			end = len(all)
		}
		ids := make([]model.LedgerItemID, 0, end-offset)
		for _, item := range all[offset:end] {
			ids = append(ids, item.ID)
		}
		partitions = append(partitions, ids)
		partitionQuestion := fmt.Sprintf("Does persisted planning-record partition %d/%d contain complete, coherent, and verifiable engineering coverage for owner sign-off?", offset/defaultPlanningItemLimit+1, (len(all)+defaultPlanningItemLimit-1)/defaultPlanningItemLimit)
		var err error
		judgment, err = controller.runPlanningJudgment(ctx, PlanningAudit, AuditPlanningQuestion,
			partitionQuestion, turn, ledger, ids, nil, nil)
		if err != nil {
			return PlanningAuditReceipt{}, err
		}
		if !judgment.Covered {
			break
		}
	}
	// Local partition checks cannot discover a semantic duplicate or conflict
	// that straddles their boundary. Compare every pair through two half-sized
	// windows so every cross-partition record pair is observed while each call
	// remains inside the same hard context limit.
	if judgment.Covered && len(partitions) > 1 {
		halves := make([][]model.LedgerItemID, 0, len(all)/(defaultPlanningItemLimit/2)+1)
		const half = defaultPlanningItemLimit / 2
		for _, partition := range partitions {
			for offset := 0; offset < len(partition); offset += half {
				end := offset + half
				if end > len(partition) {
					end = len(partition)
				}
				halves = append(halves, append([]model.LedgerItemID(nil), partition[offset:end]...))
			}
		}
		for left := 0; left < len(halves) && judgment.Covered; left++ {
			for right := left + 1; right < len(halves) && judgment.Covered; right++ {
				ids := append(append([]model.LedgerItemID(nil), halves[left]...), halves[right]...)
				question := fmt.Sprintf("Do planning-record windows %d and %d contain any semantic duplicate or contradiction across their boundary?", left+1, right+1)
				var err error
				judgment, err = controller.runPlanningJudgment(ctx, PlanningAudit, AuditPlanningQuestion,
					question, turn, ledger, ids, nil, nil)
				if err != nil {
					return PlanningAuditReceipt{}, err
				}
			}
		}
	}
	if judgment.Covered && len(persistedTurns) > 0 {
		turnPartitions := partitionPersistedPlanningTurns(persistedTurns[0], defaultPlanningItemLimit/2)
		for index, turns := range turnPartitions {
			partitionQuestion := fmt.Sprintf("Does persisted owner-turn partition %d/%d have a durable disposition for every actionable statement?", index+1, len(turnPartitions))
			ids := make([]model.LedgerItemID, 0)
			for _, persisted := range turns {
				for _, disposition := range persisted.Dispositions {
					ids = append(ids, disposition.ItemIDs...)
					ids = append(ids, disposition.QuestionIDs...)
				}
			}
			var err error
			judgment, err = controller.runPlanningJudgment(ctx, PlanningAudit, AuditPlanningQuestion,
				partitionQuestion, turn, ledger, ids, turns, nil)
			if err != nil {
				return PlanningAuditReceipt{}, err
			}
			if !judgment.Covered {
				break
			}
		}
		for left := 0; left < len(turnPartitions) && judgment.Covered; left++ {
			for right := left + 1; right < len(turnPartitions) && judgment.Covered; right++ {
				turns := append(append([]PersistedPlanningTurn(nil), turnPartitions[left]...), turnPartitions[right]...)
				ids := make([]model.LedgerItemID, 0, defaultPlanningItemLimit)
				for _, persisted := range turns {
					for _, disposition := range persisted.Dispositions {
						ids = append(ids, disposition.ItemIDs...)
						ids = append(ids, disposition.QuestionIDs...)
					}
				}
				question := fmt.Sprintf("Do persisted owner-turn partitions %d and %d contain any cross-partition contradiction or undisposed instruction?", left+1, right+1)
				var err error
				judgment, err = controller.runPlanningJudgment(ctx, PlanningAudit, AuditPlanningQuestion, question, turn, ledger, ids, turns, nil)
				if err != nil {
					return PlanningAuditReceipt{}, err
				}
			}
		}
	}
	return PlanningAuditReceipt{
		AuditID: auditID, Revision: ledger.Revision,
		Ready: judgment.Covered, Judgment: judgment,
	}, nil
}

func (controller *Controller) runPlanningJudgment(
	ctx context.Context,
	role Role,
	name, question string,
	turn PlanningTurn,
	ledger EngineeringLedger,
	relevant []model.LedgerItemID,
	persistedTurns []PersistedPlanningTurn,
	declaredDispositions []model.OwnerStatementDisposition,
) (PlanningCoverageJudgment, error) {
	if err := controller.validatePlanning(); err != nil {
		return PlanningCoverageJudgment{}, err
	}
	if err := turn.validate(false); err != nil {
		return PlanningCoverageJudgment{}, err
	}
	if ledger.Revision != turn.ObservedRevision {
		return PlanningCoverageJudgment{}, fmt.Errorf(
			"planning analysis observed revision %d but database revision is %d",
			turn.ObservedRevision, ledger.Revision,
		)
	}
	var ledgerErr error
	if role == PlanningCoverage {
		ledgerErr = ledger.ValidateDraft()
	} else {
		ledgerErr = ledger.Validate()
	}
	if ledgerErr != nil {
		return PlanningCoverageJudgment{}, ledgerErr
	}
	contextLedger := BoundedPlanningLedger(ledger, relevant, defaultPlanningItemLimit)
	if len(uniquePlanningIDs(relevant)) > defaultPlanningItemLimit {
		return PlanningCoverageJudgment{}, fmt.Errorf("planning analysis requires %d explicit records; partition the engineering question to at most %d records", len(uniquePlanningIDs(relevant)), defaultPlanningItemLimit)
	}
	call := controller.planningAssignment(role, name, question, turn, &contextLedger)
	if len(declaredDispositions) > 0 {
		raw, err := json.Marshal(declaredDispositions)
		if err != nil {
			return PlanningCoverageJudgment{}, err
		}
		call.ConstructionFacts = append(call.ConstructionFacts, ConstructionFact{Kind: "declared_owner_turn_dispositions", Quote: string(raw)})
	}
	if len(persistedTurns) > 0 {
		raw, err := json.Marshal(persistedTurns)
		if err != nil {
			return PlanningCoverageJudgment{}, err
		}
		call.ConstructionFacts = append(call.ConstructionFacts, ConstructionFact{Kind: "persisted_owner_turns", Quote: string(raw)})
	}
	result, err := controller.invoke(ctx, call, 1)
	if err != nil {
		return PlanningCoverageJudgment{}, err
	}
	var judgment PlanningCoverageJudgment
	if err := decodeContract(result.Content, &judgment); err != nil {
		return PlanningCoverageJudgment{}, err
	}
	if err := judgment.validate(name, ledger); err != nil {
		return PlanningCoverageJudgment{}, err
	}
	return judgment, nil
}

func uniquePlanningIDs(ids []model.LedgerItemID) []model.LedgerItemID {
	seen := map[model.LedgerItemID]bool{}
	result := make([]model.LedgerItemID, 0, len(ids))
	for _, id := range ids {
		if id != "" && !seen[id] {
			seen[id] = true
			result = append(result, id)
		}
	}
	return result
}

func partitionPersistedPlanningTurns(turns []PersistedPlanningTurn, limit int) [][]PersistedPlanningTurn {
	result := make([][]PersistedPlanningTurn, 0)
	for _, turn := range turns {
		current := PersistedPlanningTurn{OwnerTurnID: turn.OwnerTurnID, BaseRevision: turn.BaseRevision, ExactText: turn.ExactText}
		count := 0
		flush := func() {
			if len(current.Dispositions) == 0 {
				return
			}
			result = append(result, []PersistedPlanningTurn{current})
			current = PersistedPlanningTurn{OwnerTurnID: turn.OwnerTurnID, BaseRevision: turn.BaseRevision, ExactText: turn.ExactText}
			count = 0
		}
		for _, disposition := range turn.Dispositions {
			ids := append(append([]model.LedgerItemID(nil), disposition.ItemIDs...), disposition.QuestionIDs...)
			if len(ids) == 0 {
				ids = []model.LedgerItemID{""}
			}
			for offset := 0; offset < len(ids); {
				if count == limit {
					flush()
				}
				take := limit - count
				if take > len(ids)-offset {
					take = len(ids) - offset
				}
				part := disposition
				part.ItemIDs, part.QuestionIDs = nil, nil
				for _, id := range ids[offset : offset+take] {
					if id == "" {
						continue
					}
					isQuestion := false
					for _, questionID := range disposition.QuestionIDs {
						if questionID == id {
							isQuestion = true
							break
						}
					}
					if isQuestion {
						part.QuestionIDs = append(part.QuestionIDs, id)
					} else {
						part.ItemIDs = append(part.ItemIDs, id)
					}
				}
				current.Dispositions = append(current.Dispositions, part)
				count += take
				offset += take
			}
		}
		flush()
	}
	return result
}

// RequiresPlanningCoverage identifies structural checkpoints. Review-state and
// ordering-only controls stay mechanical; semantic shape or coverage changes
// receive one bounded coverage judgment.
func RequiresPlanningCoverage(mutation LedgerMutation) bool {
	for _, operation := range mutation.Operations {
		switch operation.Kind {
		case model.LedgerMutationAdd, model.LedgerMutationAmend, model.LedgerMutationRemove, model.LedgerMutationRestore:
			return true
		}
	}
	return false
}

// RequiresPlanningCoverageForTurn distinguishes deterministic interface
// controls from natural-language turns. Every natural-language turn is checked
// against its declared dispositions, even when it changes only ordering,
// review state, or records an out-of-scope clause.
func RequiresPlanningCoverageForTurn(mutation LedgerMutation, direct bool) bool {
	return !direct || RequiresPlanningCoverage(mutation)
}

// ValidatePlanningSignoff proves only that sign-off may be offered. It cannot
// freeze, agree, or otherwise mutate the proposal.
func ValidatePlanningSignoff(ledger EngineeringLedger, audit PlanningAuditReceipt) error {
	if err := ledger.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(audit.AuditID) == "" || audit.Revision != ledger.Revision ||
		audit.Judgment.BaseRevision != ledger.Revision {
		return fmt.Errorf("planning sign-off requires a fresh audit of the exact displayed revision")
	}
	if !audit.Ready || !audit.Judgment.Covered {
		return fmt.Errorf("planning audit did not declare the displayed revision ready")
	}
	for _, item := range planningItemPointers(&ledger) {
		if item.Disposition == model.LedgerDispositionActive && item.ReviewState != model.LedgerReviewAgreed {
			return fmt.Errorf("active planning item %s is not agreed", item.ID)
		}
	}
	for _, value := range ledger.DecisionsAndQuestions {
		if value.Disposition == model.LedgerDispositionActive &&
			(value.Kind == model.LedgerQuestion || value.Kind == model.LedgerDecision) &&
			strings.TrimSpace(value.Answer) == "" {
			return fmt.Errorf("planning item %s remains unresolved", value.ID)
		}
	}
	for _, finding := range ledger.Findings {
		if finding.Disposition == model.LedgerDispositionActive &&
			finding.Kind == model.LedgerFindingPlanning && len(finding.ResolvedBy) == 0 {
			return fmt.Errorf("planning finding %s remains unresolved", finding.ID)
		}
	}
	return nil
}

// BoundedPlanningLedger returns a deterministic relevant slice. Explicit IDs and
// their normalized neighbors are selected first, followed by ranked active
// records. It never mutates the authoritative ledger.
func BoundedPlanningLedger(ledger EngineeringLedger, relevant []model.LedgerItemID, limit int) EngineeringLedger {
	if limit < 1 {
		limit = defaultPlanningItemLimit
	}
	selected := map[model.LedgerItemID]bool{}
	explicit := map[model.LedgerItemID]bool{}
	for _, id := range relevant {
		if id != "" {
			selected[id] = true
			explicit[id] = true
		}
	}
	for _, task := range ledger.Tasks {
		if selected[task.ID] {
			for _, id := range task.RequirementIDs {
				selected[id] = true
			}
			for _, id := range task.AcceptanceCriterionIDs {
				selected[id] = true
			}
			if task.GroupID != "" {
				selected[task.GroupID] = true
			}
		}
	}
	for _, dependency := range ledger.Dependencies {
		if selected[dependency.TaskID] || selected[dependency.PrerequisiteID] {
			selected[dependency.ID], selected[dependency.TaskID], selected[dependency.PrerequisiteID] = true, true, true
		}
	}
	for _, check := range ledger.Checks {
		ids := append(append([]model.LedgerItemID{}, check.RequirementIDs...), check.AcceptanceCriterionIDs...)
		for _, id := range ids {
			if selected[id] {
				selected[check.ID] = true
			}
		}
	}
	type candidate struct {
		id     model.LedgerItemID
		rank   int64
		active bool
	}
	candidates := make([]candidate, 0)
	for _, item := range planningItemPointers(&ledger) {
		candidates = append(candidates, candidate{id: item.ID, rank: item.Rank, active: item.Disposition == model.LedgerDispositionActive})
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].active != candidates[j].active {
			return candidates[i].active
		}
		if candidates[i].rank != candidates[j].rank {
			return candidates[i].rank < candidates[j].rank
		}
		return candidates[i].id < candidates[j].id
	})
	for _, value := range candidates {
		if len(selected) >= limit {
			break
		}
		if value.active {
			selected[value.id] = true
		}
	}
	if len(selected) > limit {
		trimmed := map[model.LedgerItemID]bool{}
		for _, value := range candidates {
			if explicit[value.id] {
				trimmed[value.id] = true
			}
		}
		// Callers partition or reject oversized explicit sets before invocation.
		// Keep a deterministic non-empty prefix here as a defensive boundary;
		// never erase explicitly relevant context.
		if len(trimmed) > limit {
			trimmed = map[model.LedgerItemID]bool{}
			for _, value := range candidates {
				if explicit[value.id] && len(trimmed) < limit {
					trimmed[value.id] = true
				}
			}
		}
		for _, value := range candidates {
			if selected[value.id] && !trimmed[value.id] && len(trimmed) < limit {
				trimmed[value.id] = true
			}
		}
		selected = trimmed
	}
	result := EngineeringLedger{ID: ledger.ID, Revision: ledger.Revision}
	for _, value := range ledger.Objectives {
		if selected[value.ID] {
			result.Objectives = append(result.Objectives, value)
		}
	}
	for _, value := range ledger.Requirements {
		if selected[value.ID] {
			result.Requirements = append(result.Requirements, value)
		}
	}
	for _, value := range ledger.Constraints {
		if selected[value.ID] {
			result.Constraints = append(result.Constraints, value)
		}
	}
	for _, value := range ledger.Groups {
		if selected[value.ID] {
			result.Groups = append(result.Groups, value)
		}
	}
	for _, value := range ledger.Tasks {
		if selected[value.ID] {
			result.Tasks = append(result.Tasks, value)
		}
	}
	for _, value := range ledger.AcceptanceCriteria {
		if selected[value.ID] {
			result.AcceptanceCriteria = append(result.AcceptanceCriteria, value)
		}
	}
	for _, value := range ledger.Checks {
		if selected[value.ID] {
			result.Checks = append(result.Checks, value)
		}
	}
	for _, value := range ledger.DecisionsAndQuestions {
		if selected[value.ID] {
			result.DecisionsAndQuestions = append(result.DecisionsAndQuestions, value)
		}
	}
	for _, value := range ledger.Dependencies {
		if selected[value.ID] {
			result.Dependencies = append(result.Dependencies, value)
		}
	}
	for _, value := range ledger.Findings {
		if selected[value.ID] {
			result.Findings = append(result.Findings, value)
		}
	}
	return result
}

func planningContextItemCount(ledger EngineeringLedger) int {
	return len(planningItemPointers(&ledger))
}
