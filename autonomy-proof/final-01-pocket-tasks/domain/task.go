package domain

import (
	"time"
)

// Task represents a single task in the system
type Task struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Completed bool      `json:"completed"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TaskRepository defines the interface for task persistence
type TaskRepository interface {
	Save(task *Task) error
	FindAll() ([]*Task, error)
	FindByID(id string) (*Task, error)
	Delete(id string) error
}

// TaskService provides business logic for tasks
type TaskService struct {
	repo TaskRepository
}

// NewTaskService creates a new task service
func NewTaskService(repo TaskRepository) *TaskService {
	return &TaskService{repo: repo}
}

// CreateTask creates a new task
func (s *TaskService) CreateTask(title string) (*Task, error) {
	task := &Task{
		ID:        generateID(),
		Title:     title,
		Completed: false,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := s.repo.Save(task); err != nil {
		return nil, err
	}

	return task, nil
}

// GetTasks retrieves all tasks
func (s *TaskService) GetTasks() ([]*Task, error) {
	return s.repo.FindAll()
}

// GetTask retrieves a task by ID
func (s *TaskService) GetTask(id string) (*Task, error) {
	return s.repo.FindByID(id)
}

// CompleteTask marks a task as completed
func (s *TaskService) CompleteTask(id string) error {
	task, err := s.repo.FindByID(id)
	if err != nil {
		return err
	}

	task.Completed = true
	task.UpdatedAt = time.Now()

	return s.repo.Save(task)
}

// DeleteTask removes a task
func (s *TaskService) DeleteTask(id string) error {
	return s.repo.Delete(id)
}

// generateID creates a simple ID for a task
func generateID() string {
	return time.Now().Format("20060102150405")
}
