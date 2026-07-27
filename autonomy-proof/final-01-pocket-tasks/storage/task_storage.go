package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"your-module/domain"
)

// JSONTaskStorage implements TaskRepository using JSON files
 type JSONTaskStorage struct {
	mu    sync.RWMutex
	file  string
	tasks []*domain.Task
}

// NewJSONTaskStorage creates a new JSON task storage
func NewJSONTaskStorage(file string) (*JSONTaskStorage, error) {
	storage := &JSONTaskStorage{
		file: file,
	}

	if err := storage.load(); err != nil {
		return nil, err
	}

	return storage, nil
}

// Save persists a task to JSON file
func (s *JSONTaskStorage) Save(task *domain.Task) error {
	s.mu.Lock()

defer s.mu.Unlock()

	// Find existing task or append new one
	for i, t := range s.tasks {
		if t.ID == task.ID {
			s.tasks[i] = task
			return s.save()
		}
	}

	s.tasks = append(s.tasks, task)
	return s.save()
}

// FindAll retrieves all tasks
func (s *JSONTaskStorage) FindAll() ([]*domain.Task, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Return a copy to prevent external modification
	tasks := make([]*domain.Task, len(s.tasks))
	for i, t := range s.tasks {
		tasks[i] = t
	}

	return tasks, nil
}

// FindByID retrieves a task by ID
func (s *JSONTaskStorage) FindByID(id string) (*domain.Task, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, task := range s.tasks {
		if task.ID == id {
			return task, nil
		}

	}

	return nil, fmt.Errorf("task with id %s not found", id)
}

// Delete removes a task by ID
func (s *JSONTaskStorage) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i, task := range s.tasks {
		if task.ID == id {
			s.tasks = append(s.tasks[:i], s.tasks[i+1:]...)
			return s.save()
		}
	}

	return fmt.Errorf("task with id %s not found", id)
}

// load reads tasks from JSON file
func (s *JSONTaskStorage) load() error {
	_, err := os.Stat(s.file)
	if os.IsNotExist(err) {
		// File doesn't exist, initialize empty tasks
		s.tasks = []*domain.Task{}
		return nil
	}

	data, err := os.ReadFile(s.file)
	if err != nil {
		return fmt.Errorf("failed to read file %s: %w", s.file, err)
	}

	if len(data) == 0 {
		s.tasks = []*domain.Task{}
		return nil
	}

	var tasks []*domain.Task
	if err := json.Unmarshal(data, &tasks); err != nil {
		return fmt.Errorf("failed to unmarshal tasks from file %s: %w", s.file, err)
	}

	s.tasks = tasks
	return nil
}

// save writes tasks to JSON file
func (s *JSONTaskStorage) save() error {
	data, err := json.MarshalIndent(s.tasks, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal tasks: %w", err)
	}

	// Ensure directory exists
	dir := filepath.Dir(s.file)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	if err := os.WriteFile(s.file, data, 0644); err != nil {
		return fmt.Errorf("failed to write file %s: %w", s.file, err)
	}

	return nil
}
