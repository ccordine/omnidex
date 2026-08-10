package cognitionstate

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/contextbuilder"
	"github.com/gryph/omnidex/internal/taskstate"
	"github.com/gryph/omnidex/internal/workingset"
)

const (
	AttentionPlanSchemaV1     = "omnidex.cognition-state-attention-plan.v1"
	DefaultContextSpecName    = "cognition-runtime"
	DefaultContextSpecVersion = "1.0.0"
	MaxContextItems           = 64
	MaxContextBytes           = 128 * 1024
	MaxContextMaterialBytes   = 96 * 1024
	MaxAdvisoryRetains        = 8
)

type EvidenceMaterial struct {
	Ref     cognition.EvidenceRef `json:"ref"`
	Content string                `json:"content"`
}

type ReconciliationInput struct {
	State             ProjectionState
	ObligationGraph   cognition.ObligationGraphSnapshot
	Ledger            taskstate.MaterializedState
	WorkingSet        workingset.Snapshot
	Evidence          []EvidenceMaterial
	RequiredAttention []cognition.AttentionRequest
	Attention         []cognition.AttentionRequest
	CapacityRejected  []cognition.EvidenceRef
}

type AdvisoryDisposition string

const (
	AdvisoryAccepted            AdvisoryDisposition = "accepted"
	AdvisoryRejectedProtected   AdvisoryDisposition = "rejected_protected"
	AdvisoryRejectedCapacity    AdvisoryDisposition = "rejected_capacity"
	AdvisoryRejectedUnavailable AdvisoryDisposition = "rejected_unavailable"
)

type AdvisoryOutcome struct {
	Request     cognition.AttentionRequest
	Disposition AdvisoryDisposition
	Reason      string
}

type AttentionPlanDescriptor struct {
	Schema       string
	ID           string
	SHA256       string
	SourceSHA256 string
	CommandCount int
}

type WorkingSetMutation struct {
	kind       workingset.CommandKind
	descriptor workingset.CommandDescriptor
	acquire    *workingset.AcquireCommand
	reacquire  *workingset.ReacquireCommand
	retain     *workingset.RetainCommand
	release    *workingset.ReleaseCommand
	closeScope *workingset.CloseScopeCommand
}

func (mutation WorkingSetMutation) Command() workingset.Command {
	switch mutation.kind {
	case workingset.CommandAcquire:
		if mutation.acquire == nil {
			return nil
		}
		value := *mutation.acquire
		return &value
	case workingset.CommandReacquire:
		if mutation.reacquire == nil {
			return nil
		}
		value := *mutation.reacquire
		return &value
	case workingset.CommandRetain:
		if mutation.retain == nil {
			return nil
		}
		value := *mutation.retain
		return &value
	case workingset.CommandRelease:
		if mutation.release == nil {
			return nil
		}
		value := *mutation.release
		return &value
	case workingset.CommandCloseScope:
		if mutation.closeScope == nil {
			return nil
		}
		value := *mutation.closeScope
		return &value
	default:
		return nil
	}
}

func (mutation WorkingSetMutation) Descriptor() workingset.CommandDescriptor {
	descriptor := mutation.descriptor
	descriptor.Raw = append([]byte(nil), descriptor.Raw...)
	return descriptor
}

type ReconciliationPlan struct {
	descriptor AttentionPlanDescriptor
	commands   []WorkingSetMutation
	spec       contextbuilder.ContextSpec
	materials  []contextbuilder.Material
	outcomes   []AdvisoryOutcome
}

func (plan ReconciliationPlan) Descriptor() AttentionPlanDescriptor { return plan.descriptor }

func (plan ReconciliationPlan) Commands() []WorkingSetMutation {
	return append([]WorkingSetMutation(nil), plan.commands...)
}

func (plan ReconciliationPlan) ContextSpec() contextbuilder.ContextSpec {
	spec := plan.spec
	spec.Required = append([]contextbuilder.Selector(nil), spec.Required...)
	spec.Optional = append([]contextbuilder.Selector(nil), spec.Optional...)
	spec.AllowedAuthorities = append([]taskstate.Authority(nil), spec.AllowedAuthorities...)
	return spec
}

func (plan ReconciliationPlan) Materials() []contextbuilder.Material {
	return append([]contextbuilder.Material(nil), plan.materials...)
}

func (plan ReconciliationPlan) AdvisoryOutcomes() []AdvisoryOutcome {
	return append([]AdvisoryOutcome(nil), plan.outcomes...)
}

func (plan ReconciliationPlan) Validate() error {
	if plan.descriptor.Schema != AttentionPlanSchemaV1 ||
		!strings.HasPrefix(plan.descriptor.ID, "cognition_attention_") ||
		plan.descriptor.ID != "cognition_attention_"+plan.descriptor.SHA256 ||
		!validMappingDigest(plan.descriptor.SHA256) || !validMappingDigest(plan.descriptor.SourceSHA256) ||
		plan.descriptor.CommandCount != len(plan.commands) {
		return fmt.Errorf("%w: attention plan identity is invalid", ErrInvalidReconciliation)
	}
	for index, mutation := range plan.commands {
		command := mutation.Command()
		if command == nil {
			return fmt.Errorf("%w: attention command %d is unavailable", ErrInvalidReconciliation, index)
		}
		descriptor, err := workingset.DescribeCommand(command)
		if err != nil || descriptor.ID != mutation.descriptor.ID ||
			descriptor.SHA256 != mutation.descriptor.SHA256 || descriptor.Kind != mutation.kind {
			return fmt.Errorf("%w: attention command %d identity is invalid", ErrInvalidReconciliation, index)
		}
	}
	expected, err := attentionPlanDescriptor(plan.descriptor.SourceSHA256, plan.commands, plan.spec)
	if err != nil || expected != plan.descriptor {
		return fmt.Errorf("%w: attention plan hash does not bind the exact commands", ErrInvalidReconciliation)
	}
	return nil
}
