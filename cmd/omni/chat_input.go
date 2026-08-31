package main

import (
	"context"
	"errors"
	"io"
)

type chatInput struct {
	Text   string
	Pasted bool
	Err    error
	EOF    bool
}

func readChatInput(ctx context.Context, console *chatConsole) (<-chan chatInput, <-chan struct{}) {
	events := make(chan chatInput, 16)
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer close(events)
		for {
			line, pasted, err := console.ReadLine()
			if errors.Is(err, io.EOF) {
				sendChatInput(ctx, events, chatInput{EOF: true})
				return
			}
			if err != nil {
				if !sendChatInput(ctx, events, chatInput{Err: err}) {
					return
				}
				if errors.Is(err, errChatInputRejected) {
					continue
				}
				return
			}
			if !sendChatInput(ctx, events, chatInput{Text: line, Pasted: pasted}) {
				return
			}
		}
	}()
	return events, done
}

func sendChatInput(ctx context.Context, events chan<- chatInput, event chatInput) bool {
	select {
	case <-ctx.Done():
		return false
	case events <- event:
		return true
	}
}
