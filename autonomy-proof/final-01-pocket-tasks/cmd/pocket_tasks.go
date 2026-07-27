package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"your-module/domain"
	"your-module/storage"
)

func main() {
	file := flag.String("file", "tasks.json", "Path to the tasks file")
	flag.Parse()

	args := flag.Args()
	if len(args) == 0 {
		fmt.Println("Usage: pocket_tasks [add <title>|list|done <id>] --file <path>")
		ose.Exit(1)
	}

	storage, err := storage.NewJSONTaskStorage(*file)
	if err != nil {
		log.Fatalf("Failed to create storage: %v", err)
	}

	service := domain.NewTaskService(storage)

	command := args[0]
	switch command {
	case "add":
		if len(args) < 2 {
			log.Fatal("Usage: pocket_tasks add <title>")
		}
		title := args[1]
		task, err := service.CreateTask(title)
		if err != nil {
			log.Fatalf("Failed to create task: %v", err)
		}
		fmt.Printf("Created task: %s\n", task.ID)

	case "list":
		tasks, err := service.GetTasks()
		if err != nil {
			log.Fatalf("Failed to get tasks: %v", err)
		}

		for _, task := range tasks {
			status := "pending"
			if task.Completed {
				status = "completed"
			}
			fmt.Printf("%s - %s (%s)\n", task.ID, task.Title, status)
		}

	case "done":
		if len(args) < 2 {
			log.Fatal("Usage: pocket_tasks done <id>")
		}
		id := args[1]
		err := service.CompleteTask(id)
		if err != nil {
			log.Fatalf("Failed to complete task: %v", err)
		}
		fmt.Printf("Task %s completed\n", id)

	default:
		log.Fatalf("Unknown command: %s", command)
	}
}
