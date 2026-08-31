package main

import (
	"bufio"
	"errors"
	"io"
)

type chatInput struct {
	Text string
	Err  error
	EOF  bool
}

func readChatInput(console *chatConsole) <-chan chatInput {
	events := make(chan chatInput, 16)
	go func() {
		defer close(events)
		if console.IsTerminal() {
			for {
				line, err := console.ReadLine()
				if errors.Is(err, io.EOF) {
					events <- chatInput{EOF: true}
					return
				}
				if err != nil {
					events <- chatInput{Err: err}
					return
				}
				events <- chatInput{Text: line}
			}
		}
		scanner := bufio.NewScanner(console.input)
		scanner.Buffer(make([]byte, 4096), 64*1024)
		for scanner.Scan() {
			events <- chatInput{Text: scanner.Text()}
		}
		if err := scanner.Err(); err != nil {
			events <- chatInput{Err: err}
			return
		}
		events <- chatInput{EOF: true}
	}()
	return events
}
