package qwenselector_test

import "github.com/gryph/omnidex/internal/cognitionreference"

type CandidateID = cognitionreference.CandidateID
type EvidenceID = cognitionreference.EvidenceID
type GapID = cognitionreference.GapID
type SemanticCandidate = cognitionreference.SemanticCandidate
type SemanticEvidence = cognitionreference.SemanticEvidence
type SemanticGap = cognitionreference.SemanticGap
type Selector = cognitionreference.Selector

const GapCandidateSelection = cognitionreference.GapCandidateSelection

var (
	ErrInvalidSelection = cognitionreference.ErrInvalidSelection
	SelectCandidate     = cognitionreference.SelectCandidate
)
