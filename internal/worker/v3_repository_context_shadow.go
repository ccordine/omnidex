package worker

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strconv"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/contextbuilder"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
	"github.com/gryph/omnidex/internal/taskstate"
	"github.com/gryph/omnidex/internal/workingset"
)

type repositoryShadowContextStore interface {
	CurrentWorkingSet(context.Context, int64) (workingset.Snapshot, error)
	CreateCurrentWorkingSet(context.Context, int64, int64, workingset.Budget) (workingset.Snapshot, error)
	ApplyWorkingSetCommand(context.Context, int64, int64, workingset.Command) (workingset.Event, error)
	StoreContextProjection(
		context.Context, queue.ContextProjectionAuthority, contextbuilder.Projection,
	) (queue.ContextProjectionRecord, error)
}

func prepareRepositoryShadowContext(
	session *directCodingSession,
	job assemblyline.PortableJob,
) (string, error) {
	if !repositoryShadowEligible(job.Kind) {
		return "", nil
	}
	if session == nil || session.runtime == nil || session.runtime.claim == nil ||
		session.runtime.svc == nil || session.runtime.svc.repo == nil {
		return "", fmt.Errorf("repository shadow context requires a claimed PostgreSQL runtime")
	}
	if session.repositoryIndex == nil {
		return "", fmt.Errorf("repository shadow context requires an existing-repository index")
	}
	return prepareRepositoryShadowProjection(
		session.runtime.ctx, session.runtime.svc.repo, session.runtime.claim, job,
	)
}

func prepareRepositoryShadowProjection(
	ctx context.Context,
	store repositoryShadowContextStore,
	claim *model.ClaimedStep,
	job assemblyline.PortableJob,
) (string, error) {
	if ctx == nil || store == nil || claim == nil {
		return "", fmt.Errorf("repository shadow projection requires context, store, and claim")
	}
	if !repositoryShadowEligible(job.Kind) {
		return "", fmt.Errorf("repository shadow projection rejects work kind %q", job.Kind)
	}
	if err := validateRepositoryShadowClaim(claim); err != nil {
		return "", err
	}
	plan, err := repositoryShadowPlan(job)
	if err != nil {
		return "", err
	}
	set, err := acquireRepositoryShadowWorkingSet(ctx, store, claim, job, plan.sources)
	if err != nil {
		return "", err
	}
	materials := make([]contextbuilder.Material, len(plan.sources))
	for index := range plan.sources {
		materials[index] = plan.sources[index].material
	}
	projection, err := contextbuilder.Build(contextbuilder.BuildInput{
		WorkID: job.ID, Spec: plan.spec, WorkingSet: set, Materials: materials,
	})
	if err != nil {
		return "", fmt.Errorf("build repository shadow context projection: %w", err)
	}
	record, err := store.StoreContextProjection(ctx, queue.ContextProjectionAuthority{
		JobID: claim.Job.ID, Generation: claim.Job.CurrentGeneration, StepID: claim.Step.ID,
		WorkKind: string(job.Kind), Mode: queue.ContextProjectionModeShadow,
	}, projection)
	if err != nil {
		return "", fmt.Errorf("store repository shadow context projection: %w", err)
	}
	if record.Authority.Mode != queue.ContextProjectionModeShadow || record.Projection.ID != projection.ID {
		return "", fmt.Errorf("stored repository shadow projection returned mismatched immutable authority")
	}
	return record.Projection.ID, nil
}

func validateRepositoryShadowClaim(claim *model.ClaimedStep) error {
	if claim.Job.ID <= 0 || claim.Job.CurrentGeneration <= 0 || claim.Job.Status != model.JobStatusRunning ||
		claim.Step.ID <= 0 || claim.Step.JobID != claim.Job.ID ||
		claim.Step.Generation != claim.Job.CurrentGeneration || claim.Step.Status != model.StepStatusRunning ||
		claim.Step.SupersededAtGeneration != nil {
		return fmt.Errorf("repository shadow context requires one current running job generation and step")
	}
	return nil
}

func acquireRepositoryShadowWorkingSet(
	ctx context.Context,
	store repositoryShadowContextStore,
	claim *model.ClaimedStep,
	job assemblyline.PortableJob,
	sources []repositoryShadowSource,
) (*workingset.Set, error) {
	owner := workingset.Owner{JobID: claim.Job.ID, Generation: claim.Job.CurrentGeneration}
	snapshot, err := store.CurrentWorkingSet(ctx, owner.JobID)
	if errors.Is(err, queue.ErrWorkingSetNotFound) {
		snapshot, err = store.CreateCurrentWorkingSet(
			ctx, owner.JobID, owner.Generation, repositoryShadowWorkingSetBudget(),
		)
	}
	if err != nil {
		return nil, fmt.Errorf("create or resume repository shadow working set: %w", err)
	}
	owner.LedgerID = snapshot.Owner.LedgerID
	if err := validateRepositoryShadowSnapshot(snapshot, owner, sources); err != nil {
		return nil, err
	}
	for _, source := range sources {
		if repositoryShadowSnapshotHasItem(snapshot, source.item.ID) {
			continue
		}
		bound := source.item
		bound.Scope = snapshot.Scope
		commandID, commandErr := workingset.NewCommandID(
			"repository-context-shadow-v1", strconv.FormatInt(owner.JobID, 10),
			strconv.FormatInt(owner.Generation, 10), job.ID, string(bound.ID),
			strconv.FormatUint(snapshot.Version, 10),
		)
		if commandErr != nil {
			return nil, commandErr
		}
		if _, err := store.ApplyWorkingSetCommand(ctx, owner.JobID, owner.Generation, workingset.AcquireCommand{
			CommandID: commandID, ExpectedVersion: snapshot.Version,
			Actor: taskstate.AuthorityCode, Request: bound,
		}); err != nil {
			return nil, fmt.Errorf("acquire repository shadow context item %q: %w", bound.ID, err)
		}
		snapshot, err = store.CurrentWorkingSet(ctx, owner.JobID)
		if err != nil {
			return nil, fmt.Errorf("reload repository shadow working set: %w", err)
		}
		if err := validateRepositoryShadowSnapshot(snapshot, owner, sources); err != nil {
			return nil, err
		}
	}
	set, err := workingset.Restore(snapshot)
	if err != nil {
		return nil, fmt.Errorf("restore repository shadow working set: %w", err)
	}
	return set, nil
}

func repositoryShadowSnapshotHasItem(snapshot workingset.Snapshot, id workingset.ItemID) bool {
	for _, item := range snapshot.Items {
		if item.ID == id {
			return true
		}
	}
	return false
}

func validateRepositoryShadowSnapshot(
	snapshot workingset.Snapshot,
	owner workingset.Owner,
	sources []repositoryShadowSource,
) error {
	if _, err := workingset.Restore(snapshot); err != nil {
		return fmt.Errorf("validate repository shadow working set: %w", err)
	}
	if snapshot.Owner != owner || snapshot.Status != workingset.StatusActive ||
		snapshot.Budget != repositoryShadowWorkingSetBudget() || len(snapshot.ClosedScopes) != 0 ||
		snapshot.ClosedTick != 0 || snapshot.CloseReason != "" {
		return fmt.Errorf("repository shadow working set has mismatched owner, lifecycle, or fixed budget")
	}
	items := make(map[workingset.ItemID]workingset.Item, len(snapshot.Items))
	for _, item := range snapshot.Items {
		items[item.ID] = item
	}
	prefix := 0
	for index, source := range sources {
		item, exists := items[source.item.ID]
		if !exists {
			for _, later := range sources[index+1:] {
				if repositoryShadowSnapshotHasItem(snapshot, later.item.ID) {
					return fmt.Errorf("repository shadow working set contains out-of-order evidence")
				}
			}
			break
		}
		prefix++
		request := source.item
		request.Scope = snapshot.Scope
		want := workingset.Item{
			ID: request.ID, Ref: request.Ref, Role: request.Role, Retention: request.Retention,
			Priority: request.Priority, State: workingset.ItemResident, ByteCost: request.ByteCost,
			Acquisition: request.Acquisition,
			Memberships: []workingset.Membership{{Scope: request.Scope, Retention: request.Retention}},
			CreatedTick: uint64(index + 1), LastUsedTick: uint64(index + 1),
		}
		if !reflect.DeepEqual(item, want) {
			return fmt.Errorf("repository shadow working-set item %q is stale or mismatched", item.ID)
		}
	}
	if len(snapshot.Items) != prefix || snapshot.Version != uint64(prefix) || snapshot.Clock != uint64(prefix) {
		return fmt.Errorf("repository shadow working set contains unregistered state or lifecycle events")
	}
	return nil
}
