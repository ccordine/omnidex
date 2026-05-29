package api

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var scrumColumns = []string{"backlog", "ready", "assigned", "in_progress", "review", "done"}

type ScrumChecklistItem struct {
	ID   string `json:"id"`
	Text string `json:"text"`
	Done bool   `json:"done"`
}

type ScrumChatMessage struct {
	Role      string `json:"role"`
	Content   string `json:"content"`
	CreatedAt string `json:"created_at"`
}

type ScrumCard struct {
	ID          string               `json:"id"`
	Title       string               `json:"title"`
	Description string               `json:"description"`
	Column      string               `json:"column"`
	Checklist   []ScrumChecklistItem `json:"checklist"`
	RefFiles    []string             `json:"ref_files"`
	Chat        []ScrumChatMessage   `json:"chat"`
	ModelConfig json.RawMessage      `json:"model_config,omitempty"`
	AgentConfig json.RawMessage      `json:"agent_config,omitempty"`
	JobID       string               `json:"job_id,omitempty"`
	ConsoleLog  string               `json:"console_log,omitempty"`
	PlayState   string               `json:"play_state,omitempty"`
	QueueOrder  int                  `json:"queue_order,omitempty"`
	CreatedAt   string               `json:"created_at"`
	UpdatedAt   string               `json:"updated_at"`
}

type ScrumBoard struct {
	ID               string      `json:"id"`
	Name             string      `json:"name"`
	ProjectDirectory string      `json:"project_directory"`
	Columns          []string    `json:"columns"`
	Cards            []ScrumCard `json:"cards"`
	UpdatedAt        string      `json:"updated_at"`
}

type ScrumStore struct {
	mu       sync.Mutex
	filePath string
	board    ScrumBoard
}

func NewScrumStore() (*ScrumStore, error) {
	path, err := scrumBoardPath()
	if err != nil {
		return nil, err
	}
	store := &ScrumStore{filePath: path}
	if err := store.load(); err != nil {
		return nil, err
	}
	return store, nil
}

func scrumBoardPath() (string, error) {
	root := strings.TrimSpace(os.Getenv("OMNI_SCRUM_ROOT"))
	if root == "" {
		home, err := os.UserHomeDir()
		if err != nil || strings.TrimSpace(home) == "" {
			root = ".omni/scrum"
		} else {
			root = filepath.Join(home, ".omni", "scrum")
		}
	}
	dir := filepath.Join(root, "boards")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create scrum board directory: %w", err)
	}
	return filepath.Join(dir, "default.json"), nil
}

func defaultScrumBoard() ScrumBoard {
	now := time.Now().UTC().Format(time.RFC3339)
	return ScrumBoard{
		ID:      "default",
		Name:    "Omni Scrum",
		Columns: append([]string(nil), scrumColumns...),
		Cards:   []ScrumCard{},
		UpdatedAt: now,
	}
}

func (s *ScrumStore) load() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := os.ReadFile(s.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			s.board = defaultScrumBoard()
			return s.saveLocked()
		}
		return err
	}
	var board ScrumBoard
	if err := json.Unmarshal(data, &board); err != nil {
		return err
	}
	if len(board.Columns) == 0 {
		board.Columns = append([]string(nil), scrumColumns...)
	}
	s.board = board
	return nil
}

func (s *ScrumStore) saveLocked() error {
	s.board.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	blob, err := json.MarshalIndent(s.board, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.filePath + ".tmp"
	if err := os.WriteFile(tmp, blob, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.filePath)
}

func (s *ScrumStore) Board() ScrumBoard {
	s.mu.Lock()
	defer s.mu.Unlock()
	return cloneBoard(s.board)
}

func (s *ScrumStore) UpdateBoard(name, projectDirectory string) (ScrumBoard, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if strings.TrimSpace(name) != "" {
		s.board.Name = strings.TrimSpace(name)
	}
	if strings.TrimSpace(projectDirectory) != "" {
		s.board.ProjectDirectory = strings.TrimSpace(projectDirectory)
	}
	if err := s.saveLocked(); err != nil {
		return ScrumBoard{}, err
	}
	return cloneBoard(s.board), nil
}

func (s *ScrumStore) CreateCard(title, description, column string) (ScrumCard, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return ScrumCard{}, fmt.Errorf("title is required")
	}
	column = normalizeScrumColumn(column)
	if column == "" {
		column = "backlog"
	}
	now := time.Now().UTC().Format(time.RFC3339)
	card := ScrumCard{
		ID:          newScrumID("card"),
		Title:       title,
		Description: strings.TrimSpace(description),
		Column:      column,
		Checklist:   []ScrumChecklistItem{},
		RefFiles:    []string{},
		Chat:        []ScrumChatMessage{},
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.board.Cards = append(s.board.Cards, card)
	if err := s.saveLocked(); err != nil {
		return ScrumCard{}, err
	}
	return card, nil
}

func (s *ScrumStore) UpdateCard(cardID string, patch ScrumCard) (ScrumCard, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	idx := s.findCardIndex(cardID)
	if idx < 0 {
		return ScrumCard{}, fmt.Errorf("card not found")
	}
	current := s.board.Cards[idx]
	if strings.TrimSpace(patch.Title) != "" {
		current.Title = strings.TrimSpace(patch.Title)
	}
	if patch.Description != "" {
		current.Description = patch.Description
	}
	if col := normalizeScrumColumn(patch.Column); col != "" {
		current.Column = col
	}
	if patch.Checklist != nil {
		current.Checklist = patch.Checklist
	}
	if patch.RefFiles != nil {
		current.RefFiles = patch.RefFiles
	}
	if len(patch.ModelConfig) > 0 {
		current.ModelConfig = patch.ModelConfig
	}
	if len(patch.AgentConfig) > 0 {
		current.AgentConfig = patch.AgentConfig
	}
	if patch.ConsoleLog != "" {
		current.ConsoleLog = patch.ConsoleLog
	}
	if strings.TrimSpace(patch.JobID) != "" {
		current.JobID = strings.TrimSpace(patch.JobID)
	}
	current.PlayState = strings.TrimSpace(patch.PlayState)
	current.QueueOrder = patch.QueueOrder
	current.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	s.board.Cards[idx] = current
	if err := s.saveLocked(); err != nil {
		return ScrumCard{}, err
	}
	return current, nil
}

func (s *ScrumStore) MoveCard(cardID, column string) (ScrumCard, error) {
	column = normalizeScrumColumn(column)
	if column == "" {
		return ScrumCard{}, fmt.Errorf("invalid column")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	idx := s.findCardIndex(cardID)
	if idx < 0 {
		return ScrumCard{}, fmt.Errorf("card not found")
	}
	s.board.Cards[idx].Column = column
	s.board.Cards[idx].UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	if err := s.saveLocked(); err != nil {
		return ScrumCard{}, err
	}
	return s.board.Cards[idx], nil
}

func (s *ScrumStore) DeleteCard(cardID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	idx := s.findCardIndex(cardID)
	if idx < 0 {
		return fmt.Errorf("card not found")
	}
	s.board.Cards = append(s.board.Cards[:idx], s.board.Cards[idx+1:]...)
	return s.saveLocked()
}

func (s *ScrumStore) AppendChat(cardID, role, content string) (ScrumCard, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	idx := s.findCardIndex(cardID)
	if idx < 0 {
		return ScrumCard{}, fmt.Errorf("card not found")
	}
	msg := ScrumChatMessage{
		Role:      strings.TrimSpace(role),
		Content:   strings.TrimSpace(content),
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}
	s.board.Cards[idx].Chat = append(s.board.Cards[idx].Chat, msg)
	s.board.Cards[idx].UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	if err := s.saveLocked(); err != nil {
		return ScrumCard{}, err
	}
	return s.board.Cards[idx], nil
}

func (s *ScrumStore) SetCardJob(cardID, jobID, column, consoleLog string) (ScrumCard, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	idx := s.findCardIndex(cardID)
	if idx < 0 {
		return ScrumCard{}, fmt.Errorf("card not found")
	}
	if strings.TrimSpace(jobID) != "" {
		s.board.Cards[idx].JobID = strings.TrimSpace(jobID)
	}
	if consoleLog != "" {
		s.board.Cards[idx].ConsoleLog = consoleLog
	}
	if col := normalizeScrumColumn(column); col != "" {
		s.board.Cards[idx].Column = col
		if col == "in_progress" {
			s.board.Cards[idx].PlayState = scrumPlayRunning
			s.board.Cards[idx].QueueOrder = 0
		}
	}
	s.board.Cards[idx].UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	if err := s.saveLocked(); err != nil {
		return ScrumCard{}, err
	}
	return s.board.Cards[idx], nil
}

func (s *ScrumStore) Card(cardID string) (ScrumCard, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	idx := s.findCardIndex(cardID)
	if idx < 0 {
		return ScrumCard{}, false
	}
	return s.board.Cards[idx], true
}

func (s *ScrumStore) findCardIndex(cardID string) int {
	cardID = strings.TrimSpace(cardID)
	for i, card := range s.board.Cards {
		if card.ID == cardID {
			return i
		}
	}
	return -1
}

func normalizeScrumColumn(column string) string {
	column = strings.ToLower(strings.TrimSpace(column))
	column = strings.ReplaceAll(column, " ", "_")
	column = strings.ReplaceAll(column, "-", "_")
	for _, allowed := range scrumColumns {
		if column == allowed {
			return allowed
		}
	}
	return ""
}

func nextPlayColumn(current string) string {
	switch normalizeScrumColumn(current) {
	case "ready":
		return "assigned"
	case "assigned":
		return "in_progress"
	case "in_progress":
		return "in_progress"
	case "review":
		return "review"
	default:
		return ""
	}
}

func newScrumID(prefix string) string {
	return fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
}

func cloneBoard(board ScrumBoard) ScrumBoard {
	out := board
	out.Columns = append([]string(nil), board.Columns...)
	out.Cards = make([]ScrumCard, len(board.Cards))
	copy(out.Cards, board.Cards)
	return out
}

func buildScrumPlayInstruction(board ScrumBoard, card ScrumCard) string {
	lines := []string{
		"Scrum task execution for card: " + card.Title,
	}
	if strings.TrimSpace(board.ProjectDirectory) != "" {
		lines = append(lines, "Project directory: "+board.ProjectDirectory)
	}
	if strings.TrimSpace(card.Description) != "" {
		lines = append(lines, "Description:", card.Description)
	}
	if len(card.RefFiles) > 0 {
		lines = append(lines, "Reference files:", strings.Join(card.RefFiles, "\n"))
	}
	pending := []string{}
	for _, item := range card.Checklist {
		if strings.TrimSpace(item.Text) == "" || item.Done {
			continue
		}
		pending = append(pending, "- [ ] "+item.Text)
	}
	if len(pending) > 0 {
		lines = append(lines, "Checklist (complete all):", strings.Join(pending, "\n"))
	}
	lines = append(lines, "Use the thinking pilot and execution layers. Produce evidence for each checklist item. Stop in review-ready state with summary of changes.")
	return strings.Join(lines, "\n\n")
}

func cardsByColumn(board ScrumBoard) map[string][]ScrumCard {
	out := map[string][]ScrumCard{}
	for _, col := range board.Columns {
		out[col] = []ScrumCard{}
	}
	for _, card := range board.Cards {
		col := normalizeScrumColumn(card.Column)
		if col == "" {
			col = "backlog"
		}
		out[col] = append(out[col], card)
	}
	for col := range out {
		sortCardsForColumn(col, out[col])
	}
	return out
}
