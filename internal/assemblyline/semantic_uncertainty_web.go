package assemblyline

func webSemanticUncertaintyContract(
	kind WorkKind,
) (SemanticUncertaintyContract, bool) {
	var contract SemanticUncertaintyContract
	switch kind {
	case WorkWebRelevanceRelation:
		contract = semanticUncertaintyContract(kind,
			"Is one web evidence candidate directly relevant to the exact question?",
			"Retrieval metadata cannot establish semantic relevance to a free-form question.",
			"The exact question, compact objective context, and one bounded candidate summary.",
			"One registered web-evidence relevance relation.",
			"DecodeWebRelevanceRelationLeaf validates the relation before code retains or discards the candidate identity.")
	case WorkWebSynthesisParagraphInventory:
		contract = semanticUncertaintyContractV3(kind,
			"What bounded source-ordered inventory of candidate answer paragraphs, if any, can be composed for the exact question from the supplied evidence?",
			"Natural-language paragraph composition and answer segmentation cannot be derived mechanically from evidence text.",
			"The exact question, compact context, bounded evidence text, and exact paragraph count and byte bounds.",
			"One bounded raw-line candidate paragraph inventory or the registered semantic absence.",
			"DecodeWebSynthesisParagraphInventory parses and binds the inventory before code alone owns the candidate queue, authorization sieve, supporting-evidence binding, accepted order, and exhaustion.")
	case WorkWebSynthesisEvidenceRelation:
		contract = semanticUncertaintyContractV2(kind,
			"Does one evidence capsule support a factual claim in the focused paragraph?",
			"Claim-level semantic support cannot be established by lexical overlap.",
			"The exact question, compact context, focused paragraph, and one bounded evidence-capsule text.",
			"One registered paragraph-support relation.",
			"DecodeWebSynthesisEvidenceRelationDecision validates the relation before code attaches that evidence identity to the already authorized candidate.")
	case WorkWebSynthesisParagraphAuthorization:
		contract = semanticUncertaintyContractV2(kind,
			"Does the complete exact paragraph directly answer the exact question with every factual claim fully supported by the supplied evidence?",
			"A capsule supporting at least one claim does not mechanically establish question responsiveness or full-claim entailment for the complete paragraph.",
			"The exact question, compact context, one focused paragraph, and the full bounded supplied evidence set.",
			"One registered paragraph question-and-evidence authorization relation.",
			"DecodeWebSynthesisParagraphAuthorizationDecision validates the relation before code discards a negative candidate immediately or binds exact supporting evidence only for that positive candidate; earlier accepted paragraphs are never reopened.")
	default:
		return SemanticUncertaintyContract{}, false
	}
	return contract, true
}
