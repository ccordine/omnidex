package domain

import (
	"time"
)

type Task struct {
	ID        string
	Title     string
	Completed bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

func NewTask(id, title string) *Task {
	now := time.Now()
	return &Task{
		ID:        id,
		Title:     title,
		Completed: false,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func (t *Task) Complete() {
	t.Completed = true
	t.UpdatedAt = time.Now()
}

func (t *Task) UpdateTitle(title string) {
	t.Title = title
	t.UpdatedAt = time.Now()
}

func (t *Task) IsOverdue() bool {
	// This is a placeholder for future logic
	// that would check if a task is overdue
	return false
}
