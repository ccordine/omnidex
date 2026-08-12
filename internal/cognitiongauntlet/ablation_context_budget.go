package cognitiongauntlet

import (
	"bytes"
	"fmt"
	"reflect"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/cognitionreplay"
	"github.com/gryph/omnidex/internal/contextbuilder"
	"github.com/gryph/omnidex/internal/llm"
)

type ablationContextBudgetFailure struct {
	projection      contextbuilder.Projection
	snapshot        semanticRuntimeSnapshot
	modelInputBytes int
}

func (failure *ablationContextBudgetFailure) Error() string {
	if failure == nil {
		return "cognition ablation context budget exhausted without exact authority"
	}
	return fmt.Sprintf(
		"cognition ablation context budget exhausted: exact raw model input needs %d bytes",
		failure.modelInputBytes,
	)
}

func (*ablationContextBudgetFailure) Unwrap() error { return errAblationContextBudget }

func (failure *ablationContextBudgetFailure) clone() *ablationContextBudgetFailure {
	if failure == nil {
		return nil
	}
	return &ablationContextBudgetFailure{
		projection: cloneAblationProjection(failure.projection),
		snapshot:   failure.snapshot.clone(), modelInputBytes: failure.modelInputBytes,
	}
}

func buildAblationContextBudgetEvidence(
	failure *ablationContextBudgetFailure,
	builder *ablationContentBuilder,
) (*ablationContextBudgetEvidence, error) {
	if failure == nil {
		return nil, nil
	}
	if failure.projection.Validate() != nil || failure.modelInputBytes <= 0 {
		return nil, fmt.Errorf("ablation context budget failure lacks exact projection authority")
	}
	content, err := builder.content(
		"ablation-context-budget-projection", "text/plain; charset=utf-8",
		[]byte(failure.projection.Rendered), false,
	)
	if err != nil {
		return nil, err
	}
	return &ablationContextBudgetEvidence{
		Projection: ablationProjectionEvidence{
			Projection: cloneAblationProjection(failure.projection), Content: content,
		},
		Snapshot: failure.snapshot.clone(), ModelInputBytes: failure.modelInputBytes,
	}, nil
}

func verifyAblationContextBudgetEvidence(
	root ablationEvidenceRoot,
	store *ablationEvidenceContentStore,
	role cognitionreplay.ChunkedBlobRole,
) error {
	value := root.ContextBudget
	want := root.TerminalCause.Kind == ablationTerminalContextBudget
	if (value != nil) != want {
		return fmt.Errorf("ablation context budget evidence presence differs from terminal cause")
	}
	if value == nil {
		return nil
	}
	projection := value.Projection.Projection
	rendered, err := store.read(value.Projection.Content, role)
	if err != nil || projection.Validate() != nil ||
		!bytes.Equal(rendered, []byte(projection.Rendered)) {
		return fmt.Errorf("ablation context budget projection changed: %v", err)
	}
	snapshot, err := value.Snapshot.runtimeSnapshot()
	if err != nil || value.Snapshot.CurrentRevision != root.Terminal.Revision ||
		value.Snapshot.Attempt != root.Actor ||
		!reflect.DeepEqual(value.Snapshot.Goal, root.Goal) ||
		!reflect.DeepEqual(value.Snapshot.CurrentObligation, root.Obligation) ||
		ablationProjectionRef(projection) != value.Snapshot.ContextProjection {
		return fmt.Errorf("ablation context budget snapshot changed: %v", err)
	}
	catalog := root.WorldCatalog
	if root.Variant == VariantRawShell {
		catalog, err = rawShellCatalog()
	}
	if err != nil || !reflect.DeepEqual(value.Snapshot.ActionCatalog, catalog) {
		return fmt.Errorf("ablation context budget action catalog changed: %v", err)
	}
	wantBudget, err := root.PublicRunAuthority.Budget.RuntimeBudget()
	if err != nil {
		return err
	}
	wantBudget.RemainingPolicyCalls = uint32(
		root.PublicRunAuthority.Budget.ModelCalls - len(root.Calls),
	)
	if value.Snapshot.Budget != wantBudget {
		return fmt.Errorf("ablation context budget snapshot changed its frozen budget")
	}
	envelope, err := cognitionpolicy.MeasureEnvelope(snapshot, projection)
	if err != nil {
		return err
	}
	modelInput, err := llm.ExactPreparedModelInput(envelope.JSON, llm.MinimalGeneratePrompt)
	if err != nil || len([]byte(modelInput)) != value.ModelInputBytes ||
		value.ModelInputBytes <= wantBudget.MaxInputBytes {
		return fmt.Errorf("ablation context budget overflow does not rederive: %v", err)
	}
	return nil
}

func verifyAblationCallInputAuthorities(root ablationEvidenceRoot) error {
	catalog := root.WorldCatalog
	var err error
	if root.Variant == VariantRawShell {
		catalog, err = rawShellCatalog()
	}
	if err != nil {
		return err
	}
	for index, call := range root.Calls {
		wantBudget, budgetErr := root.PublicRunAuthority.Budget.RuntimeBudget()
		if budgetErr != nil {
			return budgetErr
		}
		wantBudget.RemainingPolicyCalls = uint32(
			root.PublicRunAuthority.Budget.ModelCalls - index,
		)
		if call.Attempt.Actor != root.Actor || call.Snapshot.Attempt != root.Actor ||
			!reflect.DeepEqual(call.Snapshot.Goal, root.Goal) ||
			!reflect.DeepEqual(call.Snapshot.CurrentObligation, root.Obligation) ||
			!reflect.DeepEqual(call.Snapshot.ActionCatalog, catalog) ||
			call.Snapshot.Budget != wantBudget {
			return fmt.Errorf("ablation call %d changed root task or budget authority", index+1)
		}
	}
	return nil
}

func verifyAblationRootTaskAuthority(root ablationEvidenceRoot) error {
	if root.Actor.Validate() != nil || root.Goal.Validate() != nil ||
		root.Completion.Validate() != nil || root.Obligation.Validate() != nil {
		return fmt.Errorf("ablation root task authority is invalid")
	}
	check, err := root.Completion.Resolve(root.Goal)
	if err != nil {
		return err
	}
	wantID, err := cognition.DeriveObligationID(
		root.EpisodeID, cognition.InitialObligationGeneration, "", root.Goal, check,
	)
	if err != nil || root.Obligation.ID != wantID || root.Obligation.ParentID != "" ||
		root.Obligation.Status != cognition.ObligationActive ||
		root.Obligation.CreatedGeneration != cognition.InitialObligationGeneration ||
		root.Obligation.CompletionCheck != check ||
		!reflect.DeepEqual(root.Obligation.Desired, root.Goal) ||
		len(root.Obligation.DependsOn) != 0 || len(root.Obligation.SupportingRefs) != 0 {
		return fmt.Errorf("ablation root obligation is not code-derived: %v", err)
	}
	return nil
}
