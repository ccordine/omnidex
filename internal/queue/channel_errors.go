package queue

import "errors"

var (
	ErrChannelAlreadyExists      = errors.New("channel already exists")
	ErrCLIChatSessionConflict    = errors.New("CLI chat session channel conflicts with persisted authority")
	ErrChannelTurnActive         = errors.New("channel already has an active turn")
	ErrChannelSessionAuthority   = errors.New("invalid channel session authority")
	ErrChannelSessionWorkspace   = errors.New("channel session workspace differs")
	ErrChannelSessionNotFound    = errors.New("channel session does not exist")
	ErrChannelSessionTurnInvalid = errors.New("invalid channel session turn")
)
