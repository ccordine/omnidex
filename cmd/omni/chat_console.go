package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"sync"

	"golang.org/x/sys/unix"
	"golang.org/x/term"
)

type terminalReadWriter struct {
	reader io.Reader
	writer io.Writer
}

func (stream terminalReadWriter) Read(buffer []byte) (int, error) {
	return stream.reader.Read(buffer)
}

func (stream terminalReadWriter) Write(buffer []byte) (int, error) {
	return stream.writer.Write(buffer)
}

type chatConsole struct {
	input io.Reader
	out   io.Writer
	err   io.Writer

	terminal      *term.Terminal
	terminalFD    int
	terminalState *term.State
	closeOnce     sync.Once
	promptMu      sync.Mutex
	prompt        string
}

func newChatConsole(input io.Reader, output, errorsOutput io.Writer) (*chatConsole, error) {
	if input == nil || output == nil || errorsOutput == nil {
		return nil, fmt.Errorf("interactive console requires stdin, stdout, and stderr")
	}
	console := &chatConsole{input: input, out: output, err: errorsOutput, terminalFD: -1}
	inputFile, inputOK := input.(*os.File)
	outputFile, outputOK := output.(*os.File)
	if !inputOK || !outputOK || !term.IsTerminal(int(inputFile.Fd())) ||
		!term.IsTerminal(int(outputFile.Fd())) {
		return console, nil
	}

	fd := int(inputFile.Fd())
	state, err := term.MakeRaw(fd)
	if err != nil {
		return nil, fmt.Errorf("enter terminal raw mode: %w", err)
	}
	if err := enableTerminalInterruptSignal(fd); err != nil {
		_ = term.Restore(fd, state)
		return nil, err
	}
	interactive := term.NewTerminal(
		terminalReadWriter{reader: inputFile, writer: output},
		"you> ",
	)
	if width, height, sizeErr := term.GetSize(int(outputFile.Fd())); sizeErr == nil {
		_ = interactive.SetSize(width, height)
	}
	console.terminal = interactive
	console.terminalFD = fd
	console.terminalState = state
	console.prompt = "you> "
	return console, nil
}

func enableTerminalInterruptSignal(fd int) error {
	settings, err := unix.IoctlGetTermios(fd, unix.TCGETS)
	if err != nil {
		return fmt.Errorf("read terminal interrupt settings: %w", err)
	}
	settings.Lflag |= unix.ISIG
	if err := unix.IoctlSetTermios(fd, unix.TCSETS, settings); err != nil {
		return fmt.Errorf("enable terminal interrupt signal: %w", err)
	}
	return nil
}

func (console *chatConsole) Close() error {
	if console == nil {
		return nil
	}
	var closeErr error
	console.closeOnce.Do(func() {
		if console.terminalState != nil && console.terminalFD >= 0 {
			closeErr = term.Restore(console.terminalFD, console.terminalState)
		}
	})
	if closeErr != nil {
		return fmt.Errorf("restore terminal state: %w", closeErr)
	}
	return nil
}

func (console *chatConsole) ReadLine() (string, error) {
	if console == nil {
		return "", fmt.Errorf("interactive console is unavailable")
	}
	if console.terminal != nil {
		line, err := console.terminal.ReadLine()
		if errors.Is(err, term.ErrPasteIndicator) {
			return line, nil
		}
		return line, err
	}
	return "", fmt.Errorf("non-terminal input requires the buffered reader")
}

func (console *chatConsole) IsTerminal() bool {
	return console != nil && console.terminal != nil
}

func (console *chatConsole) SetPrompt(prompt string) error {
	if console == nil || prompt == "" {
		return fmt.Errorf("interactive prompt is required")
	}
	console.promptMu.Lock()
	defer console.promptMu.Unlock()
	if console.prompt == prompt {
		return nil
	}
	console.prompt = prompt
	if console.terminal == nil {
		_, err := fmt.Fprint(console.out, prompt)
		return err
	}
	console.terminal.SetPrompt(prompt)
	_, err := console.terminal.Write(nil)
	return err
}

func (console *chatConsole) WriteOutput(value string) error {
	if console == nil {
		return fmt.Errorf("interactive console is unavailable")
	}
	if console.terminal != nil {
		_, err := console.terminal.Write([]byte(value))
		return err
	}
	_, err := io.WriteString(console.out, value)
	return err
}

func (console *chatConsole) WriteError(value string) error {
	if console == nil {
		return fmt.Errorf("interactive console is unavailable")
	}
	if console.terminal != nil {
		_, err := console.terminal.Write([]byte(value))
		return err
	}
	_, err := io.WriteString(console.err, value)
	return err
}
