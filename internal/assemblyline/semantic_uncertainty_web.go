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
	default:
		return SemanticUncertaintyContract{}, false
	}
	return contract, true
}
