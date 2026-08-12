package cognitiongauntlet

import (
	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/cognitionreplay"
	"github.com/gryph/omnidex/internal/contextbuilder"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/taskstate"
	"github.com/gryph/omnidex/internal/workingset"
)

const (
	AblationEvidenceSchemaV1          = "omnidex.ablation-evidence.v1"
	AblationEvidenceAuthoritySchemaV1 = "omnidex.ablation-evidence-authority.v1"
)

type AblationEvidenceAuthority struct {
	Schema string `json:"schema"`
	ID     string `json:"id"`
	SHA256 string `json:"sha256"`
	Bytes  int    `json:"bytes"`
	File   string `json:"file"`
}

type ablationEvidenceRoot struct {
	Schema                   string                                     `json:"schema"`
	EpisodeID                cognition.EpisodeID                        `json:"episode_id"`
	Variant                  Variant                                    `json:"variant"`
	Class                    AblationReplayClass                        `json:"class"`
	PublicRunAuthority       PublicRunAuthority                         `json:"public_run_authority"`
	PublicRunAuthoritySHA256 string                                     `json:"public_run_authority_sha256"`
	Actor                    cognition.AttemptRef                       `json:"actor"`
	Goal                     cognition.GoalExpression                   `json:"goal"`
	Completion               cognition.CompletionAuthority              `json:"completion"`
	Obligation               cognition.Obligation                       `json:"obligation"`
	BrainBootstrap           RuntimeBrainBootstrapEvidenceAuthority     `json:"brain_bootstrap"`
	ProviderActivation       RuntimeProviderActivationEvidenceAuthority `json:"provider_activation"`
	WorldCatalog             cognition.ActionCatalog                    `json:"world_catalog"`
	Transitions              []cognition.Transition                     `json:"transitions"`
	Calls                    []ablationCallEvidence                     `json:"calls"`
	Actions                  []ablationActionEvidence                   `json:"actions"`
	NoActions                []ablationNoActionEvidence                 `json:"no_actions"`
	Ledger                   *ablationLedgerEvidence                    `json:"ledger,omitempty"`
	WorkingSet               *ablationWorkingSetEvidence                `json:"working_set,omitempty"`
	ContextBudget            *ablationContextBudgetEvidence             `json:"context_budget,omitempty"`
	TerminalCause            ablationTerminalCause                      `json:"terminal_cause"`
	Terminal                 ablationPendingTerminal                    `json:"terminal"`
	ChunkedBlobs             []cognitionreplay.ChunkedBlobBinding       `json:"chunked_blobs"`
}

type ablationContextBudgetEvidence struct {
	Projection      ablationProjectionEvidence `json:"projection"`
	Snapshot        semanticRuntimeSnapshot    `json:"runtime_snapshot"`
	ModelInputBytes int                        `json:"model_input_bytes"`
}

type ablationTerminalCause struct {
	Kind            ablationTerminalCauseKind `json:"kind"`
	CallOrdinal     uint32                    `json:"call_ordinal,omitempty"`
	ActionID        cognition.ActionID        `json:"action_id,omitempty"`
	Reason          string                    `json:"reason,omitempty"`
	CompletedCalls  int                       `json:"completed_calls"`
	CompletedCycles int                       `json:"completed_cycles"`
}

type ablationTerminalCauseKind string

const (
	ablationTerminalWorld          ablationTerminalCauseKind = "world_terminal"
	ablationTerminalActionFailure  ablationTerminalCauseKind = "dispatched_action_failure"
	ablationTerminalPolicyDecision ablationTerminalCauseKind = "policy_no_decision"
	ablationTerminalNoDispatch     ablationTerminalCauseKind = "accepted_not_dispatched"
	ablationTerminalPreCallBudget  ablationTerminalCauseKind = "pre_call_budget"
	ablationTerminalContextBudget  ablationTerminalCauseKind = "context_projection_budget"
	ablationTerminalCycleBudget    ablationTerminalCauseKind = "cycle_budget"
)

type AblationReplayClass string

const (
	AblationReplaySerious       AblationReplayClass = "serious"
	AblationReplayBenchmarkOnly AblationReplayClass = "benchmark_only"
	AblationReplayContaminated  AblationReplayClass = "contaminated"
)

type ablationCallEvidence struct {
	Ordinal    uint32                      `json:"ordinal"`
	Projection ablationProjectionEvidence  `json:"projection"`
	Snapshot   semanticRuntimeSnapshot     `json:"runtime_snapshot"`
	Attempt    cognitionpolicy.CallAttempt `json:"attempt"`
	Result     cognitionpolicy.CallResult  `json:"result"`
	Evidence   ablationPolicyEvidence      `json:"evidence"`
}

type ablationActionEvidence struct {
	Cycle  uint32      `json:"cycle"`
	CallID string      `json:"call_id"`
	Trace  ActionTrace `json:"trace"`
}

type ablationNoActionEvidence struct {
	Cycle  uint32                      `json:"cycle"`
	CallID string                      `json:"call_id"`
	Kind   ablationNoActionDisposition `json:"kind"`
	Reason string                      `json:"reason"`
}

type ablationNoActionDisposition string

const (
	ablationPolicyNoDecision ablationNoActionDisposition = "policy_no_decision"
	ablationAcceptedNoAction ablationNoActionDisposition = "accepted_decision_not_dispatched"
)

type ablationProjectionEvidence struct {
	Projection contextbuilder.Projection                  `json:"projection"`
	Content    cognitionreplay.ProjectionContentAuthority `json:"content"`
}

type ablationPolicyEvidence struct {
	ModelResponse           *ablationModelResponseEvidence      `json:"model_response,omitempty"`
	ProviderIdentity        *ablationIdentityEvidence           `json:"provider_identity,omitempty"`
	ProviderResponseCapture *ablationProviderCaptureEvidence    `json:"provider_response_capture,omitempty"`
	ProviderGeneration      *ablationProviderGenerationEvidence `json:"provider_generation,omitempty"`
}

type ablationModelResponseEvidence struct {
	Ref     cognitionpolicy.ModelResponseEvidenceRef   `json:"ref"`
	CallID  string                                     `json:"call_id"`
	Content cognitionreplay.ProjectionContentAuthority `json:"content"`
}

type ablationProviderCaptureEvidence struct {
	Ref     cognitionpolicy.ProviderResponseCaptureEvidenceRef `json:"ref"`
	CallID  string                                             `json:"call_id"`
	Content cognitionreplay.ProjectionContentAuthority         `json:"content"`
}

type ablationProviderGenerationEvidence struct {
	Ref     cognitionpolicy.ProviderGenerationEvidenceRef `json:"ref"`
	CallID  string                                        `json:"call_id"`
	Content cognitionreplay.ProjectionContentAuthority    `json:"content"`
}

type ablationIdentityEvidence struct {
	Schema     string                              `json:"schema"`
	Ref        llm.ProviderIdentityEvidenceRef     `json:"ref"`
	Operations []ablationIdentityOperationEvidence `json:"operations"`
}

type ablationIdentityOperationEvidence struct {
	Operation          llm.ProviderIdentityOperation              `json:"operation"`
	Method             string                                     `json:"method"`
	Endpoint           string                                     `json:"endpoint"`
	RequestDisposition llm.ProviderRequestDisposition             `json:"request_disposition"`
	RequestSHA256      string                                     `json:"request_sha256"`
	RequestBytes       int                                        `json:"request_bytes"`
	RequestContent     cognitionreplay.ProjectionContentAuthority `json:"request_content"`
	HTTPStatus         int                                        `json:"http_status"`
	Disposition        llm.ProviderIdentityOperationDisposition   `json:"disposition"`
	ResponseComplete   bool                                       `json:"response_complete"`
	ContentEncoding    llm.ProviderContentEncodingEvidence        `json:"content_encoding"`
	ResponseSHA256     string                                     `json:"response_sha256"`
	ResponseBytes      int                                        `json:"response_bytes"`
	ResponseContent    cognitionreplay.ProjectionContentAuthority `json:"response_content"`
}

type ablationEvidenceArtifact struct {
	Schema string                 `json:"schema"`
	Root   ablationEvidenceRoot   `json:"root"`
	Blobs  []ablationEvidenceBlob `json:"blobs"`
}

type ablationEvidenceBlob struct {
	SHA256    string `json:"sha256"`
	MediaType string `json:"media_type"`
	Data      []byte `json:"data"`
}

type ablationLedgerEvidence struct {
	ID       taskstate.LedgerID          `json:"id"`
	Owner    taskstate.LedgerOwner       `json:"owner"`
	Events   []taskstate.Event           `json:"events"`
	Terminal taskstate.MaterializedState `json:"terminal"`
}

type ablationWorkingSetEvidence struct {
	Initial  workingset.Snapshot `json:"initial"`
	Events   []workingset.Event  `json:"events"`
	Terminal workingset.Snapshot `json:"terminal"`
}
