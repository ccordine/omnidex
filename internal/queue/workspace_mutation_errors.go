package queue

import "errors"

var (
	ErrWorkspaceMutationConflict   = errors.New("workspace mutation identity conflict")
	ErrWorkspaceMutationUnresolved = errors.New("workspace mutation requires reconciliation")
)
