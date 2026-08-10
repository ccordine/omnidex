package cognition

func planRevisionProposalSHA256(proposal PlanRevisionProposal) (string, error) {
	canonical, err := canonicalPlanRevisionProposal(proposal)
	if err != nil {
		return "", err
	}
	return cognitionValueSHA256(struct {
		Schema   string
		Proposal PlanRevisionProposal
	}{PlanRevisionMaterializationSchemaV1, canonical})
}

func planRevisionMaterializationSHA256(value PlanRevisionMaterialization) (string, error) {
	return cognitionValueSHA256(struct {
		Schema, SourceSnapshot, SourceDecision, SourceProposal string
		ProposalIndex                                          int
		EpisodeID                                              EpisodeID
		PreviousGeneration, NextGeneration                     uint64
		ExpectedGraph                                          string
		Active                                                 ObligationID
		Authority                                              CompletionAuthority
		Root, Next                                             ObligationSpec
		ResultGraph                                            string
	}{
		value.Schema, value.SourceSnapshotSHA256, value.SourceDecisionSHA256,
		value.SourceProposalSHA256, value.ProposalIndex, value.EpisodeID,
		value.PreviousGeneration, value.NextGeneration, value.ExpectedGraphSHA256,
		value.ActiveObligationID, value.CompletionAuthority, value.Root, value.Next,
		value.ResultGraphSHA256,
	})
}
