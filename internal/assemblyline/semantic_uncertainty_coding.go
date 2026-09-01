package assemblyline

func codingSemanticUncertaintyContract(
	kind WorkKind,
) (SemanticUncertaintyContract, bool) {
	var contract SemanticUncertaintyContract
	switch kind {
	case WorkArtifactHandling:
		contract = semanticUncertaintyContract(kind,
			"What explicit authority does the user grant over the focused repository artifact?",
			"The state relation expressed in free-form language cannot be derived from artifact existence.",
			"The immutable request, one opaque artifact token, and the closed handling-relation vocabulary.",
			"One registered artifact-handling relation.",
			"DecodeArtifactHandlingDecision validates the relation before code updates the focused artifact obligation.")
	case WorkCapabilityRelation:
		contract = semanticUncertaintyContract(kind,
			"What direct live-state dependency relation exists between two focused local behaviors?",
			"Behavior text does not mechanically reveal whether one uniquely produced result is required by the other.",
			"The bounded local context, left behavior need, right behavior need, and registered relation vocabulary.",
			"One registered direct capability relation.",
			"DecodeCapabilityRelationDecision validates the relation before code adds the corresponding dependency edge.")
	case WorkFragmentGeneration:
		contract = semanticUncertaintyContract(kind,
			"What implementation body fulfills the exact local behavioral contract?",
			"Parsing and type rules validate source but cannot synthesize the semantically intended implementation bytes.",
			"The source language, exact signature, local behavior, source dialect, direct capabilities, and permitted symbols.",
			"One ordinary plain-text implementation body.",
			"Code places the body inside its declaration, parses and validates the result, and either accepts it or continues the same persisted job with one exact defect.")
	default:
		return SemanticUncertaintyContract{}, false
	}
	return contract, true
}
