package assemblyline

func repositorySemanticUncertaintyContract(
	kind WorkKind,
) (SemanticUncertaintyContract, bool) {
	var contract SemanticUncertaintyContract
	switch kind {
	case WorkContextRelevanceRelation:
		contract = semanticUncertaintyContract(kind,
			"Does this exact code-known context candidate directly contribute context needed for the exact instruction?",
			"Candidate availability is mechanical but semantic necessity for variable natural-language intent is not.",
			"The exact instruction and one bounded candidate content; code-owned identity, scope, provenance, and queue state remain hidden.",
			"One registered candidate-to-instruction relevance relation.",
			"DecodeContextRelevanceRelationResult binds the relation to the exact authority before code retains or discards only that candidate and advances its queue.")
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
	case WorkRoleplayGroundedResponseParagraphInventory:
		contract = semanticUncertaintyContract(kind,
			"What bounded source-ordered candidate paragraphs could answer the exact real-world question in character from supplied evidence?",
			"Code cannot mechanically compose faithful narrative language or identify every semantically responsive formulation.",
			"The exact question, roleplay identity, compact fictional context, and bounded real-world evidence text.",
			"One bounded positive raw candidate-paragraph inventory.",
			"DecodeRoleplayGroundedParagraphInventory validates the inventory before code sieves each candidate independently.")
	case WorkRoleplayGroundedResponseEvidenceRelation:
		contract = semanticUncertaintyContract(kind,
			"Does one real-world evidence capsule support a factual claim in the focused candidate paragraph?",
			"Claim-level semantic support cannot be proven by lexical overlap.",
			"Only the focused candidate paragraph and one bounded evidence-capsule text; question, character identity, and continuity remain absent.",
			"One registered roleplay paragraph-support relation.",
			"DecodeRoleplayGroundedResponseEvidenceRelationLeaf validates the relation before code attaches that evidence identity to the already authorized candidate.")
	case WorkGroundedParagraphSupport:
		contract = semanticUncertaintyContract(kind,
			"Is every factual claim in this exact paragraph within the selected claim domain supported by the supplied evidence?",
			"Text structure and source identifiers cannot establish complete semantic support for a paragraph's factual claims.",
			"Only one paragraph, bounded evidence text, and the code-selected factual or real-world factual claim scope; question, continuity, and voice are absent.",
			"One opaque letter representing fully supported or not fully supported claims.",
			"DecodeGroundedParagraphSupport maps the letter to a semantic relation; code discards an unsupported candidate or independently binds its source citations.")
	case WorkRoleplayCanonFactInventory:
		contract = semanticUncertaintyContract(kind,
			"What bounded source-ordered durable fictional facts does the exact current contribution directly establish?",
			"Attribution and durable narrative meaning require semantic interpretation beyond structural validation.",
			"The exact attributed contribution, reference context, and typed antecedent when present.",
			"One bounded raw inventory of ordinary candidate-fact lines, or the exact absence value when no fact is established.",
			"DecodeRoleplayCanonFactInventory parses and counts the candidates; code ends an empty queue or sieves each candidate independently.")
	case WorkRoleplayCanonFactCandidateAuthorization:
		contract = semanticUncertaintyContract(kind,
			"Is this exact candidate fact directly established by the exact current contribution?",
			"Direct semantic entailment and narrative attribution cannot be proven from text shape.",
			"The exact candidate, attributed contribution, reference context, and typed antecedent when present.",
			"One registered candidate-authorization relation.",
			"DecodeRoleplayCanonFactCandidateAuthorization validates the relation before code retains or discards only that candidate.")
	case WorkRoleplayCanonFactCandidateRelation:
		contract = semanticUncertaintyContract(kind,
			"Do this candidate fact and one accepted fact express the same durable fictional assertion?",
			"Paraphrased semantic identity cannot be computed from byte equality.",
			"Exactly one candidate fact and one already accepted fact.",
			"One registered pairwise fact relation.",
			"DecodeRoleplayCanonFactCandidateRelation validates the relation before code retains or discards only the candidate.")
	case WorkRoleplayOngoingActionRelation:
		contract = semanticUncertaintyContract(kind,
			"Is no action, the same action, or a different action underway for the named character after the exact contribution?",
			"Completion or continuation of a described action is a narrative semantic relation not encoded structurally.",
			"The named character, contribution source, exact contribution, and previous ongoing-action state.",
			"One opaque choice identifying the bounded action-state relation.",
			"DecodeRoleplayOngoingActionRelation maps the opaque choice to code-owned state; code then clears, retains, or requests one new action value.")
	case WorkRoleplayOngoingActionValue:
		contract = semanticUncertaintyContract(kind,
			"What one action newly remains underway for the named character after the exact contribution?",
			"The current action's concise natural-language meaning cannot be generated mechanically.",
			"The named character, contribution source, exact contribution, and code-owned fact that a new ongoing action was selected.",
			"One ordinary plain-text ongoing-action value.",
			"DecodeRoleplayOngoingActionValue validates the text before code binds it to the already selected replacement branch.")
	case WorkGroundedAnswerParagraphInventory:
		contract = semanticUncertaintyContract(kind,
			"What bounded candidate paragraphs could directly answer the exact requirement using only the selected evidence capsules?",
			"Deterministic evidence retrieval cannot compose candidate natural-language answer paragraphs.",
			"The exact requirement, compact objective context, and selected bounded evidence text.",
			"One bounded raw inventory of candidate paragraph text leaves.",
			"DecodeGroundedAnswerParagraphInventory validates the untrusted candidates before code queues exact-unique paragraphs.")
	case WorkGroundedAnswerParagraphEvidenceRelation:
		contract = semanticUncertaintyContract(kind,
			"Does one evidence capsule support a factual claim in the exact candidate paragraph?",
			"Factual claim support cannot be determined exactly from token overlap or evidence identity.",
			"The exact candidate paragraph and one bounded evidence-capsule text.",
			"One registered paragraph-support relation.",
			"DecodeGroundedAnswerParagraphEvidenceRelationDecision validates the relation before code attaches that evidence identity to the already authorized candidate.")
	case WorkGroundedParagraphRelevance:
		contract = semanticUncertaintyContract(kind,
			"Does this exact candidate paragraph address the meaning of the user's question?",
			"Paragraph structure and literal overlap cannot establish semantic relevance to a natural-language question.",
			"Only the exact question, compact context needed to resolve its meaning, and one candidate paragraph; evidence and character-style instructions are absent.",
			"One opaque letter representing relevant or not relevant to the question.",
			"DecodeGroundedParagraphRelevance maps the letter to a semantic relation; code discards an unrelated candidate before asking any factual-support question.")
	default:
		return SemanticUncertaintyContract{}, false
	}
	return contract, true
}
