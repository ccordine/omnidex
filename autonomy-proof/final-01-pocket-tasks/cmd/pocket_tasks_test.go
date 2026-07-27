package main

import (
	"os"
	"path/filepath"
	"testing"

	"your-module/domain"
	"your-module/storage"
)

func TestCreateTask(t *testing.T) {
	// Create a temporary file for testing
	tmpFile, err := os.CreateTemp("", "tasks_test.json")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	// Create storage and service
	storage, err := storage.NewJSONTaskStorage(tmpFile.Name())
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}

	service := domain.NewTaskService(storage)

	// Test creating a task
	task, err := service.CreateTask("Test task")
	if err != nil {
		t.Fatalf("Failed to create task: %v", err)
	}

	if task.Title != "Test task" {
		t.Errorf("Expected task title 'Test task', got '%s'", task.Title)
	}

	if task.Completed {
		t.Error("Expected task to not be completed")
	}

	// Test retrieving the task
	retrievedTask, err := service.GetTask(task.ID)
	if err != nil {
		t.Fatalf("Failed to get task: %v", err)
	}

	if retrievedTask.Title != "Test task" {
		t.Errorf("Expected retrieved task title 'Test task', got '%s'", retrievedTask.Title)
	}
}

func TestCompleteTask(t *testing.T) {
	// Create a temporary file for testing
	tmpFile, err := os.CreateTemp("", "tasks_test.json")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	// Create storage and service
	storage, err := storage.NewJSONTaskStorage(tmpFile.Name())
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}

	service := domain.NewTaskService(storage)

	// Create a task first
	task, err := service.CreateTask("Test task")
	if err != nil {
		t.Fatalf("Failed to create task: %v", err)
	}

	// Complete the task
	err = service.CompleteTask(task.ID)
	if err != nil {
		t.Fatalf("Failed to complete task: %v", err)
	}

	// Verify the task is completed
	retrievedTask, err := service.GetTask(task.ID)
	if err != nil {
		t.Fatalf("Failed to get task: %v", err)
	}

	if !retrievedTask.Completed {
		t.Error("Expected task to be completed")
	}
}

func TestGetTasks(t *testing.T) {
	// Create a temporary file for testing
	tmpFile, err := os.CreateTemp("", "tasks_test.json")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	// Create storage and service
	storage, err := storage.NewJSONTaskStorage(tmpFile.Name())
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}

	service := domain.NewTaskService(storage)

	// Create some tasks
	_, err = service.CreateTask("Task 1")
	if err != nil {
		t.Fatalf("Failed to create task: %v", err)
	}

	_, err = service.CreateTask("Task 2")
	if err != nil {
		t.Fatalf("Failed to create task: %v", err)
	}

	// Get all tasks
	tasks, err := service.GetTasks()
	if err != nil {
		t.Fatalf("Failed to get tasks: %v", err)
	}

	if len(tasks) != 2 {
		t.Errorf("Expected 2 tasks, got %d", len(tasks))
	}
}

func TestDeleteTask(t *testing.T) {
	// Create a temporary file for testing
	tmpFile, err := os.CreateTemp("", "tasks_test.json")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	// Create storage and service
	storage, err := storage.NewJSONTaskStorage(tmpFile.Name())
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}

	service := domain.NewTaskService(storage)

	// Create a task
	task, err := service.CreateTask("Test task")
	if err != nil {
		t.Fatalf("Failed to create task: %v", err)
	}

	// Delete the task
	err = service.DeleteTask(task.ID)
	if err != nil {
		t.Fatalf("Failed to delete task: %v", err)
	}

	// Verify the task is deleted
	_, err = service.GetTask(task.ID)
	if err == nil {
		t.Error("Expected error when getting deleted task")
	}
}

func TestEmptyTaskTitle(t *testing.T) {
	// Create a temporary file for testing
	tmpFile, err := os.CreateTemp("", "tasks_test.json")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	// Create storage and service
	storage, err := storage.NewJSONTaskStorage(tmpFile.Name())
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}

	service := domain.NewTaskService(storage)

	// Try to create a task with empty title
	_, err = service.CreateTask("")
	if err != nil {
		t.Logf("Expected error for empty task title: %v", err)
	} else {
		t.Error("Expected error for empty task title")
	}
}

func TestNonExistentTask(t *testing.T) {
	// Create a temporary file for testing
	tmpFile, err := os.CreateTemp("", "tasks_test.json")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	// Create storage and service
	storage, err := storage.NewJSONTaskStorage(tmpFile.Name())
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}

	service := domain.NewTaskService(storage)

	// Try to complete a non-existent task
	err = service.CompleteTask("non-existent-id")
	if err != nil {
		t.Logf("Expected error for non-existent task: %v", err)
	} else {
		t.Error("Expected error for non-existent task")
	}
}
