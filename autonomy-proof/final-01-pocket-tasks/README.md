# Pocket Tasks

Pocket Tasks is a simple command-line tool to manage tasks.

## Installation

```sh
git clone https://github.com/your-module/pocket_tasks.git
cd pocket_tasks
go build -o pocket_tasks cmd/pocket_tasks.go
```

## Usage

### Add a Task

```sh
pocket_tasks add "Buy groceries" --file tasks.json
```

### List Tasks

```sh
pocket_tasks list --file tasks.json
```

### Mark a Task as Completed

```sh
pocket_tasks done 1234567890 --file tasks.json
```
