package host

import "errors"

var (
	ErrNotConfigured    = errors.New("labyrinth durable host is not configured")
	ErrSchemaInvalid    = errors.New("labyrinth durable host schema is invalid")
	ErrEpisodeNotFound  = errors.New("labyrinth durable episode was not found")
	ErrReceiptNotFound  = errors.New("labyrinth durable action receipt was not found")
	ErrScenarioConflict = errors.New("labyrinth durable episode scenario conflict")
	ErrReceiptCorrupt   = errors.New("labyrinth durable receipt is corrupt")
)
