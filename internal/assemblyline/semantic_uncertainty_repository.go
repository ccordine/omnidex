package assemblyline

func repositorySemanticUncertaintyContract(
	kind WorkKind,
) (SemanticUncertaintyContract, bool) {
	var contract SemanticUncertaintyContract
	switch kind {
	case WorkRepositoryRequirementInventory:
		contract = semanticUncertaintyContractV5(kind,
			"What bounded source-ordered raw-clause candidate inventory is explicitly present in the immutable existing-repository request?",
			"Semantic clause boundaries in unconstrained human phrasing cannot be derived by repository parsing or byte validation.",
			"Only the immutable repository request.",
			"One bounded raw-line inventory of exact source clauses without classifications or workflow authority.",
			"DecodeRepositoryRequirementInventory binds exact source quotes to request authority; code alone owns the candidate queue, duplicate filtering, and exhaustion.")
	case WorkRepositoryRequirementCandidateAuthorization:
		contract = semanticUncertaintyContractV3(kind,
			"Does this exact source clause require or directly constrain a persisted change to the existing workspace?",
			"Whether a natural-language clause establishes desired repository mutation is semantic and cannot be inferred from syntax or keywords.",
			"The immutable repository request, established context, and one exact inventory-bound source clause.",
			"One request-and-candidate-bound existing-workspace-change relation.",
			"DecodeRepositoryRequirementCandidateAuthorizationResult validates the relation before code continues sieving or discards only that queued candidate.")
	case WorkRepositoryRequirementCandidateRelation:
		contract = semanticUncertaintyContractV3(kind,
			"Do this exact authorized candidate and one retained requirement express the same requested existing-workspace change?",
			"Semantic equivalence between byte-different natural-language change statements cannot be established by syntax or byte comparison.",
			"Exactly one authorized candidate and one retained existing-workspace requirement.",
			"One pair-bound same-or-distinct workspace-change relation.",
			"DecodeRepositoryRequirementCandidateRelationResult validates the pair receipt before code discards only the duplicate candidate or compares the next retained requirement.")
	case WorkRepositoryEvidenceRelevanceRelation:
		contract = semanticUncertaintyContract(kind,
			"Is this one exact repository evidence candidate directly relevant to the exact repository requirement?",
			"Repository indexing establishes available evidence but not its semantic relevance to free-form intent.",
			"Only the exact requirement and one code-bound evidence candidate text, without other candidates or retained state.",
			"One candidate-bound directly-relevant-or-not-directly-relevant relation.",
			"DecodeRepositoryEvidenceRelevanceRelationResult validates the relation before code retains that candidate ID in source order or discards only that candidate; code owns the citation cap and exhaustion.")
	case WorkRepositoryChangeOwner:
		contract = semanticUncertaintyContract(kind,
			"Which eligible symbol directly owns the existing declaration that must change for the focused requirement?",
			"Parser-proven symbols expose structure but cannot assign requirement meaning to one declaration owner.",
			"The focused requirement plus bounded symbol signatures, relations, and code-owned eligibility.",
			"One opaque repository symbol ID.",
			"DecodeRepositoryChangeOwnerLeaf validates the ID before code binds the focused change responsibility.")
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
			"One raw candidate-paragraph inventory or the registered absence value.",
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
	case WorkRoleplayCanonFactInventory:
		contract = semanticUncertaintyContract(kind,
			"What bounded source-ordered candidate facts does the exact current contribution directly express?",
			"Attribution and durable narrative meaning require semantic interpretation beyond structural validation.",
			"The exact attributed contribution, reference context, and typed antecedent when present.",
			"One raw candidate-fact inventory or the registered absence value.",
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
	case WorkRoleplayOngoingAction:
		contract = semanticUncertaintyContract(kind,
			"What single action remains underway for the named character after the exact contribution?",
			"Completion or continuation of a described action is a narrative semantic relation not encoded structurally.",
			"The named character, contribution source, exact contribution, and previous ongoing-action state.",
			"One optional ongoing-action text leaf.",
			"DecodeRoleplayOngoingActionDecision validates the leaf before code replaces the character's persisted ongoing-action state.")
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
