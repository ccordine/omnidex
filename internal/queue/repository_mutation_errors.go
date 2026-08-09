package queue

import "errors"

var (
	ErrRepositoryMutationConflict   = errors.New("repository mutation identity conflict")
	ErrRepositoryMutationUnresolved = errors.New("repository mutation requires manual reconciliation")
)
