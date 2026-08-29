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
	case WorkWebSynthesisParagraphCoverage:
		contract = semanticUncertaintyContract(kind,
			"Is one more distinct grounded paragraph necessary to answer the exact question?",
			"Paragraph-level semantic answer coverage cannot be computed from evidence counts or text shape.",
			"The exact question, compact context, bounded evidence text, retained paragraphs, and paragraph limit.",
			"One registered paragraph-coverage relation.",
			"DecodeWebSynthesisParagraphCoverageDecision validates the relation before code continues or closes the bounded paragraph set.")
	case WorkWebSynthesisParagraph:
		contract = semanticUncertaintyContract(kind,
			"What single next grounded paragraph is necessary to answer the exact question?",
			"Evidence parsing cannot mechanically compose the needed natural-language paragraph.",
			"The exact question, compact context, bounded evidence text, retained paragraphs, and paragraph byte limit.",
			"One grounded answer paragraph.",
			"DecodeWebSynthesisParagraphDecision validates the paragraph before code retains it for support evaluation.")
	case WorkWebSynthesisEvidenceRelation:
		contract = semanticUncertaintyContract(kind,
			"Does one evidence capsule support a factual claim in the focused paragraph?",
			"Claim-level semantic support cannot be established by lexical overlap.",
			"The exact question, compact context, focused paragraph, and one bounded evidence-capsule text.",
			"One registered paragraph-support relation.",
			"DecodeWebSynthesisEvidenceRelationDecision validates the relation before code attaches the evidence identity.")
	default:
		return SemanticUncertaintyContract{}, false
	}
	return contract, true
}
