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
		contract = semanticUncertaintyContractV2(kind,
			"Does one real-world evidence capsule support a factual claim in the focused candidate paragraph?",
			"Claim-level semantic support cannot be proven by lexical overlap.",
			"The exact question, focused candidate paragraph, and one bounded evidence-capsule text.",
			"One registered roleplay paragraph-support relation.",
			"DecodeRoleplayGroundedResponseEvidenceRelationLeaf validates the relation before code attaches that evidence identity to the already authorized candidate.")
	case WorkRoleplayGroundedResponseParagraphAuthorization:
		contract = semanticUncertaintyContractV2(kind,
			"Is this complete candidate paragraph responsive in character and fully supported by the supplied evidence?",
			"Narrative responsiveness, viewpoint consistency, and complete factual entailment cannot be proven mechanically.",
			"The exact question, roleplay identity, compact fictional context, exact candidate paragraph, and the complete bounded evidence text supplied for the answer.",
			"One registered paragraph-admissibility relation.",
			"DecodeRoleplayGroundedParagraphAuthorizationDecision validates the relation before code discards a negative candidate immediately or performs pairwise evidence attribution for only that positive candidate.")
	case WorkRoleplayCanonFactPresence:
		contract = semanticUncertaintyContract(kind,
			"Does the current contribution directly establish any durable fictional fact?",
			"Durable fictional meaning cannot be determined from contribution shape alone.",
			"The exact attributed contribution, reference context, and typed antecedent when present.",
			"One opaque binary choice identifying presence or absence.",
			"DecodeRoleplayCanonFactPresenceResult validates the relation before code either assembles an empty fact set or opens the positive-only inventory.")
	case WorkRoleplayCanonFactInventory:
		contract = semanticUncertaintyContract(kind,
			"What bounded source-ordered durable fictional facts does the exact current contribution directly establish?",
			"Attribution and durable narrative meaning require semantic interpretation beyond structural validation.",
			"The exact attributed contribution, reference context, and typed antecedent when present.",
			"Between one and the code-owned maximum ordinary candidate-fact lines.",
			"DecodeRoleplayCanonFactInventory validates the inventory before code sieves each candidate independently.")
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
		contract = semanticUncertaintyContractV2(kind,
			"Does one evidence capsule support a factual claim in the exact candidate paragraph?",
			"Factual claim support cannot be determined exactly from token overlap or evidence identity.",
			"The exact candidate paragraph and one bounded evidence-capsule text.",
			"One registered paragraph-support relation.",
			"DecodeGroundedAnswerParagraphEvidenceRelationDecision validates the relation before code attaches that evidence identity to the already authorized candidate.")
	case WorkGroundedAnswerParagraphAuthorization:
		contract = semanticUncertaintyContractV2(kind,
			"Does the complete exact candidate paragraph directly answer the exact requirement with every factual claim supported?",
			"Responsiveness and complete factual entailment cannot be determined exactly from text shape.",
			"The exact requirement, compact context, exact candidate paragraph, and the complete bounded evidence text supplied for the answer.",
			"One registered complete-paragraph authorization relation.",
			"DecodeGroundedAnswerParagraphAuthorizationDecision validates the relation before code discards a negative candidate immediately or performs pairwise evidence attribution for only that positive candidate.")
	default:
		return SemanticUncertaintyContract{}, false
	}
	return contract, true
}
