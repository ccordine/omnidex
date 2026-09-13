package experiment

import (
	"bufio"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net/http"
)

func startDockerExec(ctx context.Context, target string, input []byte, result *Result) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	stdout, stderr := &boundedBuffer{cancel: cancel}, &boundedBuffer{cancel: cancel}
	err := dockerAPI(ctx, http.MethodPost, target, map[string]bool{"Detach": false, "Tty": false}, nil,
		func(response *http.Response, reader *bufio.Reader, writer io.WriteCloser) error {
			if response.Header.Get("Upgrade") != "tcp" {
				return fmt.Errorf("Docker exec did not upgrade to its declared stream protocol")
			}
			written := make(chan error, 1)
			go func() {
				_, err := writer.Write(input)
				err = errors.Join(err, writer.Close())
				if err != nil {
					cancel()
				}
				written <- err
			}()
			err := readDockerExecStreams(reader, stdout, stderr)
			if err != nil {
				cancel()
			}
			return errors.Join(err, <-written)
		})
	result.Stdout, result.Stderr = stdout.Bytes(), stderr.Bytes()
	result.StdoutComplete, result.StderrComplete = !stdout.overflow && err == nil, !stderr.overflow && err == nil
	if stdout.overflow || stderr.overflow {
		return fmt.Errorf("Docker exec output exceeded the %d-byte stream bound", MaxStreamBytes)
	}
	return err
}

func readDockerExecStreams(reader io.Reader, stdout, stderr io.Writer) error {
	for {
		var header [8]byte
		if _, err := io.ReadFull(reader, header[:]); err != nil {
			if err == io.EOF {
				return nil
			}
			return fmt.Errorf("read Docker exec frame: %w", err)
		}
		if header[1] != 0 || header[2] != 0 || header[3] != 0 {
			return fmt.Errorf("Docker exec stream has an invalid frame header")
		}
		var writer io.Writer
		switch header[0] {
		case 1:
			writer = stdout
		case 2:
			writer = stderr
		default:
			return fmt.Errorf("Docker exec returned unsupported stream %d", header[0])
		}
		if _, err := io.CopyN(writer, reader, int64(binary.BigEndian.Uint32(header[4:]))); err != nil {
			return fmt.Errorf("read Docker exec frame payload: %w", err)
		}
	}
}
