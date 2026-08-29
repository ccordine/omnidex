package assemblyline

type webEvidenceTextProjection struct {
	Title   string `json:"title"`
	Snippet string `json:"snippet"`
	Content string `json:"content"`
}

func projectWebGroundedEvidenceText(
	evidence []WebGroundedEvidence,
) []webEvidenceTextProjection {
	projected := make([]webEvidenceTextProjection, len(evidence))
	for index, item := range evidence {
		projected[index] = webEvidenceTextProjection{
			Title: item.Title, Snippet: item.Snippet, Content: item.Content,
		}
	}
	return projected
}
