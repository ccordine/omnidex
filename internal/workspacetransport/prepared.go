package workspacetransport

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/gryph/omnidex/internal/workspace"
)

type prepared struct {
	access  *access
	id      uint64
	desired map[string]workspace.DesiredFile
	mu      sync.Mutex
	applied bool
}

func (a *access) Prepare(ctx context.Context, desired []workspace.DesiredFile, expected []workspace.File) (workspace.Prepared, error) {
	desired, expected, err := workspace.ValidateReconciliationFiles(desired, expected)
	if err != nil {
		return nil, err
	}
	reply, err := a.call(ctx, requestData{
		request: request{Kind: opPrepare, DesiredCount: len(desired), ExpectedCount: len(expected)}, desired: desired, expected: expected,
	}, nil)
	if err != nil {
		return nil, err
	}
	states := make(map[string]workspace.DesiredFile, len(desired))
	for _, state := range desired {
		states[state.Path] = state
	}
	return &prepared{access: a, id: reply.ID, desired: states}, nil
}

func (p *prepared) ApplyVerified(ctx context.Context, observer workspace.VerifiedChangeObserver) (workspace.ReconciliationResult, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.applied {
		return workspace.ReconciliationResult{}, fmt.Errorf("workspace preparation was already applied")
	}
	p.applied = true
	result := workspace.ReconciliationResult{}
	seen := make(map[string]bool, len(p.desired))
	reply, err := p.access.call(ctx, requestData{request: request{Kind: opApply, Prepared: p.id}}, func(change workspace.Change) error {
		if err := p.validateChange(change, seen); err != nil {
			return err
		}
		seen[change.Path] = true
		result.Changes = append(result.Changes, change)
		if observer != nil {
			observer(change)
		}
		return nil
	})
	if reply.ID != 0 && reply.AppliedChanges != len(result.Changes) {
		mismatch := fmt.Errorf("workspace publication count differs from its observed changes")
		p.access.remote.stop(mismatch)
		return result, mismatch
	}
	return result, err
}

func (p *prepared) validateChange(change workspace.Change, seen map[string]bool) error {
	if err := (workspace.Entry{Path: change.Path, Kind: workspace.EntryFile}).Validate(change.Path); err != nil {
		return err
	}
	if change.Path == "." {
		return fmt.Errorf("workspace root is not a prepared file")
	}
	state, exists := p.desired[change.Path]
	if seen[change.Path] {
		return fmt.Errorf("workspace change has no unique prepared file")
	}
	if !exists {
		if change.Kind != workspace.ChangeDelete || change.SourcePath != "" {
			return fmt.Errorf("workspace change has no prepared file")
		}
		for _, owner := range p.desired {
			if owner.MoveFrom == change.Path || (owner.Present && !owner.CreateOnly && strings.HasPrefix(owner.Path, change.Path+"/")) {
				return nil
			}
		}
		return fmt.Errorf("workspace deletion has no prepared source or parent")
	}
	switch change.Kind {
	case workspace.ChangeCreate, workspace.ChangeReplace:
		if !state.Present || change.SourcePath != "" || (state.CreateOnly && change.Kind != workspace.ChangeCreate) {
			return fmt.Errorf("workspace write differs from its prepared authority")
		}
	case workspace.ChangeDelete:
		if state.Present || change.SourcePath != "" {
			return fmt.Errorf("workspace deletion differs from its prepared authority")
		}
	case workspace.ChangeMove:
		if !state.Present || state.MoveFrom == "" || change.SourcePath != state.MoveFrom {
			return fmt.Errorf("workspace move differs from its prepared authority")
		}
	default:
		return fmt.Errorf("workspace change has an unregistered kind")
	}
	return nil
}
