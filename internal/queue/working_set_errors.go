package queue

import "errors"

var (
	ErrWorkingSetNotFound = errors.New("working set not found")
	ErrWorkingSetExists   = errors.New("working set already exists")
)
