package semanticreview

import (
	"context"

	"github.com/gryph/omnidex/internal/cognitionreference"
)

type ArtifactID string
type ReviewSpecificationID string
type FindingCode string
type FindingID string
type CorrectionObjectiveKind string
type VerificationReceiptID string

type ObjectiveStatus string

const (
	ObjectivePending      ObjectiveStatus = "pending"
	ObjectiveComplete     ObjectiveStatus = "complete"
	ObjectiveFailed       ObjectiveStatus = "failed"
	ObjectiveBoundBlocked ObjectiveStatus = "bound_blocked"
)

type AcceptancePredicate string

const (
	AcceptanceCurrentArtifactVerified AcceptancePredicate = "current_artifact_verified"
	AcceptanceNoOpenSemanticFinding   AcceptancePredicate = "no_open_semantic_finding"
)

type ReviewAcceptancePredicate string

const AcceptanceReviewFindingResolved ReviewAcceptancePredicate = "review_finding_resolved"

type CorrectionAcceptancePredicate string

const AcceptanceCorrectionArtifactVerified CorrectionAcceptancePredicate = "correction_artifact_verified"

type Objective struct {
	ID         cognitionreference.ObjectiveID
	Acceptance []AcceptancePredicate
	Status     ObjectiveStatus
}

type ArtifactValue struct {
	Content []byte
}

type Artifact struct {
	ID              ArtifactID
	RootObjectiveID cognitionreference.ObjectiveID
	ParentID        ArtifactID
	Revision        uint32
	SHA256          string
	Content         []byte
}

type EvidenceKind string

const (
	EvidenceFixed           EvidenceKind = "fixed"
	EvidenceCurrentArtifact EvidenceKind = "current_artifact"
)

type EvidenceDefinition struct {
	ID      cognitionreference.EvidenceID
	Kind    EvidenceKind
	Content string
}

type FindingKind string

const (
	FindingSemanticIssue FindingKind = "semantic_issue"
	FindingNone          FindingKind = "none"
)

const FindingCodeNone FindingCode = "none"

type FindingDefinition struct {
	CandidateID cognitionreference.CandidateID
	FindingCode FindingCode
	Kind        FindingKind
	Summary     string
	EvidenceIDs []cognitionreference.EvidenceID
}

type ReviewSpecification struct {
	ID          ReviewSpecificationID
	ObjectiveID cognitionreference.ObjectiveID
	Question    string
	Evidence    []EvidenceDefinition
	Candidates  []FindingDefinition
}

type ReviewObjective struct {
	ID              cognitionreference.ObjectiveID
	RootObjectiveID cognitionreference.ObjectiveID
	ParentID        cognitionreference.ObjectiveID
	DependsOn       []cognitionreference.ObjectiveID
	Round           int
	ArtifactID      ArtifactID
	ArtifactSHA256  string
	GapID           cognitionreference.GapID
	Acceptance      []ReviewAcceptancePredicate
	Status          ObjectiveStatus
}

type ReviewFinding struct {
	ID                FindingID
	RootObjectiveID   cognitionreference.ObjectiveID
	ReviewObjectiveID cognitionreference.ObjectiveID
	GapID             cognitionreference.GapID
	ArtifactID        ArtifactID
	ArtifactSHA256    string
	CandidateID       cognitionreference.CandidateID
	Kind              FindingKind
	FindingCode       FindingCode
	EvidenceIDs       []cognitionreference.EvidenceID
}

type CorrectionRule struct {
	FindingCode   FindingCode
	ObjectiveKind CorrectionObjectiveKind
	Acceptance    []CorrectionAcceptancePredicate
}

type CorrectionObjective struct {
	ID               cognitionreference.ObjectiveID
	RootObjectiveID  cognitionreference.ObjectiveID
	ParentID         cognitionreference.ObjectiveID
	DependsOn        []cognitionreference.ObjectiveID
	Round            int
	Finding          ReviewFinding
	InputArtifactID  ArtifactID
	InputSHA256      string
	ObjectiveKind    CorrectionObjectiveKind
	Acceptance       []CorrectionAcceptancePredicate
	OutputArtifactID ArtifactID
	Status           ObjectiveStatus
}

type CorrectionExecutor interface {
	Execute(context.Context, CorrectionObjective, Artifact) (ArtifactValue, error)
}

type CorrectionExecutorRegistration struct {
	ObjectiveKind CorrectionObjectiveKind
	Executor      CorrectionExecutor
}

type VerificationKind string

const (
	VerificationCurrentArtifact    VerificationKind = "current_artifact"
	VerificationCorrectionArtifact VerificationKind = "correction_artifact"
)

type VerificationInput struct {
	Kind                 VerificationKind
	RootObjectiveID      cognitionreference.ObjectiveID
	Artifact             Artifact
	Correction           *CorrectionObjective
	ArtifactAcceptance   []AcceptancePredicate
	CorrectionAcceptance []CorrectionAcceptancePredicate
}

type Verifier interface {
	Verify(context.Context, VerificationInput) error
}

type VerificationReceipt struct {
	ID                    VerificationReceiptID
	Kind                  VerificationKind
	RootObjectiveID       cognitionreference.ObjectiveID
	ArtifactID            ArtifactID
	ArtifactSHA256        string
	ArtifactRevision      uint32
	CorrectionObjectiveID cognitionreference.ObjectiveID
	ArtifactAcceptance    []AcceptancePredicate
	CorrectionAcceptance  []CorrectionAcceptancePredicate
}

type Limits struct {
	MaxReviewRounds int
}

type Result struct {
	EvidenceClass        EvidenceClass
	Objective            Objective
	InitialArtifact      Artifact
	CurrentArtifact      Artifact
	Reviews              []ReviewObjective
	Findings             []ReviewFinding
	Corrections          []CorrectionObjective
	VerificationReceipts []VerificationReceipt
	StationCalls         int
	CorrectionCalls      int
	VerificationCalls    int
	Complete             bool
}
