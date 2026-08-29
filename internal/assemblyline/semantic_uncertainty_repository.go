package assemblyline

func repositorySemanticUncertaintyContract(
	kind WorkKind,
) (SemanticUncertaintyContract, bool) {
	var contract SemanticUncertaintyContract
	switch kind {
	case WorkRepositoryRequirementCoverage:
		contract = semanticUncertaintyContract(kind,
			"Does one explicit workspace-change requirement remain uncovered by the retained requirements?",
			"Semantic equivalence between free-form change requirements cannot be computed by structural comparison.",
			"The immutable repository request, established context, and retained change-requirement statements.",
			"One registered repository-requirement coverage relation.",
			"DecodeRepositoryRequirementCoverageLeaf validates the relation before code continues or closes the bounded fixed point.")
	case WorkRepositoryRequirement:
		contract = semanticUncertaintyContract(kind,
			"What is the earliest explicit workspace-change requirement not covered by the retained requirements?",
			"Faithful semantic extraction from unconstrained human phrasing cannot be performed by repository parsing.",
			"The immutable repository request, established context, and retained change-requirement statements.",
			"One explicit workspace-change requirement leaf.",
			"DecodeRepositoryRequirementLeaf validates the leaf before code appends it to the bounded retained set.")
	case WorkRepositoryEvidenceRelevanceLeaf:
		contract = semanticUncertaintyContract(kind,
			"Which single remaining evidence candidate is most directly relevant to the exact repository requirement?",
			"Repository indexing establishes available evidence but not its semantic relevance to free-form intent.",
			"The exact requirement, bounded semantic evidence candidates, retained evidence IDs, and selection limit.",
			"One opaque repository-evidence candidate ID.",
			"DecodeRepositoryEvidenceRelevanceLeaf validates the ID before code retains the corresponding evidence capsule.")
	case WorkRepositoryChangeOwner:
		contract = semanticUncertaintyContract(kind,
			"Which eligible symbol directly owns the existing declaration that must change for the focused requirement?",
			"Parser-proven symbols expose structure but cannot assign requirement meaning to one declaration owner.",
			"The focused requirement plus bounded symbol signatures, relations, and code-owned eligibility.",
			"One opaque repository symbol ID.",
			"DecodeRepositoryChangeOwnerLeaf validates the ID before code binds the focused change responsibility.")
	case WorkContextRelevanceSelection:
		contract = semanticUncertaintyContract(kind,
			"Which not-yet-retained context candidate is most necessary for the exact instruction?",
			"Candidate availability is mechanical but semantic necessity for variable natural-language intent is not.",
			"The exact instruction, bounded candidate contents, retained candidate IDs, and selection limit.",
			"One opaque context-candidate ID.",
			"DecodeContextRelevanceSelectionDecision validates the ID before code retains the corresponding authority.")
	case WorkContextMinification:
		contract = semanticUncertaintyContract(kind,
			"What minimal selected context is necessary to interpret the exact instruction?",
			"Mechanical truncation cannot preserve only the semantically necessary referents and relationships.",
			"The exact instruction and the code-selected bounded authority contents.",
			"One minimal context text leaf.",
			"DecodeContextMinificationDecision validates the leaf before code binds it as compact objective context.")
	case WorkConversationObjectiveKind:
		contract = semanticUncertaintyContract(kind,
			"Which registered objective kind exactly describes the user instruction?",
			"Free-form language cannot be routed faithfully by keywords or structural parsing.",
			"The exact instruction, compact objective context, explicit transport facts, and registered objective kinds.",
			"One registered conversation-objective kind.",
			"DecodeConversationObjectiveKindDecision validates the kind before code enters its authoritative deterministic branch.")
	case WorkConversationResponse:
		contract = semanticUncertaintyContract(kind,
			"What bounded response text directly satisfies the exact conversational instruction?",
			"The requested natural-language response content has no mechanically derivable exact byte sequence.",
			"The exact instruction, compact objective context, and typed roleplay identity when present.",
			"One bounded conversation-response text leaf.",
			"DecodeConversationResponseDecision validates the text before code records the response result.")
	case WorkRoleplayGroundedResponseText:
		contract = semanticUncertaintyContract(kind,
			"What concise in-character answer resolves the exact real-world question from supplied evidence?",
			"Evidence validation cannot mechanically compose faithful narrative language or semantic factual claims.",
			"The exact question, roleplay identity, compact fictional context, and bounded real-world evidence text.",
			"One grounded roleplay-response text leaf.",
			"DecodeRoleplayGroundedResponseTextLeaf validates the text before code splits it into bounded paragraphs.")
	case WorkRoleplayGroundedResponseEvidenceRelation:
		contract = semanticUncertaintyContract(kind,
			"Does one real-world evidence capsule support a factual claim in the focused answer paragraph?",
			"Claim-level semantic support cannot be proven by lexical overlap.",
			"The exact question, focused paragraph text, and one bounded evidence-capsule text.",
			"One registered roleplay paragraph-support relation.",
			"DecodeRoleplayGroundedResponseEvidenceRelationLeaf validates the relation before code attaches evidence identity.")
	case WorkRoleplayCanonFactCoverage:
		contract = semanticUncertaintyContract(kind,
			"Does one durable fictional fact remain uncovered in the exact current contribution?",
			"Durable fictional meaning cannot be separated from questions or decorative prose by syntax alone.",
			"The exact attributed contribution, reference context, and retained current-contribution facts.",
			"One registered canon-fact coverage relation.",
			"DecodeRoleplayCanonFactCoverageLeaf validates the relation before code continues or closes the bounded fact set.")
	case WorkRoleplayCanonFact:
		contract = semanticUncertaintyContract(kind,
			"What single durable fictional fact remains uncovered in the exact current contribution?",
			"Attribution and durable narrative meaning require semantic interpretation beyond structural validation.",
			"The exact attributed contribution, reference context, and retained current-contribution facts.",
			"One durable fictional-fact text leaf.",
			"DecodeRoleplayCanonFactLeaf validates the fact before code appends it to the bounded canon set.")
	case WorkRoleplayOngoingAction:
		contract = semanticUncertaintyContract(kind,
			"What single action remains underway for the named character after the exact contribution?",
			"Completion or continuation of a described action is a narrative semantic relation not encoded structurally.",
			"The named character, contribution source, exact contribution, and previous ongoing-action state.",
			"One optional ongoing-action text leaf.",
			"DecodeRoleplayOngoingActionDecision validates the leaf before code replaces the character's persisted ongoing-action state.")
	case WorkGroundedAnswerText:
		contract = semanticUncertaintyContract(kind,
			"What answer text satisfies the exact requirement using only the selected evidence capsules?",
			"Deterministic evidence retrieval cannot compose the semantically complete natural-language answer.",
			"The exact requirement, compact objective context, and selected bounded evidence text.",
			"One grounded answer-text leaf.",
			"DecodeGroundedAnswerTextDecision validates the text before code evaluates each evidence-support relation.")
	case WorkGroundedAnswerEvidenceRelation:
		contract = semanticUncertaintyContract(kind,
			"Does one evidence capsule support a factual claim in the exact answer?",
			"Factual claim support cannot be determined exactly from token overlap or evidence identity.",
			"The exact requirement, compact context, answer text, and one bounded evidence-capsule text.",
			"One registered grounded-answer support relation.",
			"DecodeGroundedAnswerEvidenceRelationDecision validates the relation before code attaches evidence identity.")
	default:
		return SemanticUncertaintyContract{}, false
	}
	return contract, true
}
