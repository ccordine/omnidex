package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

func (s *Server) handleScrum(w http.ResponseWriter, r *http.Request) {
	if !s.scrumAvailable() {
		writeError(w, http.StatusServiceUnavailable, "scrum store unavailable")
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
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleScrumCards(w http.ResponseWriter, r *http.Request) {
	if !s.scrumAvailable() {
		writeError(w, http.StatusServiceUnavailable, "scrum store unavailable")
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Title       string `json:"title"`
		Description string `json:"description"`
		Column      string `json:"column"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	card, err := s.scrumCreateCard(r, req.Title, req.Description, req.Column)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"card": card})
}

func (s *Server) handleScrumCardByID(w http.ResponseWriter, r *http.Request) {
	if !s.scrumAvailable() {
		writeError(w, http.StatusServiceUnavailable, "scrum store unavailable")
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
		case "jira":
			s.handleScrumCardJira(w, r, cardID)
		case "coach":
			s.handleScrumCardCoach(w, r, cardID)
		case "coach-config":
			s.handleScrumCardCoachConfig(w, r, cardID)
		case "tags-suggest":
			s.handleScrumCardTagsSuggest(w, r, cardID)
		case "move":
			s.handleScrumCardMove(w, r, cardID)
		case "done":
			s.handleScrumCardDone(w, r, cardID)
		case "sync":
			s.handleScrumCardSync(w, r)
		default:
			writeError(w, http.StatusNotFound, "unknown card action")
		}
		return
	}
	switch r.Method {
	case http.MethodPatch:
		body, err := io.ReadAll(r.Body)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		raw := map[string]json.RawMessage{}
		if err := json.Unmarshal(body, &raw); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json body")
			return
		}
		var patch ScrumCard
		if err := json.Unmarshal(body, &patch); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json body")
			return
		}
		card, err := s.scrumUpdateCard(r, cardID, patch, raw)
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
	var req struct {
		Column       string `json:"column"`
		BeforeCardID string `json:"before_card_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
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
	card, err := s.scrumMoveCard(r, cardID, "done", "")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"card": card})
}

func (s *Server) scrumLLMGenerate(ctx context.Context, system, user string) (string, error) {
	if s.llmClient == nil {
		return "", fmt.Errorf("no llm client configured")
	}
	modelName := firstNonEmpty(s.ollamaDefaultModel, "llama3.2")
	prompt := strings.TrimSpace(system + "\n\n" + user)
	return s.llmClient.Generate(ctx, modelName, prompt)
}

func (s *Server) runScrumDirectInstruct(ctx context.Context, instruction string, board ScrumBoard, card ScrumCard) (string, error) {
	system := strings.Join([]string{
		"You are the Omni scrum task pilot.",
		"Think through the task, then provide actionable next steps and evidence.",
		"Project directory: " + board.ProjectDirectory,
		"Reference files: " + strings.Join(card.RefFiles, ", "),
	}, "\n")
	return s.scrumLLMGenerate(ctx, system, instruction)
}

func (s *Server) handleScrumCardChat(w http.ResponseWriter, r *http.Request, cardID string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Message string `json:"message"`
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
	card, board, _, err := s.scrumGetCard(r, cardID)
	if err != nil {
		writeError(w, http.StatusNotFound, "card not found")
		return
	}
	if _, err := s.scrumAppendChat(r, cardID, "user", req.Message); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	reply := ""
	if s.llmClient != nil {
		contextLines := []string{
			"Scrum card: " + card.Title,
			"Column: " + card.Column,
			"Project directory: " + board.ProjectDirectory,
			"Description: " + card.Description,
			"Reference files: " + strings.Join(card.RefFiles, ", "),
		}
		for _, item := range card.Checklist {
			state := "[ ]"
			if item.Done {
				state = "[x]"
			}
			contextLines = append(contextLines, fmt.Sprintf("%s %s", state, item.Text))
		}
		for _, msg := range card.Chat {
			contextLines = append(contextLines, msg.Role+": "+msg.Content)
		}
		contextLines = append(contextLines, "user: "+req.Message)
		reply, err = s.scrumLLMGenerate(r.Context(), "You are the Omni thinking pilot for a scrum card. Reason about the task and suggest concrete actions.", strings.Join(contextLines, "\n"))
		if err != nil {
			writeError(w, http.StatusBadGateway, err.Error())
			return
		}
	} else {
		reply = "LLM client unavailable. Configure Ollama or another provider in Settings."
	}
	updated, err := s.scrumAppendChat(r, cardID, "assistant", reply)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"card": updated, "reply": reply})
}

func (s *Server) handleScrumCardJira(w http.ResponseWriter, r *http.Request, cardID string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Prompt string `json:"prompt"`
		Ticket string `json:"ticket"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	card, board, _, err := s.scrumGetCard(r, cardID)
	if err != nil {
		writeError(w, http.StatusNotFound, "card not found")
		return
	}
	ticket := strings.TrimSpace(req.Ticket)
	if ticket == "" {
		prompt := strings.TrimSpace(req.Prompt)
		if prompt == "" {
			prompt = "Draft a Jira ticket for this scrum card."
		}
		contextLines := []string{
			"Scrum card: " + card.Title,
			"Column: " + card.Column,
			"Project directory: " + board.ProjectDirectory,
			"Description: " + card.Description,
			"Reference files: " + strings.Join(card.RefFiles, ", "),
		}
		for _, item := range card.Checklist {
			state := "[ ]"
			if item.Done {
				state = "[x]"
			}
			contextLines = append(contextLines, fmt.Sprintf("%s %s", state, item.Text))
		}
		for _, item := range card.TestCriteria {
			if strings.TrimSpace(item.Text) == "" {
				continue
			}
			contextLines = append(contextLines, "Test: "+item.Text)
		}
		if len(card.Tags) > 0 {
			contextLines = append(contextLines, "Tags: "+strings.Join(card.Tags, ", "))
		}
		contextLines = append(contextLines, "Author prompt: "+prompt)
		system := strings.Join([]string{
			"You are a technical project manager drafting Jira tickets.",
			"Return markdown with sections: Summary, Description, Acceptance Criteria (checklist), Test Criteria, Technical Notes.",
			"Test Criteria should list verifiable tests the implementer must satisfy.",
			"Be concise and actionable. Do not wrap the response in code fences.",
		}, "\n")
		generated, err := s.scrumLLMGenerate(r.Context(), system, strings.Join(contextLines, "\n"))
		if err != nil {
			writeError(w, http.StatusBadGateway, err.Error())
			return
		}
		ticket = strings.TrimSpace(generated)
	}
	ticketRaw, _ := json.Marshal(ticket)
	raw := map[string]json.RawMessage{"jira_ticket": ticketRaw}
	patch := ScrumCard{JiraTicket: ticket}
	updated, err := s.scrumUpdateCard(r, cardID, patch, raw)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"card": updated, "ticket": ticket})
}

func (s *Server) handleScrumFiles(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.scrumAvailable() {
		writeError(w, http.StatusServiceUnavailable, "scrum store unavailable")
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
	writeJSON(w, http.StatusOK, map[string]any{"files": files, "root": root})
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
	writeJSON(w, http.StatusOK, map[string]any{
		"board":        board,
		"cards_by_col": cardsByColumn(board),
		"play_queue":   scrumPlayQueueSummary(board),
	})
}

func parseJobID(raw string) (int64, error) {
	raw = strings.TrimSpace(raw)
	var id int64
	_, err := fmt.Sscan(raw, &id)
	return id, err
}
