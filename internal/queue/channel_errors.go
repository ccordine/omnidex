package queue

import "errors"

var (
	ErrChannelAlreadyExists = errors.New("channel already exists")
	ErrChannelTurnActive    = errors.New("channel already has an active turn")
	ErrChannelDataAuthority = errors.New("channel data authority is invalid")
)
