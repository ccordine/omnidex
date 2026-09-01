package main

import (
	"bufio"
)

type chatInputEvent struct {
	line string
	err  error
	eof  bool
}

type chatInputReader struct {
	events chan chatInputEvent
}

func newChatInputReader(scanner *bufio.Scanner) *chatInputReader {
	reader := &chatInputReader{
		events: make(chan chatInputEvent, 64),
	}
	go func() {
		if scanner == nil {
			reader.events <- chatInputEvent{eof: true}
			close(reader.events)
			return
		}
		for scanner.Scan() {
			reader.events <- chatInputEvent{line: scanner.Text()}
		}
		if err := scanner.Err(); err != nil {
			reader.events <- chatInputEvent{err: err}
		} else {
			reader.events <- chatInputEvent{eof: true}
		}
		close(reader.events)
	}()
	return reader
}

func (r *chatInputReader) readBlocking() (string, bool, error) {
	if r == nil {
		return "", true, nil
	}
	event, ok := <-r.events
	if !ok {
		return "", true, nil
	}
	if event.err != nil {
		return "", false, event.err
	}
	if event.eof {
		return "", true, nil
	}
	return event.line, false, nil
}

func (r *chatInputReader) readNonBlocking() (chatInputEvent, bool) {
	if r == nil {
		return chatInputEvent{}, false
	}
	select {
	case event, ok := <-r.events:
		if !ok {
			return chatInputEvent{eof: true}, true
		}
		return event, true
	default:
		return chatInputEvent{}, false
	}
}
