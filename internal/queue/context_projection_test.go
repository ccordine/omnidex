package queue

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/contextbuilder"
	"github.com/gryph/omnidex/internal/taskstate"
	"github.com/gryph/omnidex/internal/workingset"
)

func TestContextProjectionStoreRejectsInvalidAuthorityBeforePostgreSQL(t *testing.T) {
	t.Parallel()
	projection := testQueueContextProjection(t, strings.Repeat("a", 64))
	base := ContextProjectionAuthority{
		JobID: 1, Generation: 1, StepID: 1, WorkKind: "repository_investigation",
		Mode: ContextProjectionModeShadow,
	}
	for name, mutate := range map[string]func(*ContextProjectionAuthority, *contextbuilder.Projection){
		"missing job":        func(a *ContextProjectionAuthority, _ *contextbuilder.Projection) { a.JobID = 0 },
		"missing generation": func(a *ContextProjectionAuthority, _ *contextbuilder.Projection) { a.Generation = 0 },
		"missing step":       func(a *ContextProjectionAuthority, _ *contextbuilder.Projection) { a.StepID = 0 },
		"bad work kind":      func(a *ContextProjectionAuthority, _ *contextbuilder.Projection) { a.WorkKind = " bad kind " },
		"missing mode":       func(a *ContextProjectionAuthority, _ *contextbuilder.Projection) { a.Mode = "" },
		"bad mode":           func(a *ContextProjectionAuthority, _ *contextbuilder.Projection) { a.Mode = "fallback" },
		"applied mode":       func(a *ContextProjectionAuthority, _ *contextbuilder.Projection) { a.Mode = "applied" },
		"bad work id":        func(_ *ContextProjectionAuthority, p *contextbuilder.Projection) { p.WorkID = "work-1" },
	} {
		t.Run(name, func(t *testing.T) {
			authority, candidate := base, projection
			mutate(&authority, &candidate)
			if _, err := (&Repository{}).StoreContextProjection(
				context.Background(), authority, candidate,
			); !errors.Is(err, ErrInvalidContextProjection) {
				t.Fatalf("error=%v, want ErrInvalidContextProjection", err)
			}
		})
	}
}

func TestContextProjectionReadsRequireHardPagination(t *testing.T) {
	t.Parallel()
	repository := &Repository{}
	if _, err := repository.ListContextProjectionSummaries(
		context.Background(), 1, 1, 0, maxContextProjectionPageSize+1,
	); !errors.Is(err, ErrInvalidContextProjection) {
		t.Fatalf("oversized page error=%v", err)
	}
	if _, err := repository.ListContextProjectionSummaries(
		context.Background(), 1, 1, -1, 1,
	); !errors.Is(err, ErrInvalidContextProjection) {
		t.Fatalf("negative cursor error=%v", err)
	}
	if _, err := repository.GetContextProjection(context.Background(), "invalid"); !errors.Is(err, ErrInvalidContextProjection) {
		t.Fatalf("invalid identity error=%v", err)
	}
}

func TestLLMCallEvidenceProjectionIdentityIsValidatedAndHashed(t *testing.T) {
	t.Parallel()
	base := normalizeLLMCallEvidenceRecord(LLMCallEvidenceRecord{
		StepID: 1, Scope: "portable", WorkID: strings.Repeat("a", 64), WorkKind: "test_work",
		RequestedModel: "requested", Model: "effective", Attempt: 1,
		SystemPrompt: "system", UserPrompt: "user", ResponseFormat: "text",
		ContextTokens: 4096, MaxOutputTokens: 512,
		Status: LLMEvidenceSucceeded, Response: "response",
	})
	_, directHash, err := validateAndHashLLMCallEvidenceRecord(base)
	if err != nil {
		t.Fatal(err)
	}
	base.ContextProjectionID = "context_projection_" + strings.Repeat("b", 64)
	_, projectedHash, err := validateAndHashLLMCallEvidenceRecord(base)
	if err != nil {
		t.Fatal(err)
	}
	if directHash == projectedHash {
		t.Fatal("bound and legacy-null calls shared a request identity")
	}
	base.ContextProjectionID = "projection-bad"
	if err := validateLLMCallEvidenceRecord(base); err == nil {
		t.Fatal("accepted malformed context projection identity")
	}
}

func testQueueContextProjection(t *testing.T, workID string) contextbuilder.Projection {
	t.Helper()
	owner := workingset.Owner{
		LedgerID: taskstate.LedgerID("ledger_" + strings.Repeat("a", 64)),
		JobID:    1, Generation: 1,
	}
	set, err := workingset.New(owner, workingset.Budget{MaxItems: 2, MaxBytes: 4096})
	if err != nil {
		t.Fatal(err)
	}
	request := workingset.AcquireRequest{
		ID: "user", Ref: taskstate.Ref{
			URI: "task:job/1/entry/user", Version: "v1",
			Hash: strings.Repeat("c", 64), Relation: taskstate.RefSource,
		},
		Role: workingset.RoleUserAuthority, Retention: workingset.RetentionJob,
		Scope: set.Scope(), Priority: 100, ByteCost: 64,
		Acquisition: workingset.Acquisition{
			Provider: workingset.ProviderTaskState, OperationID: "operation-user",
			Reason: "Direct authority for the current task.",
		},
	}
	item, err := set.Acquire(request)
	if err != nil {
		t.Fatal(err)
	}
	projection, err := contextbuilder.Build(contextbuilder.BuildInput{
		WorkID: workID,
		Spec: contextbuilder.ContextSpec{
			Name: "repository-investigation", Version: "v1",
			ScopeRef: taskstate.Ref{
				URI: "task:job/1/node/task-1", Version: "v1",
				Hash: strings.Repeat("d", 64), Relation: taskstate.RefConcerns,
			},
			Required: []contextbuilder.Selector{{
				ID: "user", Role: workingset.RoleUserAuthority, MinItems: 1, MaxItems: 1,
			}},
			AllowedAuthorities: []taskstate.Authority{taskstate.AuthorityUser},
			MaxItems:           2, MaxBytes: 4096, MaxAcquisitionRounds: 1,
		},
		WorkingSet: set,
		Materials: []contextbuilder.Material{{
			ItemID: item.Item.ID, CurrentRef: item.Item.Ref, Authority: taskstate.AuthorityUser,
			Content: "authoritative user request", ByteCost: len("authoritative user request"),
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return projection
}
