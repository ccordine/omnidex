package cognition

const (
	MaxIdentityBytes          = 128
	MaxVersionBytes           = 64
	MaxPredicateArgs          = 16
	MaxPredicateArgBytes      = 512
	MaxGoalPredicates         = 64
	MaxObservationBytes       = 64 * 1024
	MaxTransitionObservations = 128
	MaxTransitionEffects      = 64
	MaxEffectBytes            = 4096
	MaxFailureMessageBytes    = 4096
	MaxTransitionCost         = 1_000_000
	MaxActionSchemas          = 128
	MaxActionParameters       = 32
	MaxActionArguments        = 32
	MaxActionValueBytes       = 4096
	MaxEvidenceRefs           = 64
	MaxExpectedEffectBytes    = 2048
	MaxLedgerProposals        = 32
	MaxAttentionRequests      = 32
	MaxProposalBytes          = 4096
	MaxAttentionReasonBytes   = 1024
	MaxEpistemicRefURIBytes   = 512
	MaxPublicOutcomeBytes     = 4096
	MaxDecisionBytes          = 256 * 1024
	MaxObligations            = 128
	MaxObligationDepth        = 16
	MaxObligationDependencies = 32
	MaxCompletionPredicates   = 256
	MaxPolicyCallsPerEpisode  = 1024
	MaxPolicyInputBytes       = 512 * 1024
	MaxPolicyInputTokens      = 128 * 1024
	MaxPolicyOutputBytes      = 64 * 1024
	MaxPolicyOutputTokens     = 16 * 1024
)
