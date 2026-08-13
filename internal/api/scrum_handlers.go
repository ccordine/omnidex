package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gryph/omnidex/internal/queue"
)

func (s *Server) handleScrum(w http.ResponseWriter, r *http.Request) {
	if !s.scrumAvailable() {
		writeError(w, http.StatusServiceUnavailable, "postgres repository is required for Scrum")
		return
	}
	switch r.Method {
	case http.MethodGet:
		payload, err := s.scrumBoardResponse(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, payload)
	case http.MethodPut:
		var req struct {
			Name             string `json:"name"`
			ProjectDirectory string `json:"project_directory"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json body")
			return
		}
		board, err := s.scrumUpdateBoard(r, req.Name, req.ProjectDirectory)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"board": board})
	case http.MethodPatch:
		if s.repo == nil {
			writeError(w, http.StatusServiceUnavailable, "patch requires queue mode")
			return
		}
		var req struct {
			AutoWork            *ScrumAutoWorkConfig `json:"auto_work"`
			RemovedAutoReview   json.RawMessage      `json:"auto_review"`
			RemovedCreateTicket json.RawMessage      `json:"create_ticket"`
		}
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json body")
			return
		}
		if err := requireJSONEOF(decoder, "Scrum automation request"); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if len(req.RemovedCreateTicket) > 0 {
			writeRemovedInferenceAction(w, "Scrum create-ticket automation")
			return
		}
		if len(req.RemovedAutoReview) > 0 {
			writeRemovedInferenceAction(w, "Scrum auto-review automation")
			return
		}
		if req.AutoWork == nil {
			writeError(w, http.StatusBadRequest, "auto_work is required")
			return
		}
		projectID, err := s.resolveProjectID(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		project, err := s.repo.GetProject(r.Context(), projectID)
		if err != nil {
			writeError(w, http.StatusNotFound, "project not found")
			return
		}
		if req.AutoWork != nil {
			cfg := *req.AutoWork
			if err := s.saveScrumAutoWorkConfig(r.Context(), project, cfg); err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
			project, err = s.repo.GetProject(r.Context(), projectID)
			if err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
		}
		payload, err := s.scrumBoardResponse(r)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if req.AutoWork != nil {
			if req.AutoWork.Enabled {
				err = s.RefreshScrumPlayQueueForProjectAsync(projectID)
			} else {
				err = s.ReconcileScrumPlayQueueForProjectAsync(projectID)
			}
			if err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
		}
		writeJSON(w, http.StatusOK, payload)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleScrumCards(w http.ResponseWriter, r *http.Request) {
	if !s.scrumAvailable() {
		writeError(w, http.StatusServiceUnavailable, "postgres repository is required for Scrum")
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Title                     string          `json:"title"`
		Description               string          `json:"description"`
		Column                    string          `json:"column"`
		RemovedCreateTicket       json.RawMessage `json:"create_ticket"`
		RemovedCreateTicketConfig json.RawMessage `json:"create_ticket_config"`
	}
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	if err := requireJSONEOF(decoder, "Scrum card create request"); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if len(req.RemovedCreateTicket) > 0 || len(req.RemovedCreateTicketConfig) > 0 {
		writeRemovedInferenceAction(w, "Scrum card ticket generation")
		return
	}
	column := req.Column
	card, err := s.scrumCreateCard(r, req.Title, req.Description, column)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	payload := map[string]any{"card": card}
	writeJSON(w, http.StatusCreated, payload)
}

func (s *Server) handleScrumCardByID(w http.ResponseWriter, r *http.Request) {
	if !s.scrumAvailable() {
		writeError(w, http.StatusServiceUnavailable, "postgres repository is required for Scrum")
		return
	}
	cardID, action := splitScrumCardPath(r.URL.Path)
	if cardID == "" {
		writeError(w, http.StatusNotFound, "card not found")
		return
	}
	if action != "" {
		switch action {
		case "play":
			s.handleScrumCardPlay(w, r, cardID)
		case "pause":
			s.handleScrumCardPause(w, r, cardID)
		case "chat":
			s.handleScrumCardChat(w, r, cardID)
		case "card-ticket":
			s.handleScrumCardTicket(w, r, cardID)
		case "checklist":
			s.handleScrumCardItem(w, r, cardID, queue.ScrumCardChecklist)
		case "test-criteria":
			s.handleScrumCardItem(w, r, cardID, queue.ScrumCardTestCriteria)
		case "coach":
			writeRemovedInferenceAction(w, "Scrum card coach")
		case "coach-config":
			writeRemovedInferenceAction(w, "Scrum card coach configuration")
		case "tags-suggest":
			s.handleScrumCardTagsSuggest(w, r, cardID)
		case "files":
			s.handleScrumCardFiles(w, r, cardID)
		case "move":
			s.handleScrumCardMove(w, r, cardID)
		case "done":
			s.handleScrumCardDone(w, r, cardID)
		case "sync":
			s.handleScrumCardSync(w, r)
		case "modal":
			s.handleScrumCardModal(w, r, cardID)
		default:
			writeError(w, http.StatusNotFound, "unknown card action")
		}
		return
	}
	switch r.Method {
	case http.MethodGet:
		card, _, _, err := s.scrumGetCard(r, cardID)
		if err != nil {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"card": card,
		})
	case http.MethodPatch:
		edit, err := decodeScrumCardEditRequest(w, r)
		if err != nil {
			writeScrumCardEditBodyError(w, err)
			return
		}
		card, err := s.scrumEditCard(r, cardID, edit)
		if err != nil {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"card": card})
	case http.MethodDelete:
		if err := s.scrumDeleteCard(r, cardID); err != nil {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"deleted": cardID})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func splitScrumCardPath(path string) (cardID, action string) {
	path = strings.TrimPrefix(path, "/v1/scrum/cards/")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 0 || strings.TrimSpace(parts[0]) == "" {
		return "", ""
	}
	cardID = parts[0]
	if len(parts) > 1 {
		action = parts[1]
	}
	return cardID, action
}

func (s *Server) handleScrumCardMove(w http.ResponseWriter, r *http.Request, cardID string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	req, err := decodeScrumCardMoveRequest(w, r)
	if err != nil {
		writeScrumCardStateBodyError(w, err)
		return
	}
	card, err := s.scrumMoveCard(r, cardID, req.Column, req.BeforeCardID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"card": card})
}

func (s *Server) handleScrumCardDone(w http.ResponseWriter, r *http.Request, cardID string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := decodeScrumCardDoneRequest(w, r); err != nil {
		writeScrumCardStateBodyError(w, err)
		return
	}
	card, err := s.scrumMoveCard(r, cardID, "done", "")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"card": card})
}

func (s *Server) handleScrumCardChat(w http.ResponseWriter, r *http.Request, cardID string) {
	if r.Method == http.MethodGet {
		card, _, _, err := s.scrumGetCard(r, cardID)
		if err != nil {
			writeError(w, http.StatusNotFound, "card not found")
			return
		}
		limit, err := parseScrumChannelPageLimit(r.URL.Query().Get("limit"))
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		page, err := scrumChannelMessagePageFor(card, limit, r.URL.Query().Get("before"))
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"messages":      page.Messages,
			"before_cursor": page.BeforeCursor,
			"has_more":      page.HasMore,
			"busy":          scrumChannelBusy(card),
		})
		return
	}
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "card chat only accepts POST")
		return
	}
	var req struct {
		OperationID queue.LifecycleOperationID `json:"operation_id"`
		Message     string                     `json:"message"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	req.Message = strings.TrimSpace(req.Message)
	if req.Message == "" {
		writeError(w, http.StatusBadRequest, "message is required")
		return
	}
	if _, err := queue.ParseLifecycleOperationID(string(req.OperationID)); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	projectID, err := s.resolveProjectID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	operationRequest := queue.ScrumChannelOperationRequest{
		OperationID: req.OperationID, ProjectID: projectID, CardID: cardID, Message: req.Message,
	}
	if replay, found, err := s.repo.LoadScrumChannelOperation(r.Context(), operationRequest); err != nil {
		status := http.StatusBadGateway
		if errors.Is(err, queue.ErrLifecycleOperationConflict) {
			status = http.StatusConflict
		}
		writeError(w, status, err.Error())
		return
	} else if found {
		result, err := decodeScrumChannelResult(replay)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeScrumChannelDispatchResponse(w, result)
		return
	}
	card, board, resolvedProjectID, err := s.scrumGetCard(r, cardID)
	if err != nil {
		writeError(w, http.StatusNotFound, "card not found")
		return
	}
	if resolvedProjectID != projectID {
		writeError(w, http.StatusInternalServerError, "Scrum channel project authority changed during request")
		return
	}
	result, err := s.dispatchScrumChannelMessage(
		r, board, projectID, card, req.OperationID, req.Message,
	)
	if err != nil {
		status := http.StatusBadGateway
		if errors.Is(err, queue.ErrLifecycleOperationConflict) || errors.Is(err, queue.ErrStepNotWritable) {
			status = http.StatusConflict
		}
		writeError(w, status, err.Error())
		return
	}
	if result.Applied {
		s.publishScrumCardUpdate(r.Context(), projectID, result.Card, "channel chat")
	}
	writeScrumChannelDispatchResponse(w, result)
}

func (s *Server) handleScrumFiles(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.scrumAvailable() {
		writeError(w, http.StatusServiceUnavailable, "postgres repository is required for Scrum")
		return
	}
	root, err := s.scrumProjectDirectory(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if root == "" {
		writeJSON(w, http.StatusOK, map[string]any{"files": []string{}, "root": ""})
		return
	}
	root, err = filepath.Abs(root)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	sub := strings.TrimSpace(r.URL.Query().Get("sub"))
	target := root
	if sub != "" {
		target = filepath.Join(root, sub)
	}
	files := []string{}
	dirs := []string{}
	_ = filepath.WalkDir(target, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if path == target {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if strings.Count(rel, string(os.PathSeparator)) > 2 {
				return filepath.SkipDir
			}
			dirs = append(dirs, filepath.ToSlash(rel))
			return nil
		}
		if strings.Count(rel, string(os.PathSeparator)) > 3 {
			return nil
		}
		files = append(files, filepath.ToSlash(rel))
		if len(files) >= 200 {
			return fs.SkipAll
		}
		return nil
	})
	writeJSON(w, http.StatusOK, map[string]any{"files": files, "dirs": dirs, "root": root})
}

func (s *Server) handleScrumCardFiles(w http.ResponseWriter, r *http.Request, cardID string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	card, _, projectID, err := s.scrumGetCard(r, cardID)
	if err != nil {
		writeError(w, http.StatusNotFound, "card not found")
		return
	}
	root, err := s.scrumProjectDirectory(r)
	if err != nil || strings.TrimSpace(root) == "" {
		writeError(w, http.StatusBadRequest, "project directory is required for file uploads")
		return
	}
	root, err = filepath.Abs(root)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := r.ParseMultipartForm(64 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "invalid multipart upload")
		return
	}
	headers := r.MultipartForm.File["files"]
	if len(headers) == 0 {
		writeError(w, http.StatusBadRequest, "upload one or more files in the files field")
		return
	}
	uploadDir := filepath.Join(root, ".omni", "card-files", safeCardFileSegment(cardID))
	if err := os.MkdirAll(uploadDir, 0o755); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	refs := append([]string(nil), card.RefFiles...)
	uploaded := []string{}
	for _, header := range headers {
		name := safeCardFileSegment(header.Filename)
		if name == "" {
			continue
		}
		src, err := header.Open()
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		dstPath := filepath.Join(uploadDir, name)
		dst, err := os.OpenFile(dstPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
		if err != nil {
			src.Close()
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		_, copyErr := io.Copy(dst, io.LimitReader(src, 64<<20))
		closeErr := dst.Close()
		src.Close()
		if copyErr != nil {
			writeError(w, http.StatusInternalServerError, copyErr.Error())
			return
		}
		if closeErr != nil {
			writeError(w, http.StatusInternalServerError, closeErr.Error())
			return
		}
		rel, err := filepath.Rel(root, dstPath)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		ref := filepath.ToSlash(rel)
		uploaded = append(uploaded, ref)
		if !stringSliceContains(refs, ref) {
			refs = append(refs, ref)
		}
	}
	updated, err := s.scrumEditCard(r, cardID, scrumCardEditRequest{
		RefFiles: editableScrumCardField(refs),
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if projectID > 0 && s.repo != nil {
		s.publishScrumCardUpdate(r.Context(), projectID, updated, "files updated")
	}
	writeJSON(w, http.StatusOK, map[string]any{"card": updated, "uploaded": uploaded})
}

func safeCardFileSegment(name string) string {
	name = filepath.Base(strings.TrimSpace(name))
	name = strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			return r
		case r == '.', r == '-', r == '_':
			return r
		default:
			return '-'
		}
	}, name)
	return strings.Trim(name, ".- ")
}

func stringSliceContains(items []string, value string) bool {
	for _, item := range items {
		if item == value {
			return true
		}
	}
	return false
}

func (s *Server) handleScrumCardSync(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.repo == nil {
		writeError(w, http.StatusServiceUnavailable, "sync requires queue mode")
		return
	}
	board, projectID, err := s.loadScrumContext(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	board, err = s.refreshScrumPlayQueue(r, projectID, board)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	payload := map[string]any{
		"board":        board,
		"cards_by_col": cardsByColumn(board),
		"play_queue":   scrumPlayQueueSummary(board),
	}
	if projectID > 0 {
		automation, err := s.scrumAutomationSettings(r.Context(), projectID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		payload["auto_work"] = automation.AutoWork
	}
	writeJSON(w, http.StatusOK, payload)
}

func parseJobID(raw string) (int64, error) {
	raw = strings.TrimSpace(raw)
	var id int64
	_, err := fmt.Sscan(raw, &id)
	return id, err
}
