package assemblyline

func minifiedObjectiveContext(content string) ObjectiveContext {
	sourceContent := "exact code-acquired source for test capsule: " + content
	return ObjectiveContext{Capsules: []ObjectiveContextCapsule{{
		Sources: []ObjectiveContextSource{{
			Namespace:     "conversation_transcript",
			CandidateID:   "CTX_1",
			ContentSHA256: ExactObjectiveContextSHA(sourceContent),
		}},
		Content:       content,
		ContentSHA256: ExactObjectiveContextSHA(content),
	}}}
}
