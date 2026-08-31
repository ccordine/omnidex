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
	case WorkTypeScriptRepairGuidance:
		contract = semanticUncertaintyContract(kind,
			"What single source transformation resolves the exact compiler-proven failure in the mutable source?",
			"Compiler diagnostics prove invalidity but do not uniquely specify the intended behavior-preserving repair.",
			"The exact signature, mutable source, bounded diagnostic, proven lexical scope, direct capabilities, and permitted symbols.",
			"One self-contained imperative repair instruction.",
			"DecodeFragmentRepairGuidanceResult validates the instruction before code dispatches the separate correction responsibility.")
	case WorkFragmentGeneration:
		contract = semanticUncertaintyContract(kind,
			"What single declaration body fulfills the exact local behavioral contract?",
			"Parsing and type rules validate source but cannot synthesize the semantically intended implementation bytes.",
			"The source language, exact signature, local behavior, source dialect, direct capabilities, and permitted symbols.",
			"One exact parseable source declaration.",
			"The registered fragment projector parses and validates the declaration before code stitches it into the in-memory document.")
	case WorkFragmentGenerationReplacement:
		contract = semanticUncertaintyContract(kind,
			"What complete non-redundant declaration fulfills the unchanged exact local behavioral contract after the prior response exhausted its provider output boundary?",
			"Code can prove that the prior response ended before completion but cannot synthesize the semantically intended implementation bytes.",
			"The unchanged source language, signature, local behavior, dialect, direct capabilities, permitted symbols, and exact bounded output-limit fact.",
			"One exact parseable source declaration.",
			"The registered fragment projector parses and validates the declaration before code stitches it into the in-memory document.")
	case WorkFragmentModification:
		contract = semanticUncertaintyContract(kind,
			"What replacement declaration applies the exact requirement to the current declaration?",
			"The requested semantic delta cannot be derived mechanically from valid existing source.",
			"The source language, exact signature, current declaration, exact requirement, dialect, direct capabilities, and permitted symbols.",
			"One exact parseable replacement declaration.",
			"The registered Go fragment projector validates the replacement before code substitutes the focused declaration.")
	case WorkFragmentCorrection:
		contract = semanticUncertaintyContract(kind,
			"What replacement source results from applying the exact repair instruction to the mutable source?",
			"The imperative semantic transformation has no mechanically unique source rendering.",
			"One exact repair instruction and one exact mutable source declaration or region.",
			"One exact parseable replacement source node.",
			"The registered fragment projector validates signature and capability boundaries before code replaces the mutable node.")
	default:
		return SemanticUncertaintyContract{}, false
	}
	return contract, true
}
