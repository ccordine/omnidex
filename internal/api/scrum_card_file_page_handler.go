package api

import "net/http"

type scrumCardFilePagePayload struct {
	Files          []string `json:"files"`
	Dirs           []string `json:"dirs"`
	Path           string   `json:"file_path"`
	Parent         string   `json:"file_parent"`
	HasParent      bool     `json:"file_has_parent"`
	Offset         int      `json:"file_offset"`
	HasPrevious    bool     `json:"file_has_previous"`
	PreviousOffset int      `json:"file_previous_offset"`
	HasMore        bool     `json:"file_has_more"`
	NextOffset     int      `json:"file_next_offset"`
}

func (s *Server) loadScrumCardFilePage(r *http.Request, cardID string) (scrumCardFilePagePayload, error) {
	query, err := decodeScrumFilePageQuery(r)
	if err != nil {
		return scrumCardFilePagePayload{}, err
	}
	board, err := s.scrumBoardMetadataFromProject(r.Context(), query.ProjectID)
	if err != nil {
		return scrumCardFilePagePayload{}, err
	}
	if _, err := s.repo.GetScrumCard(r.Context(), query.ProjectID, cardID); err != nil {
		return scrumCardFilePagePayload{}, err
	}
	ctx := &scrumModalRenderContext{}
	if err := s.populateScrumModalFileContext(
		r, board.ProjectDirectory, query.FilePath, query.FileOffset, ctx,
	); err != nil {
		return scrumCardFilePagePayload{}, err
	}
	return scrumCardFilePagePayload{
		Files: append([]string{}, ctx.Files...), Dirs: append([]string{}, ctx.Dirs...),
		Path: ctx.FilePath, Parent: ctx.FileParent, HasParent: ctx.FileHasParent,
		Offset: ctx.FileOffset, HasPrevious: ctx.FileHasPrevious,
		PreviousOffset: ctx.FilePreviousOffset, HasMore: ctx.FileHasMore, NextOffset: ctx.FileNextOffset,
	}, nil
}
