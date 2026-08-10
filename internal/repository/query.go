package repository

type SymbolMatch struct {
	Symbol    Symbol  `json:"symbol"`
	MatchKind string  `json:"match_kind"`
	Score     float64 `json:"score"`
}

type GraphNeighborhood struct {
	AnalysisID string   `json:"analysis_id"`
	SubjectIDs []string `json:"subject_ids"`
	Edges      []Edge   `json:"edges"`
}
