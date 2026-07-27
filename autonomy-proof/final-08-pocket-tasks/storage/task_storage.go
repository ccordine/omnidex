package storage

import (
	"sync"
	"time"

	"omnidex.local/app/domain"
)

type TaskStorage struct {
	tasks map[string]*domain.Task
	mutex sync.RWMutex
}

func NewTaskStorage() *TaskStorage {
	return &TaskStorage{
		tasks: make(map[string]*domain.Task),
	}
}

func (ts *TaskStorage) Save(task *domain.Task) error {
	ts.mutex.Lock()
	defer ts.mutex.Unlock()
	ts.tasks[task.ID] = task
	return nil
}

func (ts *TaskStorage) FindByID(id string) (*domain.Task, error) {
	ts.mutex.RLock()
	defer ts.mutex.RUnlock()
	task, exists := ts.tasks[id]
	if !exists {
		return nil, &TaskNotFoundError{ID: id}
	}
	return task, nil
}

func (ts *TaskStorage) FindAll() []*domain.Task {
	ts.mutex.RLock()
	defer ts.mutex.RUnlock()
	tasks := make([]*domain.Task, 0, len(ts.tasks))
	for _, task := range ts.tasks {
		tasks = append(tasks, task)
	}
	return tasks
}

func (ts *TaskStorage) Delete(id string) error {
	ts.mutex.Lock()
	defer ts.mutex.Unlock()
	if _, exists := ts.tasks[id]; !exists {
		return &TaskNotFoundError{ID: id}
	}
	delete(ts.tasks, id)
	return nil
}

func (ts *TaskStorage) Update(id string, updateFunc func(*domain.Task)) error {
	ts.mutex.Lock()
	defer ts.mutex.Unlock()
	task, exists := ts.tasks[id]
	if !exists {
		return &TaskNotFoundError{ID: id}
	}
	updateFunc(task)
	task.UpdatedAt = time.Now()
	return nil
}

type TaskNotFoundError struct {
	ID string
}

func (e *TaskNotFoundError) Error() string {
	return "task not found: " + e.ID
}
