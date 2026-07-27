# Pocket Tasks
A simple command-line task tracker written in Go.
## Usage
### Add a Task
```sh
go run main.go add --file tasks.json "Task description"
```
### List Tasks
```sh
go run main.go list --file tasks.json
```
### Mark Task as Done
```sh
go run main.go done --file tasks.json <task_id>
```
