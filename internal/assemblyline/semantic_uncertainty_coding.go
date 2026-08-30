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
	case WorkRepositoryArtifactAbsence:
		contract = semanticUncertaintyContract(kind,
			"Does the exact requirement explicitly require one complete repository-owned semantic artifact to be absent?",
			"Complete semantic absence cannot be distinguished from partial behavior change by structural parsing.",
			"One exact cohesive requirement statement.",
			"One registered repository-artifact absence relation.",
			"DecodeRepositoryArtifactAbsenceDecision validates the relation before code may derive an absence obligation.")
	case WorkPlainTextArtifactCreation:
		contract = semanticUncertaintyContract(kind,
			"Does the exact cohesive requirement call for exactly one complete unstructured plain-text artifact with no other change?",
			"The completeness and exclusivity of free-form artifact intent cannot be proven by syntax alone.",
			"One exact cohesive requirement statement.",
			"One registered plain-text artifact-creation relation.",
			"DecodePlainTextArtifactCreationDecision validates the relation before code may create one text-node responsibility.")
	case WorkDeclarationArtifactBoundary:
		contract = semanticUncertaintyContract(kind,
			"What explicit artifact boundary does the requirement assign to the focused declaration?",
			"A declaration signature establishes syntax but not its semantically intended ownership boundary.",
			"The exact requirement statement, exact Go signature, and opaque declaration identity.",
			"One registered declaration-artifact boundary relation.",
			"DecodeDeclarationArtifactBoundaryDecision validates the relation before code updates the artifact graph.")
	case WorkArtifactCandidateSelection:
		contract = semanticUncertaintyContract(kind,
			"Which known semantic artifact is explicitly identified as required to be absent?",
			"Bounded declarations establish candidates but cannot resolve ambiguous natural-language reference exactly.",
			"The exact absence requirement and code-enumerated opaque candidates with bounded declaration evidence.",
			"One opaque artifact-candidate ID.",
			"DecodeArtifactCandidateSelectionDecision validates the ID before code binds repository identity.")
	case WorkCapabilityRelation:
		contract = semanticUncertaintyContract(kind,
			"What direct live-state dependency relation exists between two focused local behaviors?",
			"Behavior text does not mechanically reveal whether one uniquely produced result is required by the other.",
			"The bounded local context, left behavior need, right behavior need, and registered relation vocabulary.",
			"One registered direct capability relation.",
			"DecodeCapabilityRelationDecision validates the relation before code adds the corresponding dependency edge.")
	case WorkSkillSelection:
		contract = semanticUncertaintyContract(kind,
			"Which existing learned skill directly covers the focused local need?",
			"Natural-language capability matching cannot be replaced by exact string or keyword comparison.",
			"The bounded local context, one local need, and code-enumerated opaque skill-purpose candidates.",
			"One opaque learned-skill candidate token.",
			"DecodeSkillSelectionDecision validates the token before code binds the accepted skill version.")
	case WorkRuntimeCapabilityNecessity:
		contract = semanticUncertaintyContract(kind,
			"Is this one exact registered runtime behavior necessary for the focused local need?",
			"Natural-language behavior cannot be matched exactly to a minimal technical runtime boundary by parsing or keyword rules.",
			"The bounded local context including direct-dependency purpose summaries, one local need, selected source dialect, and one exact candidate purpose with its identity hidden.",
			"One candidate-bound necessary-or-not-necessary relation.",
			"DecodeRuntimeCapabilityNecessityDecision validates the relation before code retains or discards only that candidate and advances its queue.")
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
