package repositoryobjective

import (
	"errors"

	"github.com/gryph/omnidex/internal/cognitionreference"
)

type SemanticGap = cognitionreference.SemanticGap
type CandidateID = cognitionreference.CandidateID

var (
	ErrInvalidObjective    = errors.New("invalid read-only repository objective")
	ErrRepositoryAuthority = errors.New("invalid read-only repository authority")
	ErrSubjectAbsent       = errors.New("repository objective subject is absent")
	ErrSubjectAmbiguous    = errors.New("repository objective subject is ambiguous")
	ErrSemanticResolution  = errors.New("repository objective semantic resolution failed")
	ErrRelationBound       = errors.New("repository objective relation bound exceeded")
	ErrObjectiveIncomplete = errors.New("read-only repository objective is incomplete")
)

type LookupKind string

const (
	LookupQualifiedName LookupKind = "qualified_name"
	LookupName          LookupKind = "name"
)

type SubjectLookup struct {
	Kind  LookupKind
	Value string
}

type AcceptancePredicate string

const (
	AcceptanceSubjectResolved      AcceptancePredicate = "subject_resolved"
	AcceptanceDeclarationObserved  AcceptancePredicate = "declaration_observed"
	AcceptanceDirectRelationsKnown AcceptancePredicate = "direct_relations_known"
	AcceptanceApplicableTestsKnown AcceptancePredicate = "applicable_tests_known"
)

type Objective struct {
	ID         cognitionreference.ObjectiveID
	Root       string
	Question   string
	Subject    SubjectLookup
	Acceptance []AcceptancePredicate
}

type SubjectAuthority string

const (
	SubjectAuthorityDeterministic SubjectAuthority = "deterministic"
	SubjectAuthoritySemantic      SubjectAuthority = "semantic"
)

type SymbolEvidence struct {
	SymbolID          string
	QualifiedName     string
	Kind              string
	Signature         string
	SourceSHA256      string
	DeclarationSHA256 string
}

type SubjectFact struct {
	ObjectiveID cognitionreference.ObjectiveID
	Acceptance  []AcceptancePredicate
	AnalysisID  string
	Authority   SubjectAuthority
	GapID       cognitionreference.GapID
	CandidateID cognitionreference.CandidateID
	Symbol      SymbolEvidence
}

type Step string

const (
	StepSnapshotCaptured    Step = "snapshot_captured"
	StepSnapshotProjected   Step = "snapshot_projected"
	StepRepositoryAnalyzed  Step = "repository_analyzed"
	StepCandidatesInspected Step = "candidates_inspected"
	StepSubjectResolved     Step = "subject_resolved"
	StepDeclarationObserved Step = "declaration_observed"
	StepRelationsTraversed  Step = "relations_traversed"
	StepTestsTraversed      Step = "tests_traversed"
	StepProjectionVerified  Step = "projection_verified"
	StepAuthorityReconciled Step = "authority_reconciled"
	StepObjectiveCompleted  Step = "objective_completed"
)

type Result struct {
	ObjectiveID      cognitionreference.ObjectiveID
	Acceptance       []AcceptancePredicate
	Satisfied        []AcceptancePredicate
	Steps            []Step
	BeforeSnapshotID string
	AfterSnapshotID  string
	AnalysisID       string
	Subject          SubjectFact
	DirectCalls      []SymbolEvidence
	DirectCallers    []SymbolEvidence
	ApplicableTests  []SymbolEvidence
	SelectorCalls    int
	InferenceCalls   int
	Complete         bool
}
