package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"your-module/domain"
	"your-module/storage"
)

func main() {
	file := os.Args[1]
	if file == "" {
		log.Fatal("File path cannot be empty")
	}

	storage, err := storage.NewJSONTaskStorage(file)
	if err != nil {
		log.Fatalf("Failed to create storage: %v", err)
	}

	service := domain.NewTaskService(storage)

	// Simulate some task operations
	task, err := service.CreateTask("Test task")
	if err != nil {
		log.Fatalf("Failed to create task: %v", err)
	}

	fmt.Printf("Created task: %s\n", task.ID)

	tasks, err := service.GetTasks()
	if err != nil {
		log.Fatalf("Failed to get tasks: %v", err)
	}

	for _, t := range tasks {
		fmt.Printf("Task: %s - %s\n", t.ID, t.Title)
	}

	// Complete the task
	err = service.CompleteTask(task.ID)
	if err != nil {
		log.Fatalf("Failed to complete task: %v", err)
	}

	fmt.Printf("Task %s completed\n", task.ID)

	// Wait a bit to demonstrate time usage
	time.Sleep(100 * time.Millisecond)

	fmt.Println("Standard library usage confirmed")
}
