package experiment

import (
	"context"
	"errors"
	"io"
)

func (docker *Docker) copyInput(ctx context.Context, id string, files []File) error {
	reader, writer := io.Pipe()
	written := make(chan error, 1)
	go func() {
		err := writeInputArchive(writer, files)
		_ = writer.CloseWithError(err)
		written <- err
	}()
	_, stderr, err := docker.capture(ctx, []string{"container", "cp", "--archive", "-", id + ":/"}, reader)
	_ = reader.CloseWithError(err)
	return errors.Join(dockerOperationError("copy exact inputs", stderr, err), <-written)
}

func (docker *Docker) collect(parent context.Context, id string, paths []string, subtree string) ([]File, error) {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	reader, writer := io.Pipe()
	stderr := &boundedBuffer{cancel: cancel}
	finished := make(chan error, 1)
	go func() {
		err := docker.cli(ctx, []string{"container", "cp", id + ":" + WorkingDirectory, "-"}, nil, writer, stderr)
		_ = writer.CloseWithError(err)
		finished <- err
	}()
	files, err := readArtifactArchive(reader, paths, subtree)
	if err != nil {
		cancel()
	}
	_ = reader.CloseWithError(err)
	commandErr := <-finished
	if stderr.overflow {
		commandErr = errors.Join(commandErr, errors.New("Docker archive diagnostic exceeded its byte bound"))
	}
	if err != nil || commandErr != nil {
		return nil, errors.Join(err, dockerOperationError("collect declared artifacts", stderr.Bytes(), commandErr))
	}
	return files, nil
}
